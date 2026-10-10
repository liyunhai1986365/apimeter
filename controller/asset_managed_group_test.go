package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestAssetManagedGroupPreRequestCancellation(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached=%v", cached), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var calls atomic.Int32
			canceled := make(chan bool, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				cancel()
				select {
				case <-r.Context().Done():
					canceled <- true
				case <-time.After(2 * time.Second):
					canceled <- false
					w.WriteHeader(http.StatusGatewayTimeout)
				}
			}))
			defer upstream.Close()
			router := assetSecurityRouter(t, upstream.URL, "service-inference", "seedance2-service-inference")
			if cached {
				seedAssetOwnershipForTest(t, 20, 1, "group", "group-cached")
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
				require.True(t, ok)
				resource, ok := profile.ResourceByID("assets_create")
				require.True(t, ok)
				require.NoError(t, model.UpsertConfigurableResourceState(&model.ConfigurableResourceState{
					ChannelID: 20, ProfileID: profile.ID, ResourceID: resource.ID, PreRequestID: "asset_group",
					UserID: 1, TokenID: 1, StateKey: configurableResourceStateKey(ch, resource, "asset_group_id"), StateValue: "group-cached",
				}))
			}
			req := httptest.NewRequest(http.MethodPost, "/api/assets?model="+tgxRegressionModel,
				strings.NewReader(`{"url":"https://example.com/a.png","asset_type":"Image"}`)).WithContext(ctx)
			req.Header.Set("Authorization", "Bearer seedancetest1")
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			require.True(t, <-canceled, "validation and creation pre-requests must stop when the caller cancels")
			require.EqualValues(t, 1, calls.Load(), "canceled preparation must not submit the asset upload")
			require.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
		})
	}
}

func TestAssetManagedGroupAcceptedCreationPersistsAfterCancellation(t *testing.T) {
	assetSecurityRouter(t, "http://127.0.0.1", "service-inference", "seedance2-service-inference")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	resource, ok := profile.ResourceByID("assets_create")
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/assets", nil).WithContext(ctx)
	common.SetContextKey(c, constant.ContextKeyUserId, 1)
	common.SetContextKey(c, constant.ContextKeyTokenId, 1)
	c.Set(middleware.ContextKeyConfigurableResourceProfileID, profile.ID)
	c.Set(assetAccessContextKey, &assetAccessRequest{channel: ch, profile: profile, resource: resource, scope: scope})
	var observedDeadline bool
	const callback = "test:accepted_group_deadline"
	require.NoError(t, model.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		deadline, exists := tx.Statement.Context.Deadline()
		observedDeadline = observedDeadline || exists
		require.True(t, exists, "accepted group bookkeeping must have a deadline")
		require.NoError(t, tx.Statement.Context.Err(), "the canceled caller must not abandon accepted creation")
		require.LessOrEqual(t, time.Until(deadline), model.AssetOperationTimeout)
	}))
	t.Cleanup(func() { _ = model.DB.Callback().Create().Remove(callback) })
	require.NoError(t, persistConfigurablePreRequestResult(c, ch, resource, resource.PreRequests[0], map[string]any{"id": "accepted-group"}))
	require.True(t, observedDeadline)
	bindings, err := model.FindAssetBindings(1, "group", "accepted-group")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	state, err := model.FindActiveConfigurableResourceState(20, profile.ID, resource.ID, "asset_group", 1, 1, configurableResourceStateKey(ch, resource, "asset_group_id"))
	require.NoError(t, err)
	require.Equal(t, "accepted-group", state.StateValue)
}

