package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func registerTgxConversionTestRoutes(t *testing.T, r *gin.Engine) {
	r.POST("/", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayArkAssetAction)
	p, ok := configurable.GetProfile("seedance-tgxmaas")
	require.True(t, ok)
	for _, resource := range p.Resources {
		for _, endpoint := range resource.Aliases {
			path := strings.NewReplacer("{id}", ":id", "{group_id}", ":group_id").Replace(endpoint.Path)
			r.Handle(endpoint.Method, path, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
		}
	}
}

func TestTgxAssetConversionActionsAndGeneric(t *testing.T) {
	for _, official := range []bool{true, false} {
		t.Run(map[bool]string{true: "official", false: "generic"}[official], func(t *testing.T) {
			var method, path string
			var body []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				method, path = r.Method, r.URL.Path
				body, _ = io.ReadAll(r.Body)
				require.Equal(t, "Bearer fixture-key", r.Header.Get("Authorization"))
				_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local","upstream_asset_id":"asset-official"},"extension":9007199254740993}`))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "fixture-key", tgxRegressionModel)
			registerTgxConversionTestRoutes(t, r)
			cases := []struct{ action, method, path, upstream, body string }{
				{"CreateAsset", "POST", "/api/assets", "/v1/private-avatar/assets", `{"group_id":"ag_local","url":"https://example.com/a.png","asset_type":"Image","name":"a","extension":9007199254740993}`},
				{"ListAssets", "GET", "/api/assets", "/v1/private-avatar/assets/list", `{"Filter":{"GroupIds":["ag_local"]},"MaxResults":20}`},
				{"GetAsset", "GET", "/api/assets/asset_local", "/v1/private-avatar/assets/asset_local", `{"Id":"asset_local"}`},
				{"UpdateAsset", "PATCH", "/api/assets/asset_local", "/v1/private-avatar/assets/asset_local", `{"Id":"asset_local","Name":"renamed"}`},
				{"DeleteAsset", "DELETE", "/api/assets/asset_local", "/v1/private-avatar/assets/asset_local", `{"Id":"asset_local"}`},
				{"CreateAssetGroup", "POST", "/api/asset-groups", "/v1/private-avatar/groups", `{"name":"group","description":""}`},
				{"ListAssetGroups", "GET", "/api/asset-groups", "/v1/private-avatar/groups/list", `{"Filter":{"GroupType":"AIGC"}}`},
				{"GetAssetGroup", "GET", "/api/asset-groups/ag_local", "/v1/private-avatar/groups/ag_local", `{"Id":"ag_local"}`},
				{"UpdateAssetGroup", "PATCH", "/api/asset-groups/ag_local", "/v1/private-avatar/groups/ag_local", `{"Id":"ag_local","Name":"new group"}`},
				{"DeleteAssetGroup", "DELETE", "/api/asset-groups/ag_local", "/v1/private-avatar/groups/ag_local", `{"Id":"ag_local"}`},
			}
			for _, tc := range cases {
				t.Run(tc.action, func(t *testing.T) {
					requestMethod, requestPath := tc.method, tc.path
					if official {
						requestMethod = "POST"
						requestPath = "/?Action=" + tc.action + "&Version=2024-01-01"
					}
					response := seedanceCall(r, requestMethod, requestPath, tc.body, 1)
					require.Equal(t, 200, response.Code, response.Body.String())
					require.Equal(t, tc.upstream, path)
					wantMethod := tc.method
					if strings.HasPrefix(tc.action, "List") {
						wantMethod = "POST"
					}
					require.Equal(t, wantMethod, method)
					require.Equal(t, "asset-official", gjson.GetBytes(response.Body.Bytes(), "Result.upstream_asset_id").String())
					require.Contains(t, response.Body.String(), "9007199254740993")
					if wantMethod == "POST" || wantMethod == "PATCH" {
						require.Equal(t, tgxRegressionModel, gjson.GetBytes(body, "model").String())
						if tc.action == "CreateAsset" {
							require.Equal(t, "ag_local", gjson.GetBytes(body, "GroupId").String())
							require.Equal(t, "9007199254740993", gjson.GetBytes(body, "extension").Raw)
						}
						if strings.HasPrefix(tc.action, "List") {
							require.True(t, gjson.GetBytes(body, "Filter").IsObject())
						}
						if official {
							require.False(t, gjson.GetBytes(body, "Id").Exists())
						}
					}
				})
			}
			response := seedanceCall(r, "GET", "/api/assets?max_results=20&next_token=abc%2B123&filter=%7B%22GroupIds%22%3A%5B%22ag_local%22%5D%7D", "", 1)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Equal(t, int64(20), gjson.GetBytes(body, "MaxResults").Int())
			require.Equal(t, "abc+123", gjson.GetBytes(body, "NextToken").String())
			require.Equal(t, "ag_local", gjson.GetBytes(body, "Filter.GroupIds.0").String())
		})
	}
}

func TestTgxAssetConversionRejectsInvalidAndUnauthorized(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{"Result":{}}`)) }))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "fixture-key", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	for _, tc := range []struct{ path, body string }{
		{"/?Action=Unknown&Version=2024-01-01", `{}`},
		{"/?Action=GetAsset&Version=2024-01-01", `{"Id":"../other"}`},
		{"/?Action=GetAsset&Version=2024-01-01", `{}`},
		{"/?Action=CreateAssetGroup&Version=wrong", `{}`},
		{"/?Action=CreateAssetGroup&Version=2024-01-01", `{"ProjectName":"private"}`},
		{"/?Action=ListAssets&Version=2024-01-01", `{"PageNumber":2,"PageSize":10}`},
		{"/api/assets", `{"name":"a","Name":"b"}`},
	} {
		w := seedanceCall(r, "POST", tc.path, tc.body, 1)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	require.Zero(t, calls)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": tgxRegressionModel}).Error)
	for _, path := range []string{"/?Action=CreateAssetGroup&Version=2024-01-01", "/api/asset-groups"} {
		w := seedanceCall(r, "POST", path, `{"Name":"group"}`, 1)
		require.Equal(t, 403, w.Code, w.Body.String())
		w = seedanceCall(r, "POST", path, `{"Name":"group","model":"`+tgxRegressionModel+`"}`, 1)
		require.Equal(t, 200, w.Code, w.Body.String())
	}
	require.Equal(t, 2, calls)
	w := seedanceCall(r, "GET", "/api/assets?model=forbidden", `{"model":"`+tgxRegressionModel+`"}`, 1)
	require.Equal(t, 400, w.Code, w.Body.String())
	require.Equal(t, 2, calls, "query must not override the model authorized from the body")
}
