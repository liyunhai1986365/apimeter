package controller

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetSecurityMutationAndRevocation(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("Action") == "DeleteAsset" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"Id":"owned","upstream_asset_id":"original"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "group", "group-owned")
	created := seedanceCall(r, "POST", "/api/assets", `{"GroupId":"group-owned","URL":"https://example.com/a.png"}`, 1)
	require.Equal(t, 200, created.Code, created.Body.String())
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/api/assets/owned", ""},
		{"PATCH", "/api/assets/original", `{"Name":"stolen"}`},
		{"DELETE", "/api/assets/owned", ""},
		{"POST", "/api/assets", `{"GroupId":"group-owned","URL":"https://example.com/a.png"}`},
		{"GET", "/api/assets", `{"Filter":{"GroupIds":["group-owned"]}}`},
	} {
		denied := seedanceCall(r, tc.method, tc.path, tc.body, 2)
		require.Equal(t, 404, denied.Code, denied.Body.String())
	}
	require.Equal(t, int32(1), calls.Load())
	for _, body := range []string{
		`{"GroupId":"group-owned","GroupId":"foreign"}`,
		`{"groupID":"foreign"}`,
		`{"GroupId":"group-owned","groupID":"foreign"}`,
	} {
		denied := seedanceCall(r, "POST", "/api/assets", body, 1)
		require.True(t, denied.Code == 400 || denied.Code == 404, denied.Body.String())
	}
	require.Equal(t, int32(1), calls.Load(), "ambiguous/case-variant IDs must not bypass authorization")
	deleted := seedanceCall(r, "DELETE", "/api/assets/original", "", 1)
	require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
	for _, id := range []string{"owned", "original"} {
		denied := seedanceCall(r, "GET", "/api/assets/"+id, "", 1)
		require.Equal(t, 404, denied.Code)
	}
	require.Equal(t, int32(2), calls.Load(), "deleted aliases never reach upstream again")
}

func TestAssetSecurityAccountChangesAndSessionExpiry(t *testing.T) {
	for _, change := range []string{"url", "backend", "expired-session"} {
		t.Run(change, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "old-asset")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			settings := ch.GetSetting()
			switch change {
			case "url":
				settings.Protocol.AssetLibrary.BaseURL += "/different-account"
			case "backend":
				settings.Protocol.AssetLibrary.Backend = "tgxmaas"
			case "expired-session":
				profile, _ := configurable.AssetProfile(settings.Protocol)
				scope, err := assetAccountScope(ch, profile)
				require.NoError(t, err)
				require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "session", ExpiresAt: common.GetTimestamp() - 1}, "expired"))
			}
			ch.SetSetting(settings)
			require.NoError(t, model.DB.Save(ch).Error)
			var got *httptest.ResponseRecorder
			if change == "expired-session" {
				got = assetSecurityAction(r, "GetVisualValidateResult", `{"BytedToken":"expired"}`, 1)
			} else {
				got = seedanceCall(r, "GET", "/api/assets/old-asset", "", 1)
			}
			require.Equal(t, 404, got.Code, got.Body.String())
			require.Zero(t, calls.Load())
		})
	}
}