func TestAssetManagedGroupLookupFailureDoesNotCreateUpstreamResources(t *testing.T) {
	for _, route := range []struct{ resource, path string }{
		{"assets_upload", "/api/assets/upload"},
		{"assets_create", "/api/assets"},
	} {
		for _, cached := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/cached=%v", route.resource, cached), func(t *testing.T) {
				var calls, creates, validates, uploads atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					switch {
					case r.Method == http.MethodGet && r.URL.Path == "/v1/asset-groups/group-cached":
						validates.Add(1)
						_, _ = w.Write([]byte(`{"id":"group-cached"}`))
					case r.Method == http.MethodPost && r.URL.Path == "/v1/asset-groups":
						creates.Add(1)
						_, _ = w.Write([]byte(`{"id":"group-created"}`))
					case r.Method == http.MethodPost && r.URL.Path == "/v1/assets":
						uploads.Add(1)
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Error(err)
						}
						wantGroup := "group-created"
						if cached {
							wantGroup = "group-cached"
						}
						if got := gjson.GetBytes(body, "group_id").String(); got != wantGroup {
							t.Errorf("uploaded to group %q, want %q", got, wantGroup)
						}
						_, _ = w.Write([]byte(`{"id":"asset-created","status":"processing"}`))
					default:
						t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer upstream.Close()
				router := assetSecurityRouter(t, upstream.URL, "service-inference", "seedance2-service-inference")
				db := model.DB
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
				require.True(t, ok)
				resource, ok := profile.ResourceByID(route.resource)
				require.True(t, ok)
				stateKey := configurableResourceStateKey(ch, resource, "asset_group_id")
				if cached {
					seedAssetOwnershipForTest(t, 20, 1, "group", "group-cached")
					require.NoError(t, model.UpsertConfigurableResourceState(&model.ConfigurableResourceState{
						ChannelID: 20, ProfileID: profile.ID, ResourceID: resource.ID, PreRequestID: "asset_group",
						UserID: 1, TokenID: 1, StateKey: stateKey,
						StateValue: "group-cached", Status: model.ConfigurableResourceStateStatusActive,
					}))
				}
				var before, after []model.ConfigurableResourceState
				require.NoError(t, db.Order("id").Find(&before).Error)
				lookupFailed := false
				const callback = "test:managed_group_lookup_failure"
				require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table != "configurable_resource_states" {
						return
					}
					// Fail only the managed-state SELECT, leaving authorization and
					// ownership queries intact so the real API reaches this branch.
					where, _ := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
					for _, condition := range where.Exprs {
						if expr, ok := condition.(clause.Expr); ok && strings.Contains(expr.SQL, "pre_request_id = ?") {
							for _, value := range expr.Vars {
								if key, ok := value.(string); ok && key == stateKey {
									lookupFailed = true
									_ = tx.AddError(errors.New("managed group database unavailable"))
								}
							}
						}
					}
				}))
				t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })
				path := route.path + "?model=" + tgxRegressionModel
				const body = `{"url":"https://example.com/a.png","asset_type":"Image"}`
				failed := seedanceCall(router, http.MethodPost, path, body, 1)
				require.True(t, lookupFailed, "the managed-state query must reach the injected failure")
				require.Zero(t, calls.Load(), "a database failure must not create or validate upstream resources")
				require.Equal(t, http.StatusBadGateway, failed.Code, failed.Body.String())
				require.Equal(t, "asset_prepare_failed", gjson.GetBytes(failed.Body.Bytes(), "error.code").String())
				require.NoError(t, db.Order("id").Find(&after).Error)
				require.Equal(t, before, after, "cache and ownership must remain unchanged on lookup failure")

				require.NoError(t, db.Callback().Query().Remove(callback))
				var cacheWrites atomic.Int32
				const writeCallback = "test:managed_group_cache_writes"
				require.NoError(t, db.Callback().Create().Before("gorm:create").Register(writeCallback, func(tx *gorm.DB) {
					if state, ok := tx.Statement.Dest.(*model.ConfigurableResourceState); ok && state.StateKey == stateKey {
						cacheWrites.Add(1)
					}
				}))
				t.Cleanup(func() { _ = db.Callback().Create().Remove(writeCallback) })
				recovered := seedanceCall(router, http.MethodPost, path, body, 1)
				require.Equal(t, http.StatusOK, recovered.Code, recovered.Body.String())
				require.EqualValues(t, 1, uploads.Load())
				if cached {
					require.Zero(t, cacheWrites.Load(), "cache reuse must not acquire a write lock via upsert")
					require.EqualValues(t, 1, validates.Load())
					require.Zero(t, creates.Load(), "a healthy retry must reuse the original cached group")
				} else {
					require.EqualValues(t, 1, cacheWrites.Load(), "a newly created group must be cached")
					require.Zero(t, validates.Load())
					require.EqualValues(t, 1, creates.Load(), "a genuine cache miss must still create a group")
				}
			})
		}
	}
}
