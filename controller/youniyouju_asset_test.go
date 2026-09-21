package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Requests follow the public Postman collection 211593/2sBYAvvqRA. The
// independent upstream checks the wire protocol, including naked liveness
// responses, rather than reproducing the backend's field mapping in a mock.
func TestYouniyoujuAssetActions(t *testing.T) {
	cases := []struct{ action, method, path, body, response string }{
		{"CreateAssetGroup", "POST", "/api/asset-groups", `{"Name":"reference","Description":"","GroupType":"AIGC"}`, `{"Result":{"Id":"group-original"}}`},
		{"ListAssetGroups", "GET", "/api/asset-groups", `{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":20}`, `{"Result":{"Items":[{"Id":"group-original"}]}}`},
		{"GetAssetGroup", "GET", "/api/asset-groups/group-original", `{"Id":"group-original"}`, `{"Result":{"Id":"group-original"}}`},
		{"UpdateAssetGroup", "PATCH", "/api/asset-groups/group-original", `{"Id":"group-original","Name":"renamed"}`, `{"Result":{}}`},
		{"DeleteAssetGroup", "DELETE", "/api/asset-groups/group-original", `{"Id":"group-original"}`, `{"Result":{}}`},
		{"CreateAsset", "POST", "/api/assets", `{"URL":"https://example.com/reference.png","AssetType":"Image","Name":"reference"}`, `{"Result":{"Id":"asset-original","Status":"Processing"}}`},
		{"ListAssets", "GET", "/api/assets", `{"Filter":{"GroupType":"AIGC"},"PageNumber":1,"PageSize":20}`, `{"Result":{"Items":[{"Id":"asset-original","Status":"Active"}]}}`},
		{"GetAsset", "GET", "/api/assets/asset-original", `{"Id":"asset-original"}`, `{"Result":{"Id":"asset-original","Status":"Active"}}`},
		{"UpdateAsset", "PATCH", "/api/assets/asset-original", `{"Id":"asset-original","Name":"renamed"}`, `{"Result":{}}`},
		{"DeleteAsset", "DELETE", "/api/assets/asset-original", `{"Id":"asset-original"}`, `{"Result":{}}`},
		{"CreateVisualValidateSession", "POST", "/v1/real-avatar/auth/session", `{"callback_url":"https://client.example/callback"}`, `{"BytedToken":"session-token","H5Link":"https://provider.example/auth","CallbackURL":"https://client.example/callback"}`},
		{"GetVisualValidateResult", "POST", "/v1/real-avatar/groups/from-token", `{"BytedToken":"session-token"}`, `{"GroupId":"group-human"}`},
	}
	for _, auth := range []string{"channel_key", "api_key"} {
		for _, entry := range []string{"action", "generic"} {
			t.Run(auth+"/"+entry, func(t *testing.T) {
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
					require.Equal(t, "/api/volcengine_asset", r.URL.Path)
					require.Equal(t, tc.action, r.URL.Query().Get("Action"))
					require.Equal(t, "2024-01-01", r.URL.Query().Get("Version"))
					require.Len(t, r.URL.Query(), 2)
					require.Equal(t, "Bearer "+wantKey, r.Header.Get("Authorization"))
					require.Empty(t, r.Header.Get("X-Date"), "must not use AK/SK signing")
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					want := tc.body[:len(tc.body)-1] + `,"extension":{"zero":0,"enabled":false,"integer":9007199254740993}}`
					if strings.HasPrefix(tc.action, "List") {
						want = strings.ReplaceAll(want, `"PageSize":20`, `"PageSize":100`)
					}
					require.JSONEq(t, want, string(body), "must retain original IDs and omit gateway routing fields and synthetic groups/projects")
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.response))
				}))
				defer upstream.Close()
				// An independent asset URL must not send requests to the video URL.
				r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "video-key", tgxRegressionModel)
				registerTgxConversionTestRoutes(t, r)
				r.POST(configurable.YouniyoujuAssetPath, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayArkAssetAction)
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
					ProfileID:    "doubao-seedance-2",
					AssetLibrary: &relaydto.AssetLibrarySettings{Backend: configurable.YouniyoujuAssetBackend, AuthMode: auth, BaseURL: upstream.URL + "/"},
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
						if entry == "action" {
							method = "POST"
							path = "/api/volcengine_asset?Action=" + tc.action + "&Version=2024-01-01"
						}
						body := tc.body[:len(tc.body)-1] + `,"model":"` + tgxRegressionModel + `","extension":{"zero":0,"enabled":false,"integer":9007199254740993}}`
						response := seedanceCall(r, method, path, body, 1)
						require.Equal(t, http.StatusOK, response.Code, response.Body.String())
						require.Equal(t, tc.response, response.Body.String())
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

func TestYouniyoujuAssetErrorsAndRouting(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "/api/volcengine_asset", r.URL.Path)
		require.Equal(t, "Bearer video-key", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "group-human", gjson.GetBytes(body, "GroupId").String())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"RateLimit","message":"try later"}}`))
	}))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "video-key", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	r.POST(configurable.YouniyoujuAssetPath, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayArkAssetAction)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "doubao-seedance-2", AssetLibrary: &relaydto.AssetLibrarySettings{Backend: configurable.YouniyoujuAssetBackend, AuthMode: "channel_key"}}})
	require.NoError(t, model.DB.Save(ch).Error)
	for _, path := range []string{
		"/api/volcengine_asset?Action=Unknown&Version=2024-01-01",
		"/api/volcengine_asset?Action=CreateAsset&Version=wrong",
		"/api/volcengine_asset?Action=GetAsset&Version=2024-01-01",
	} {
		response := seedanceCall(r, "POST", path, `{}`, 1)
		require.Equal(t, http.StatusBadRequest, response.Code)
	}
	unauthorized := httptest.NewRecorder()
	r.ServeHTTP(unauthorized, httptest.NewRequest("POST", "/api/volcengine_asset?Action=CreateAsset&Version=2024-01-01", nil))
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	require.Zero(t, calls)
	seedAssetOwnershipForTest(t, 20, 1, "group", "group-human")
	// Existing Action callers may keep using the root endpoint. Missing asset
	// Base URL reuses the video channel address, with one POST and no replay.
	response := seedanceCall(r, "POST", "/?Action=CreateAsset&Version=2024-01-01", `{"group_id":"group-human","URL":"https://example.com/human.png","AssetType":"Image"}`, 1)
	require.Equal(t, http.StatusTooManyRequests, response.Code)
	require.Equal(t, "5", response.Header().Get("Retry-After"))
	require.JSONEq(t, `{"error":{"code":"RateLimit","message":"try later"}}`, response.Body.String())
	require.Equal(t, 1, calls)
}