func TestAssetSecurityVideoReferences(t *testing.T) {
	var calls, wrongCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"code":"Overloaded","message":"busy"}}`))
	}))
	defer upstream.Close()
	wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrongCalls.Add(1); w.WriteHeader(500) }))
	defer wrong.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "seedance-tgxmaas")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "video-asset")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.Key = "rotated-credential-for-same-library"
	require.NoError(t, model.DB.Save(ch).Error)
	ch.Id, ch.Key, ch.BaseURL, ch.Priority = 21, "another-account", &wrong.URL, common.GetPointer(int64(100))
	require.NoError(t, model.DB.Create(ch).Error)
	createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
	payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"image_url","image_url":{"url":"asset://video-asset"}}],"duration":4}`
	denied := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 2)
	require.Equal(t, 404, denied.Code, denied.Body.String())
	denied = seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", strings.ReplaceAll(payload, "asset://video-asset", " ASSET://video-asset "), 2)
	require.Equal(t, 404, denied.Code, denied.Body.String())
	require.Zero(t, calls.Load())
	owner := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
	require.Equal(t, 503, owner.Code, owner.Body.String())
	require.Equal(t, int32(1), calls.Load(), "failed submit must not be replayed")
	require.Zero(t, wrongCalls.Load(), "higher priority cannot move an asset to another account")
	// Exercise multipart normalization before the same production guard.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("metadata", `{"content":[{"image_url":{"url":"asset://video-asset"}}]}`))
	require.NoError(t, writer.Close())
	for _, user := range []int{1, 2} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewReader(body.Bytes()))
		ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
		info := &relaycommon.RelayInfo{UserId: user, OriginModelName: tgxRegressionModel, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		err := lockAssetVideoChannel(ctx, info)
		if user == 2 {
			require.ErrorIs(t, err, model.ErrAssetNotOwned)
		} else {
			require.NoError(t, err)
			require.Equal(t, 20, info.LockedChannel.(*model.Channel).Id)
		}
	}
}

