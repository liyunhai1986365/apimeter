package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
		{"/?Action=CreateAssetGroup&Version=2024-01-01", `{"ProjectName":123}`},
		{"/?Action=ListAssets&Version=2024-01-01", `{"PageNumber":2,"PageSize":10,"NextToken":"cursor"}`},
		{"/?Action=ListAssets&Version=2024-01-01", `{"PageNumber":0}`},
		{"/?Action=ListAssets&Version=2024-01-01", `{"PageSize":1.5}`},
		{"/?Action=ListAssets&Version=2024-01-01", `{"Filter":[]}`},
		{"/?Action=CreateAssetGroup&Version=2024-01-01", `{"ProjectName":null}`},
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

// Match the supplier guide's original handles, project scope and page-based
// lists through all three entry formats. Verify the actual upstream request.
func TestTgxAssetDocumentedProjectAndPagination(t *testing.T) {
	for _, mode := range []string{"official", "generic", "native"} {
		t.Run(mode, func(t *testing.T) {
			var gotMethod, gotPath, gotProject string
			var gotBody []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath, gotProject = r.Method, r.URL.Path, r.URL.Query().Get("ProjectName")
				gotBody, _ = io.ReadAll(r.Body)
				require.Empty(t, r.URL.Query().Get("Action"))
				require.Empty(t, r.URL.Query().Get("Version"))
				_, _ = w.Write([]byte(`{"Result":{"Id":"asset-20260916-original","GroupId":"group-20260916-original","ProjectName":"brand + cn","Status":"Active"}}`))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "fixture-key", tgxRegressionModel)
			registerTgxConversionTestRoutes(t, r)
			for _, kind := range []struct{ singular, plural, native, generic, id string }{
				{"Asset", "Assets", "assets", "assets", "asset-20260916-original"},
				{"AssetGroup", "AssetGroups", "groups", "asset-groups", "group-20260916-original"},
			} {
				for _, op := range []string{"Create", "List", "Get", "Update", "Delete"} {
					t.Run(op+kind.singular, func(t *testing.T) {
						body := map[string]any{"ProjectName": "brand + cn"}
						method, suffix, action := "POST", "", op+kind.singular
						switch op {
						case "Create":
							body["Name"] = "fixture"
							if kind.singular == "Asset" {
								body["GroupId"], body["URL"], body["AssetType"] = "group-20260916-original", "https://example.com/sample.mp4", "Video"
							} else {
								body["GroupType"] = "AIGC"
							}
						case "List":
							suffix, action = "/list", op+kind.plural
							body["PageNumber"], body["PageSize"] = 2, 10
							body["Filter"] = map[string]any{"GroupIds": []string{"group-20260916-original"}, "Name": "fixture"}
						case "Get", "Update", "Delete":
							method = map[string]string{"Get": "GET", "Update": "PATCH", "Delete": "DELETE"}[op]
							suffix = "/" + kind.id
							if op == "Update" {
								body["Name"] = "renamed"
							}
						}
						wantMethod, wantPath := method, "/v1/private-avatar/"+kind.native+suffix
						path := wantPath
						if mode == "generic" {
							path = "/v1/" + kind.generic + suffix
							if op == "List" {
								method, path = "GET", "/v1/"+kind.generic
							}
						} else if mode == "official" {
							method, path = "POST", "/?Action="+action+"&Version=2024-01-01"
							if op == "Get" || op == "Update" || op == "Delete" {
								body["Id"] = kind.id
							}
						}
						raw, err := common.Marshal(body)
						require.NoError(t, err)
						w := seedanceCall(r, method, path, string(raw), 1)
						require.Equal(t, 200, w.Code, w.Body.String())
						require.Equal(t, wantMethod, gotMethod)
						require.Equal(t, wantPath, gotPath)
						require.Equal(t, "brand + cn", gotProject)
						if wantMethod == "GET" {
							require.Empty(t, gotBody)
						} else {
							require.Equal(t, "brand + cn", gjson.GetBytes(gotBody, "ProjectName").String())
							if op == "List" {
								require.Equal(t, int64(2), gjson.GetBytes(gotBody, "PageNumber").Int())
								require.Equal(t, int64(10), gjson.GetBytes(gotBody, "PageSize").Int())
								require.Equal(t, "group-20260916-original", gjson.GetBytes(gotBody, "Filter.GroupIds.0").String())
							}
							if mode == "official" {
								require.False(t, gjson.GetBytes(gotBody, "Id").Exists())
							}
						}
						require.Equal(t, "asset-20260916-original", gjson.GetBytes(w.Body.Bytes(), "Result.Id").String())
					})
				}
			}
			for _, path := range []string{
				"/v1/private-avatar/assets/asset-20260916-original?ProjectName=brand+%2B+cn",
				"/api/assets/asset-20260916-original?project_name=brand+%2B+cn",
				"/v1/assets/get?asset_id=asset-20260916-original&project_name=brand+%2B+cn",
			} {
				w := seedanceCall(r, "GET", path, "", 1)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, "brand + cn", gotProject)
			}
			w := seedanceCall(r, "GET", "/api/assets?page_number=2&page_size=10&project_name=brand", "", 1)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Equal(t, int64(2), gjson.GetBytes(gotBody, "PageNumber").Int())
			require.Equal(t, "brand", gjson.GetBytes(gotBody, "ProjectName").String())
			// Omission must retain group-project inheritance rather than inject default.
			w = seedanceCall(r, "POST", "/api/assets", `{"group_id":"group-original","asset_type":"Audio","url":"https://example.com/a.mp3"}`, 1)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.False(t, gjson.GetBytes(gotBody, "ProjectName").Exists())
			require.Empty(t, gotProject)
		})
	}
}

func TestTgxAssetOriginalHandleLifecycle(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local","upstream_asset_id":"asset-original","ProjectName":"brand","Status":"Active"}}`))
	}))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "fixture-key", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	w := seedanceCall(r, "POST", "/?Action=CreateAsset&Version=2024-01-01", `{"GroupId":"ag_local","AssetType":"Image","URL":"https://example.com/a.png","ProjectName":"brand"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	// The mapping is in SQLite, not a request-local or process-global cache.
	var stored model.ConfigurableResourceState
	require.NoError(t, model.DB.Where("resource_id = ?", "asset_handle").First(&stored).Error)
	require.Equal(t, "asset_local", stored.StateValue)
	for _, action := range []string{"GetAsset", "UpdateAsset", "DeleteAsset"} {
		w = seedanceCall(r, "POST", "/?Action="+action+"&Version=2024-01-01", `{"Id":"asset-original","ProjectName":"brand","Name":"new"}`, 1)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "/v1/private-avatar/assets/asset_local", gotPath)
	}
}
