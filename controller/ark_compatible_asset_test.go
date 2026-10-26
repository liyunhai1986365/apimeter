package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestArkCompatiblePaginationAndScope(t *testing.T) {
	for _, mode := range []string{"page", "cursor"} {
		t.Run(mode, func(t *testing.T) {
			calls, failPage := 0, false
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				require.Equal(t, "/api", r.URL.Path)
				require.Equal(t, "ListAssets", r.URL.Query().Get("Action"))
				require.Equal(t, "asset_manager_intl", r.URL.Query().Get("ns"))
				require.Equal(t, "Bearer account-a", r.Header.Get("Authorization"))
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.False(t, gjson.GetBytes(body, "ProjectName").Exists())
				second := false
				if mode == "cursor" {
					require.Equal(t, int64(100), gjson.GetBytes(body, "MaxResults").Int())
					require.False(t, gjson.GetBytes(body, "PageNumber").Exists())
					second = gjson.GetBytes(body, "NextToken").String() == "supplier-next"
				} else {
					require.Equal(t, int64(100), gjson.GetBytes(body, "PageSize").Int())
					require.False(t, gjson.GetBytes(body, "MaxResults").Exists())
					second = gjson.GetBytes(body, "PageNumber").Int() == 2
				}
				w.Header().Set("Content-Type", "application/json")
				if second && failPage {
					w.Header().Set("Retry-After", "5")
					w.WriteHeader(http.StatusTooManyRequests)
					_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"Throttled","Message":"later"}}}`))
				} else if second {
					_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"own-1"},{"Id":"own-2"}],"TotalCount":3}}`))
				} else {
					_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"foreign"}],"TotalCount":3,"NextToken":"supplier-next"}}`))
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, configurable.ArkCompatibleAssetBackend, "")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			settings := ch.GetSetting()
			cfg := &relaydto.ArkCompatibleAssetSettings{Pagination: mode, FixedQuery: map[string]string{"ns": "asset_manager_intl"}}
			settings.Protocol.AssetLibrary.ArkCompatible = cfg
			ch.SetSetting(settings)
			require.NoError(t, model.DB.Save(ch).Error)
			for _, id := range []string{"own-1", "own-2"} {
				seedAssetOwnershipForTest(t, 20, 1, "asset", id)
			}
			seedAssetOwnershipForTest(t, 20, 2, "asset", "foreign")
			first := assetSecurityAction(r, "ListAssets", `{"MaxResults":1}`, 1)
			require.Equal(t, http.StatusOK, first.Code, first.Body.String())
			require.Equal(t, 2, calls)
			require.Equal(t, "own-1", gjson.GetBytes(first.Body.Bytes(), "Result.Items.0.Id").String())
			require.NotContains(t, first.Body.String(), "foreign")
			cursor := gjson.GetBytes(first.Body.Bytes(), "Result.NextToken").String()
			require.NotEmpty(t, cursor)
			require.NotEqual(t, "supplier-next", cursor)
			next := assetSecurityAction(r, "ListAssets", fmt.Sprintf(`{"MaxResults":1,"NextToken":%q}`, cursor), 1)
			require.Equal(t, http.StatusOK, next.Code, next.Body.String())
			require.Equal(t, "own-2", gjson.GetBytes(next.Body.Bytes(), "Result.Items.0.Id").String())
			// A later-page failure must not become a successful partial list.
			failPage = true
			failed := assetSecurityAction(r, "ListAssets", `{"PageNumber":1,"PageSize":20}`, 1)
			require.Equal(t, http.StatusTooManyRequests, failed.Code, failed.Body.String())
			require.Equal(t, "5", failed.Header().Get("Retry-After"))
			require.NotContains(t, failed.Body.String(), "Items")
			before := calls
			profile, _ := configurable.AssetProfile(settings.Protocol)
			scope, err := assetAccountScope(ch, profile)
			require.NoError(t, err)
			ch.Key = "rotated-key"
			rotatedScope, err := assetAccountScope(ch, profile)
			require.NoError(t, err)
			require.Equal(t, scope, rotatedScope)
			cfg.FixedQuery["ns"] = "asset_manager"
			ch.SetSetting(settings)
			require.NoError(t, model.DB.Save(ch).Error)
			denied := assetSecurityAction(r, "ListAssets", fmt.Sprintf(`{"MaxResults":1,"NextToken":%q}`, cursor), 1)
			require.Equal(t, http.StatusBadRequest, denied.Code, denied.Body.String())
			denied = assetSecurityAction(r, "GetAsset", `{"Id":"own-1"}`, 1)
			require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
			require.Equal(t, before, calls)
			cfg.FixedQuery["ns"] = "asset_manager_intl"
			for _, change := range []func(){func() { cfg.EndpointPath = "/other" }, func() { cfg.EndpointPath = ""; cfg.APIVersion = "2025-01-01" }} {
				change()
				ch.SetSetting(settings)
				changed, err := assetAccountScope(ch, profile)
				require.NoError(t, err)
				require.NotEqual(t, scope, changed)
			}
		})
	}
}

func TestArkCompatibleUpstreamProjectAndLegacyLookup(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/custom/assets", r.URL.Path)
		require.Equal(t, "asset_manager", r.URL.Query().Get("ns"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(body, "ProjectName").Exists())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Id":"asset-owned","ProjectName":"supplier-project","Status":"active"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, configurable.ArkCompatibleAssetBackend, "")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.AssetLibrary.ArkCompatible = &relaydto.ArkCompatibleAssetSettings{EndpointPath: "/custom/assets", FixedQuery: map[string]string{"ns": "asset_manager"}}
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	for _, tc := range []struct {
		action, body string
		status       int
	}{
		{"CreateAsset", `{"ProjectName":"untrusted-project","URL":"https://example.com/a.mp4"}`, http.StatusBadRequest},
		{"CreateVisualValidateSession", `{}`, http.StatusNotImplemented},
		// Unknown session handles are rejected by ownership checks before routing.
		{"GetVisualValidateResult", `{"BytedToken":"unknown"}`, http.StatusNotFound},
	} {
		response := assetSecurityAction(r, tc.action, tc.body, 1)
		require.Equal(t, tc.status, response.Code, response.Body.String())
	}
	require.Zero(t, calls)
	// First access verifies the historical handle through the same endpoint.
	response := assetSecurityAction(r, "GetAsset", `{"Id":"asset-owned"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Positive(t, calls)
	bindings, _, err := model.FindAssetAccessBindingsContext(t.Context(), 1, "asset", "asset-owned", false)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "supplier-project", bindings[0].Project)
	before := calls
	response = assetSecurityAction(r, "GetAsset", `{"Id":"asset-owned"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, before+1, calls)
	response = assetSecurityAction(r, "GetAsset", `{"Id":"asset-owned"}`, 2)
	require.Equal(t, http.StatusNotFound, response.Code)
	require.Equal(t, before+1, calls)
}

func TestArkCompatibleVideoReference(t *testing.T) {
	assetSecurityRouter(t, "https://video.example", configurable.ArkCompatibleAssetBackend, "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "original-id")
	for _, user := range []int{1, 2} {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"image":"asset://original-id"}`))
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "default")
		info := &relaycommon.RelayInfo{UserId: user, OriginModelName: tgxRegressionModel, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
		err := lockAssetVideoChannel(ctx, info)
		if user == 1 {
			require.NoError(t, err)
			require.Equal(t, 20, info.LockedChannel.(*model.Channel).Id)
			require.Empty(t, info.AssetAliases, "original IDs must remain unchanged")
		} else {
			require.ErrorIs(t, err, model.ErrAssetNotOwned)
		}
	}
}