// Legacy wire-format tests explicitly declare ownership of pre-existing mock
// handles. Production has no implicit adoption on first read/list.
func seedAssetOwnershipForTest(t *testing.T, channelID, userID int, kind, handle string) {
	t.Helper()
	ch, err := model.GetChannelById(channelID, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	binding := model.AssetBinding{ChannelID: channelID, UserID: userID, Backend: profile.ID, Scope: scope, Kind: kind}
	if kind == "session" {
		binding.ExpiresAt = common.GetTimestamp() + 1800
	}
	require.NoError(t, model.SaveAssetBinding(binding, handle))
}

func assetSecurityRouter(t *testing.T, origin, backend, inheritedProfile string) *gin.Engine {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", "asset-security-test")
	r := tgxMaasResourceTestRouter(t, origin, "account-a", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	r.POST(configurable.YouniyoujuAssetPath, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayArkAssetAction)
	for _, path := range []string{"/v2/sd-max/assets", "/v2/db-sd-max/assets"} {
		r.POST(path, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
		r.GET(path+"/:id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	}
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2", ProjectName: "test-project"}
	if inheritedProfile != "" {
		settings.ProfileID = inheritedProfile
	} else {
		settings.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: backend, BaseURL: origin, AuthMode: "channel_key"}
	}
	if backend == configurable.OfficialAssetBackend {
		settings.AssetLibrary.AuthMode = "aksk"
		require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{AccessKeyID: "test-ak", SecretAccessKey: "test-sk"}))
	}
	ch.SetSetting(dto.ChannelSettings{Protocol: settings})
	require.NoError(t, model.DB.Save(ch).Error)
	return r
}

func assetSecurityAction(r http.Handler, action, body string, user int) *httptest.ResponseRecorder {
	return seedanceCall(r, "POST", configurable.YouniyoujuAssetPath+"?Action="+action+"&Version=2024-01-01", body, user)
}

func TestAssetSecurityAllBackends(t *testing.T) {
	cases := []struct{ backend, profile, createPath, getPath, response string }{
		{"volcengine-assets", "", "/api/assets", "/api/assets/asset-owned", `{"Result":{"Id":"asset-owned"}}`},
		{"youniyouju", "", "/api/assets", "/api/assets/asset-owned", `{"Result":{"Id":"asset-owned"}}`},
		{"tgxmaas", "seedance-tgxmaas", "/api/assets", "/api/assets/asset-owned", `{"Result":{"Id":"asset-owned"}}`},
		{"task", "seedance2-ark-task-assets", "/api/assets/upload", "/api/assets/asset-owned", `{"code":0,"data":{"asset_id":"asset-owned"}}`},
		{"modelsell", "seedance2-modelsell", "/api/assets/upload", "/api/assets/asset-owned", `{"code":0,"data":{"Id":"asset-owned"}}`},
		{"api-assets", "doubao-seedance-2-api-assets", "/api/assets", "/api/assets/asset-owned", `{"id":"asset-owned"}`},
		{"material", "doubao-seedance-2", "/api/assets", "/api/assets/asset-owned", `{"id":"asset-owned"}`},
		{"service-inference", "seedance2-service-inference", "/api/assets", "/api/assets/asset-owned", `{"id":"asset-owned","task_id":"task-owned"}`},
		{"max-service-inference", "doubao-seedance-max-service-inference", "/v2/db-sd-max/assets", "/v2/db-sd-max/assets/asset-owned", `{"success":true,"data":{"Id":"asset-owned"}}`},
	}
	for _, tc := range cases {
		for _, inherited := range []bool{false, true} {
			if inherited && tc.profile == "" {
				continue
			}
			t.Run(fmt.Sprintf("%s/inherit=%v", tc.backend, inherited), func(t *testing.T) {
				var calls, wrongCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/v1/asset-groups" {
						_, _ = w.Write([]byte(`{"id":"group-auto"}`))
						return
					}
					_, _ = w.Write([]byte(tc.response))
				}))
				defer upstream.Close()
				wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wrongCalls.Add(1); w.WriteHeader(404) }))
				defer wrong.Close()
				profile := ""
				if inherited {
					profile = tc.profile
				}
				r := assetSecurityRouter(t, upstream.URL, tc.backend, profile)
				created := seedanceCall(r, "POST", tc.createPath, `{"model":"`+tgxRegressionModel+`","url":"https://example.com/test.png","asset_type":"Image"}`, 1)
				require.Equal(t, 200, created.Code, created.Body.String())
				before := calls.Load()
				for _, id := range []string{"asset-owned", "asset-untracked"} {
					denied := seedanceCall(r, "GET", strings.ReplaceAll(tc.getPath, "asset-owned", id), "", 2)
					require.Equal(t, 404, denied.Code, denied.Body.String())
				}
				require.Equal(t, before, calls.Load(), "unauthorized handles must not reach the supplier")
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				ch.Id, ch.Name, ch.Key, ch.BaseURL, ch.Priority = 21, "higher-priority", "account-b", &wrong.URL, common.GetPointer(int64(100))
				settings := ch.GetSetting()
				if settings.Protocol.AssetLibrary != nil {
					settings.Protocol.AssetLibrary.BaseURL = wrong.URL
					ch.SetSetting(settings)
				}
				require.NoError(t, model.DB.Create(ch).Error)
				createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
				got := seedanceCall(r, "GET", tc.getPath, "", 1)
				require.Equal(t, 200, got.Code, got.Body.String())
				require.Equal(t, before+1, calls.Load())
				require.Zero(t, wrongCalls.Load(), "a follow-up must stay on its creation channel")
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 20).Update("status", common.ChannelStatusManuallyDisabled).Error)
				got = seedanceCall(r, "GET", tc.getPath, "", 1)
				require.Equal(t, 404, got.Code)
				require.Zero(t, wrongCalls.Load(), "unavailable original channels must not fall back")
			})
		}
	}
}

