package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetQueriesPreserveUpstreamStatus(t *testing.T) {
	for _, backend := range []string{"volcengine-assets", "tgxmaas", "youniyouju"} {
		t.Run(backend, func(t *testing.T) {
			var status, calls atomic.Int32
			var list atomic.Bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "7")
				w.Header().Set("X-Request-Id", "asset-upstream-request")
				w.WriteHeader(int(status.Load()))
				body := `{"ResponseMetadata":{"Error":{"Code":"UpstreamError","Message":"try later"}}}`
				if status.Load() < 400 {
					body = `{"Result":{"Id":"owned","Status":"Processing"}}`
					if list.Load() {
						body = `{"Result":{"Items":[{"Id":"owned","Status":"Processing"}],"TotalCount":1}}`
					}
				}
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, backend, "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			for _, route := range []struct {
				name, method, path, body string
				list                     bool
			}{
				{"rest-get", "GET", "/api/assets/owned", "", false},
				{"official-get", "POST", "/?Action=GetAsset&Version=2024-01-01", `{"Id":"owned"}`, false},
				{"rest-list", "GET", "/api/assets", "", true},
				{"official-list", "POST", "/?Action=ListAssets&Version=2024-01-01", `{}`, true},
			} {
				for _, code := range []int{200, 202, 400, 401, 403, 404, 409, 429, 500, 502, 503, 504} {
					t.Run(fmt.Sprintf("%s/%d", route.name, code), func(t *testing.T) {
						status.Store(int32(code))
						list.Store(route.list)
						before := calls.Load()
						got := seedanceCall(r, route.method, route.path, route.body, 1)
						require.Equal(t, code, got.Code, got.Body.String())
						require.Equal(t, "7", got.Header().Get("Retry-After"))
						require.Equal(t, "asset-upstream-request", got.Header().Get("X-Request-Id"))
						require.Equal(t, before+1, calls.Load(), "queries must not replay an upstream error")
						if code < 400 {
							path := "Result.Status"
							if route.list {
								path = "Result.Items.0.Status"
							}
							require.Equal(t, "Processing", gjson.GetBytes(got.Body.Bytes(), path).String())
						} else {
							require.Equal(t, "UpstreamError", gjson.GetBytes(got.Body.Bytes(), "ResponseMetadata.Error.Code").String())
						}
					})
				}
			}
		})
	}
}

func TestAssetPrerequisitesPreserveUpstreamErrors(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached=%v", cached), func(t *testing.T) {
			var status, calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				method, path := http.MethodPost, "/v1/asset-groups"
				if cached {
					method, path = http.MethodGet, "/v1/asset-groups/group-cached"
				}
				if r.Method != method || r.URL.Path != path {
					t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "9")
				w.Header().Set("X-Request-Id", "asset-prepare-request")
				w.WriteHeader(int(status.Load()))
				_, _ = w.Write([]byte(`{"error":{"code":"GroupUnavailable","message":"try later","channel_id":20}}`))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "service-inference", "seedance2-service-inference")
			if cached {
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
				require.True(t, ok)
				resource, ok := profile.ResourceByID("assets_upload")
				require.True(t, ok)
				require.NoError(t, model.UpsertConfigurableResourceState(&model.ConfigurableResourceState{
					ChannelID: 20, ProfileID: profile.ID, ResourceID: resource.ID, PreRequestID: "asset_group",
					UserID: 1, TokenID: 1, StateKey: configurableResourceStateKey(ch, resource, "asset_group_id"),
					StateValue: "group-cached", Status: model.ConfigurableResourceStateStatusActive,
				}))
			}
			for _, code := range []int{200, 400, 401, 403, 409, 429, 500, 502, 503, 504} {
				t.Run(fmt.Sprint(code), func(t *testing.T) {
					status.Store(int32(code))
					before := calls.Load()
					got := seedanceCall(r, "POST", "/api/assets/upload?model="+tgxRegressionModel, `{"url":"https://example.com/a.png","asset_type":"Image"}`, 1)
					require.Equal(t, code, got.Code, got.Body.String())
					require.JSONEq(t, `{"error":{"code":"GroupUnavailable","message":"try later"}}`, got.Body.String())
					require.Equal(t, "9", got.Header().Get("Retry-After"))
					require.Equal(t, "asset-prepare-request", got.Header().Get("X-Request-Id"))
					require.Equal(t, before+1, calls.Load(), "failed preparation must not create another group or upload an asset")
					var states []model.ConfigurableResourceState
					require.NoError(t, model.DB.Where("pre_request_id = ?", "asset_group").Find(&states).Error)
					if cached {
						require.Len(t, states, 1)
						require.Equal(t, "group-cached", states[0].StateValue)
						require.Equal(t, model.ConfigurableResourceStateStatusActive, states[0].Status)
						require.Zero(t, states[0].FailCount)
					} else {
						require.Empty(t, states)
					}
					var bindings int64
					require.NoError(t, model.DB.Model(&model.ConfigurableResourceState{}).Where("profile_id = ?", "asset-access-v1").Count(&bindings).Error)
					require.Zero(t, bindings, "upstream errors must not establish asset ownership")
				})
			}
		})
	}
}