// Contract from MoreRouter doc0022, exercised through both public entry formats.
func TestArkCompatibleAssetActions(t *testing.T) {
	cases := []struct{ action, method, path, body, response string }{
		{"CreateAssetGroup", "POST", "/api/asset-groups", `{"Name":"reference","Description":"","GroupType":"AIGC"}`, `{"Result":{"Id":"group-original"}}`},
		{"ListAssetGroups", "GET", "/api/asset-groups", `{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":20}`, `{"Result":{"Items":[{"Id":"group-original"}]}}`},
		{"GetAssetGroup", "GET", "/api/asset-groups/group-original", `{"Id":"group-original"}`, `{"Result":{"Id":"group-original"}}`},
		{"UpdateAssetGroup", "PATCH", "/api/asset-groups/group-original", `{"Id":"group-original","Name":"renamed"}`, `{"Result":{}}`},
		{"DeleteAssetGroup", "DELETE", "/api/asset-groups/group-original", `{"Id":"group-original"}`, `{"Result":{}}`},
		{"CreateAsset", "POST", "/api/assets", `{"URL":"https://example.com/reference.mp4","AssetType":"video","Name":"reference"}`, `{"Result":{"Id":"asset-original","Status":"processing"}}`},
		{"ListAssets", "GET", "/api/assets", `{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":20}`, `{"Result":{"Items":[{"Id":"asset-original","Status":"active"}]}}`},
		{"GetAsset", "GET", "/api/assets/asset-original", `{"Id":"asset-original"}`, `{"Result":{"Id":"asset-original","Status":"active"}}`},
		{"UpdateAsset", "PATCH", "/api/assets/asset-original", `{"Id":"asset-original","Name":"renamed"}`, `{"Result":{}}`},
		{"DeleteAsset", "DELETE", "/api/assets/asset-original", `{"Id":"asset-original"}`, `{"Result":{}}`},
	}
	for _, namespace := range []string{"asset_manager", "asset_manager_intl"} {
		for _, auth := range []string{"channel_key", "api_key"} {
			for _, entry := range []string{"generic", "official"} {
				t.Run(namespace+"/"+auth+"/"+entry, func(t *testing.T) {
					t.Setenv("CRYPTO_SECRET", "test-only-persistent-secret")
					var current, calls int
					wantKey := "video-key"
					if auth == "api_key" {
						wantKey = "asset-key"
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls++
						tc := cases[current]
						require.Equal(t, "POST", r.Method)
						require.Equal(t, "/prefix/api", r.URL.Path)
						require.Equal(t, tc.action, r.URL.Query().Get("Action"))
						require.Equal(t, "2024-01-01", r.URL.Query().Get("Version"))
						require.Len(t, r.URL.Query(), 3)
						require.Equal(t, namespace, r.URL.Query().Get("ns"))
						require.Equal(t, "Bearer "+wantKey, r.Header.Get("Authorization"))
						require.Empty(t, r.Header.Get("X-Date"), "must not use AK/SK signing")
						body, err := io.ReadAll(r.Body)
						require.NoError(t, err)
						want := tc.body[:len(tc.body)-1] + `,"extension":{"zero":0,"enabled":false,"integer":9007199254740993}}`
						if strings.HasPrefix(tc.action, "List") {
							want = strings.ReplaceAll(want, `"PageSize":20`, `"PageSize":100`)
						}
						require.Equal(t, "9007199254740993", gjson.GetBytes(body, "extension.integer").Raw)
						require.JSONEq(t, want, string(body), "must retain original IDs and omit gateway routing fields and synthetic groups/projects")
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(tc.response))
					}))
					defer upstream.Close()
					// An independent asset URL must not send requests to the video URL.
					r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "video-key", tgxRegressionModel)
					registerTgxConversionTestRoutes(t, r)
					ch, err := model.GetChannelById(20, true)
					require.NoError(t, err)
					ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
						ProfileID: "doubao-seedance-2", ProjectName: "video-project-must-not-leak",
						AssetLibrary: &relaydto.AssetLibrarySettings{Backend: configurable.ArkCompatibleAssetBackend, AuthMode: auth, BaseURL: upstream.URL + "/prefix/", ArkCompatible: &relaydto.ArkCompatibleAssetSettings{FixedQuery: map[string]string{"ns": namespace}}},
					}})
					if auth == "api_key" {
						require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "asset-key"}))
					}
					require.NoError(t, validateAssetLibrary(ch, nil))
					require.NoError(t, model.DB.Save(ch).Error)
					// Old TgxMaas handle mappings must not rewrite this provider's IDs.
					require.NoError(t, model.SaveTgxMaasAssetHandle(20, 1, ch.AssetHandleProject(""), "asset-original", "wrong-provider-id"))
					for i, tc := range cases {
						current = i
						t.Run(tc.action, func(t *testing.T) {
							method, path := tc.method, tc.path
							if entry == "official" {
								method = "POST"
								path = "/?Action=" + tc.action + "&Version=2024-01-01&ns=client-must-not-override"
							}
							body := tc.body[:len(tc.body)-1] + `,"model":"` + tgxRegressionModel + `","extension":{"zero":0,"enabled":false,"integer":9007199254740993}}`
							response := seedanceCall(r, method, path, body, 1)
							require.Equal(t, http.StatusOK, response.Code, response.Body.String())
							if entry == "official" {
								want := gjson.Get(tc.response, "Result").Raw
								// The gateway's official entry normalizes status casing.
								want = strings.NewReplacer(`"processing"`, `"Processing"`, `"active"`, `"Active"`).Replace(want)
								if strings.HasPrefix(tc.action, "List") {
									want = want[:len(want)-1] + `,"TotalCount":1,"PageNumber":1,"PageSize":20}`
								}
								require.JSONEq(t, want, gjson.GetBytes(response.Body.Bytes(), "Result").Raw)
								require.Equal(t, tc.action, gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.Action").String())
								require.NotEmpty(t, gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.RequestId").String())
							} else {
								require.Equal(t, tc.response, response.Body.String())
							}
						})
					}
					require.Equal(t, len(cases), calls)
					var tasks int64
					require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
					require.Zero(t, tasks)
				})
			}
		}
	}
}