func TestAssetSecurityListPagination(t *testing.T) {
	for _, backend := range []string{"volcengine-assets", "youniyouju", "tgxmaas", "api-assets", "material"} {
		t.Run(backend, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				page := r.URL.Query().Get("page")
				if r.Method == "POST" {
					body, _ := io.ReadAll(r.Body)
					page = gjson.GetBytes(body, "PageNumber").String()
				}
				rows := `[{"Id":"foreign-a","Name":"private-a"},{"Id":"owned-a","extension":9007199254740993}]`
				if page == "2" {
					rows = `[{"Id":"foreign-b","Name":"private-b"},{"Id":"owned-b"}]`
				}
				w.Header().Set("Content-Type", "application/json")
				if backend == "api-assets" || backend == "material" {
					_, _ = fmt.Fprintf(w, `{"data":{"items":%s,"total":4,"page":%s,"page_size":100}}`, rows, page)
				} else {
					_, _ = fmt.Fprintf(w, `{"Result":{"Items":%s,"TotalCount":4,"PageNumber":%s,"PageSize":100}}`, rows, page)
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, backend, "")
			for _, id := range []string{"owned-a", "owned-b"} {
				seedAssetOwnershipForTest(t, 20, 1, "asset", id)
			}
			response := seedanceCall(r, "GET", "/api/assets?page=1&page_size=1", "", 1)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "owned-a")
			require.NotContains(t, response.Body.String(), "owned-b")
			require.NotContains(t, response.Body.String(), "private")
			require.Contains(t, response.Body.String(), "9007199254740993")
			total := "Result.TotalCount"
			if backend == "material" || backend == "api-assets" {
				total = "data.total"
			}
			require.Equal(t, int64(2), gjson.GetBytes(response.Body.Bytes(), total).Int())
			response = seedanceCall(r, "GET", "/api/assets?page=2&page_size=1", "", 1)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "owned-b")
			require.NotContains(t, response.Body.String(), "owned-a")
			response = seedanceCall(r, "GET", "/api/assets", "", 2)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.NotContains(t, response.Body.String(), "owned")
			require.NotContains(t, response.Body.String(), "private")
			require.Zero(t, gjson.GetBytes(response.Body.Bytes(), total).Int())
			require.Equal(t, int32(5), calls.Load())
		})
	}
}

func TestAssetSecurityLivenessAndSmartErrors(t *testing.T) {
	for _, backend := range []string{"volcengine-assets", "youniyouju", "tgxmaas"} {
		t.Run(backend, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			var calls atomic.Int32
			const session = "private-liveness-session"
			const providerError = `{"ResponseMetadata":{"RequestId":"provider-request","Error":{"Code":"ExpiredSession","Message":"complete authentication again"}}}`
			fail := false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("Action") == "CreateVisualValidateSession" || r.URL.Path == "/v1/real-avatar/auth/session" {
					_, _ = w.Write([]byte(`{"BytedToken":"` + session + `","H5Link":"https://provider.example/h5","CallbackURL":"https://customer.example/callback"}`))
				} else if fail {
					w.Header().Set("Retry-After", "4")
					w.WriteHeader(400)
					_, _ = w.Write([]byte(providerError))
				} else {
					_, _ = w.Write([]byte(`{"GroupId":"group-human"}`))
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, backend, "")
			created := assetSecurityAction(r, "CreateVisualValidateSession", `{"callback_url":"https://customer.example/callback"}`, 1)
			require.Equal(t, 200, created.Code, created.Body.String())
			denied := assetSecurityAction(r, "GetVisualValidateResult", `{"BytedToken":"`+session+`"}`, 2)
			require.Equal(t, 404, denied.Code)
			require.Equal(t, int32(1), calls.Load())
			got := assetSecurityAction(r, "GetVisualValidateResult", `{"byted_token":"`+session+`"}`, 1)
			require.Equal(t, 200, got.Code, got.Body.String())
			groups, err := model.FindAssetBindings(1, "group", "group-human")
			require.NoError(t, err)
			require.Len(t, groups, 1)
			var states []model.ConfigurableResourceState
			require.NoError(t, model.DB.Find(&states).Error)
			for _, state := range states {
				require.NotContains(t, state.Metadata+state.StateValue+state.StateKey, session)
			}
			require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + model.RoutingStrategies()[0] + `"}`}).Error)
			fail = true
			got = assetSecurityAction(r, "GetVisualValidateResult", `{"BytedToken":"`+session+`"}`, 1)
			require.Equal(t, 400, got.Code, got.Body.String())
			require.Equal(t, providerError, got.Body.String())
			require.Equal(t, "4", got.Header().Get("Retry-After"))
			require.Equal(t, int32(3), calls.Load())
		})
	}
}

func TestAssetSecurityOperationCoverage(t *testing.T) {
	for _, profile := range configurable.ListProfiles() {
		for _, resource := range profile.Resources {
			if resource.AssetLibrary {
				_, ok := assetResourceOperation(resource.ID)
				require.True(t, ok, "%s/%s needs an access policy", profile.ID, resource.ID)
			}
		}
	}
}

func TestAssetSecurityCursorAndPageFailures(t *testing.T) {
	for _, mode := range []string{"cursor", "page-error", "loop"} {
		t.Run(mode, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				if gjson.GetBytes(body, "NextToken").String() == "provider-page2" {
					if mode == "page-error" {
						w.Header().Set("X-Request-Id", "second-page-error")
						w.WriteHeader(429)
						_, _ = w.Write([]byte(`{"error":{"code":"SlowDown"}}`))
						return
					}
					if mode != "loop" {
						_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"b"}],"NextToken":""}}`))
						return
					}
				}
				_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"foreign"},{"Id":"a"}],"NextToken":"provider-page2"}}`))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "a")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "b")
			got := assetSecurityAction(r, "ListAssets", `{"MaxResults":1}`, 1)
			switch mode {
			case "cursor":
				require.Equal(t, 200, got.Code, got.Body.String())
				require.Equal(t, "a", gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
				token := gjson.GetBytes(got.Body.Bytes(), "Result.NextToken").String()
				require.True(t, strings.HasPrefix(token, "asset-v1-"))
				got = assetSecurityAction(r, "ListAssets", `{"MaxResults":1,"NextToken":"`+token+`"}`, 1)
				require.Equal(t, 200, got.Code, got.Body.String())
				require.Equal(t, "b", gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
				require.Empty(t, gjson.GetBytes(got.Body.Bytes(), "Result.NextToken").String())
				before := calls.Load()
				got = assetSecurityAction(r, "ListAssets", `{"NextToken":"`+token+`"}`, 2)
				require.Equal(t, 400, got.Code)
				require.Equal(t, before, calls.Load())
			case "page-error":
				require.Equal(t, 429, got.Code, got.Body.String())
				require.Equal(t, `{"error":{"code":"SlowDown"}}`, got.Body.String())
				require.Equal(t, "second-page-error", got.Header().Get("X-Request-Id"))
			case "loop":
				require.Equal(t, 502, got.Code, got.Body.String())
				require.Equal(t, int32(2), calls.Load(), "repeated supplier pages must terminate")
			}
			require.NotContains(t, got.Body.String(), "foreign")
		})
	}
}

func TestAssetSecurityMappedErrorsRemainErrors(t *testing.T) {
	for _, body := range []string{`{"error":{"code":"UnknownAsset","request_id":"original-request"}}`, `<html>Not found</html>`} {
		t.Run(body[:5], func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-Request-Id", "original-request")
				w.WriteHeader(404)
				_, _ = w.Write([]byte(body))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "service-inference", "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			got := seedanceCall(r, "GET", "/api/assets/owned", "", 1)
			require.Equal(t, 404, got.Code)
			require.Equal(t, body, got.Body.String(), "response mappings cannot turn provider failure into code=0")
			require.Equal(t, "original-request", got.Header().Get("X-Request-Id"))
		})
	}
}

func TestAssetSecurityAsyncHandleResolution(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if gjson.GetBytes(body, "input.action").String() == "upload" {
			_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"upload-task"}}`))
		} else {
			_, _ = w.Write([]byte(`{"code":0,"data":{"asset_id":"finished-asset"}}`))
		}
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "task", "")
	created := seedanceCall(r, "POST", "/api/assets/upload", `{"url":"https://example.com/a.png","asset_type":"Image"}`, 1)
	require.Equal(t, 200, created.Code, created.Body.String())
	denied := seedanceCall(r, "GET", "/api/assets/upload-task", "", 2)
	require.Equal(t, 404, denied.Code)
	require.Equal(t, int32(1), calls.Load())
	resolved := seedanceCall(r, "GET", "/api/assets/upload-task", "", 1)
	require.Equal(t, 200, resolved.Code, resolved.Body.String())
	bindings, err := model.FindAssetBindings(1, "asset", "finished-asset")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "upload-task", bindings[0].CanonicalID)
}
