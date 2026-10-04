package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func hanxingtuAssetTestRouter(t *testing.T, baseURL, auth string) *gin.Engine {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", "hanxingtu-test-only-secret")
	r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "asset-test-key", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	profile, ok := configurable.GetProfile(configurable.HanxingtuAssetBackend)
	require.True(t, ok)
	for _, resource := range profile.Resources {
		r.Handle(resource.Public.Method, resource.Public.Path, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	}
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
		ProfileID: "doubao-seedance-2", ProjectName: "separate-video-project",
		AssetLibrary: &relaydto.AssetLibrarySettings{Backend: configurable.HanxingtuAssetBackend, BaseURL: baseURL, AuthMode: auth},
	}})
	if auth == "api_key" {
		ch.Key = "unusable-video-key"
		require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "asset-test-key"}))
	}
	require.NoError(t, validateAssetLibrary(ch, nil))
	require.NoError(t, model.DB.Save(ch).Error)
	return r
}

// Wire contracts are copied from Hanxingtu's seven public API examples.
// Each lifecycle traverses TokenAuth, ownership and the real relay handler.
func TestHanxingtuAssetLifecycle(t *testing.T) {
	cases := []struct{ action, method, generic, body, wire, response string }{
		{"CreateAssetGroup", "POST", "/api/asset-groups", `{"Name":"人物","Description":"AI generated character","GroupType":"AIGC"}`, `{"Name":"人物","Description":"AI generated character","GroupType":"AIGC"}`, `{"Id":"group-owned"}`},
		{"ListAssetGroups", "GET", "/api/asset-groups", `{"Filter":{"Name":"人物"}}`, `{"Filter":{"Name":"人物"},"MaxResults":100}`, `{"Items":[{"Id":"group-foreign"},{"Id":"group-owned","Name":"人物"}]}`},
		{"CreateAsset", "POST", "/api/assets", `{"GroupId":"group-owned","URL":"https://example.com/character.png","AssetType":"Image","Name":"character"}`, `{"GroupId":"group-owned","URL":"https://example.com/character.png","AssetType":"Image","Name":"character"}`, `{"Id":"asset-owned"}`},
		{"GetAsset", "GET", "/api/assets/asset-owned", `{"Id":"asset-owned"}`, `{"Id":"asset-owned"}`, `{"Id":"asset-owned","GroupId":"group-owned","Status":"Active","ProjectName":"default","AssetType":"Image"}`},
		{"ListAssets", "GET", "/api/assets", `{"Filter":{"GroupIds":["group-owned"]}}`, `{"Filter":{"GroupIds":["group-owned"]},"MaxResults":100}`, `{"Items":[{"Id":"asset-foreign"},{"Id":"asset-owned","Status":"Active"}]}`},
		{"DeleteAsset", "DELETE", "/api/assets/asset-owned", `{"Id":"asset-owned"}`, `{"Id":"asset-owned"}`, `{}`},
		{"DeleteAssetGroup", "DELETE", "/api/asset-groups/group-owned", `{"Id":"group-owned"}`, `{"Id":"group-owned"}`, `{}`},
	}
	for _, auth := range []string{"channel_key", "api_key"} {
		for _, entry := range []string{"native", "generic", "official"} {
			t.Run(auth+"/"+entry, func(t *testing.T) {
				var current, calls int
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					tc := cases[current]
					require.Equal(t, "POST", r.Method)
					require.Equal(t, "/prefix/v1/asset/"+tc.action, r.URL.Path)
					require.Empty(t, r.URL.RawQuery)
					require.Equal(t, "Bearer asset-test-key", r.Header.Get("Authorization"))
					require.Equal(t, "application/json", r.Header.Get("Content-Type"))
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					require.JSONEq(t, tc.wire, string(body))
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.response))
				}))
				defer upstream.Close()
				r := hanxingtuAssetTestRouter(t, upstream.URL+"/prefix/", auth)
				for i, tc := range cases {
					current = i
					method, path, body := "POST", "/v1/asset/"+tc.action, tc.body
					if entry == "generic" {
						method, path = tc.method, tc.generic
						body = strings.NewReplacer(`"Name":`, `"name":`, `"Description":`, `"description":`, `"GroupType":`, `"group_type":`, `"GroupId":`, `"group_id":`, `"AssetType":`, `"asset_type":`, `"URL":`, `"url":`, `"Id":`, `"id":`).Replace(body)
						// Filter uses the Ark schema, including its nested field names.
						body = strings.ReplaceAll(body, `"Filter":{"name":`, `"Filter":{"Name":`)
						if method == "GET" {
							query := url.Values{"model": {tgxRegressionModel}}
							gjson.Parse(body).ForEach(func(k, v gjson.Result) bool { query.Set(k.String(), v.String()); return true })
							path, body = path+"?"+query.Encode(), ""
						}
					} else if entry == "official" {
						path = "/?Action=" + tc.action + "&Version=2024-01-01"
					}
					if body != "" {
						body = body[:len(body)-1] + `,"model":"` + tgxRegressionModel + `"}`
					}
					w := seedanceCall(r, method, path, body, 1)
					require.Equal(t, 200, w.Code, "%s: %s", tc.action, w.Body.String())
					require.NotContains(t, w.Body.String(), "foreign")
					result := w.Body.String()
					if entry == "official" {
						result = gjson.Get(result, "Result").Raw
						require.Equal(t, tc.action, gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Action").String())
					}
					if strings.HasPrefix(tc.action, "List") {
						require.Len(t, gjson.Get(result, "Items").Array(), 1)
					} else {
						require.JSONEq(t, tc.response, result)
					}
					if tc.action == "CreateAsset" {
						before := calls
						denied := seedanceCall(r, "POST", "/v1/asset/GetAsset", `{"Id":"asset-owned"}`, 2)
						require.Equal(t, 404, denied.Code)
						require.Equal(t, before, calls, "another user must not reach the supplier")
					}
				}
				require.Equal(t, 7, calls)
				w := seedanceCall(r, "POST", "/v1/asset/GetAsset", `{"Id":"asset-owned"}`, 1)
				require.Equal(t, 404, w.Code, "successful deletion revokes the handle")
				require.Equal(t, 7, calls)
			})
		}
	}
}

func TestHanxingtuAssetPagination(t *testing.T) {
	for _, kind := range []string{"asset", "group"} {
		t.Run(kind, func(t *testing.T) {
			var calls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.Equal(t, int64(100), gjson.GetBytes(body, "MaxResults").Int())
				require.False(t, gjson.GetBytes(body, "PageNumber").Exists())
				require.False(t, gjson.GetBytes(body, "PageSize").Exists())
				if kind == "asset" {
					require.Equal(t, `["group-owned"]`, gjson.GetBytes(body, "Filter.GroupIds").Raw)
				}
				w.Header().Set("Content-Type", "application/json")
				if gjson.GetBytes(body, "NextToken").String() == "" {
					_, _ = w.Write([]byte(`{"Items":[{"Id":"foreign"}],"NextToken":"private-upstream-cursor"}`))
				} else {
					require.Equal(t, "private-upstream-cursor", gjson.GetBytes(body, "NextToken").String())
					_, _ = w.Write([]byte(`{"Items":[{"Id":"owned-1"},{"Id":"owned-2"}]}`))
				}
			}))
			defer upstream.Close()
			r := hanxingtuAssetTestRouter(t, upstream.URL, "api_key")
			seedAssetOwnershipForTest(t, 20, 1, kind, "owned-1")
			seedAssetOwnershipForTest(t, 20, 1, kind, "owned-2")
			path, prefix := "/v1/asset/ListAssetGroups", `{`
			if kind == "asset" {
				seedAssetOwnershipForTest(t, 20, 1, "group", "group-owned")
				path, prefix = "/v1/asset/ListAssets", `{"Filter":{"GroupIds":["group-owned"]},`
			}
			call := func(suffix string) string {
				body := strings.TrimSuffix(prefix+suffix, ",") + `}`
				w := seedanceCall(r, "POST", path, body, 1)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.NotContains(t, w.Body.String(), "foreign")
				require.NotContains(t, w.Body.String(), "private-upstream-cursor")
				return w.Body.String()
			}
			all := call("") // Even omitted pagination must scan the supplier's cursor.
			require.Len(t, gjson.Get(all, "Items").Array(), 2)
			require.False(t, gjson.Get(all, "NextToken").Exists())
			first := call(`"MaxResults":1`)
			require.Equal(t, "owned-1", gjson.Get(first, "Items.0.Id").String())
			next := gjson.Get(first, "NextToken").String()
			require.True(t, strings.HasPrefix(next, "asset-v1-"))
			second := call(fmt.Sprintf(`"MaxResults":1,"NextToken":%q`, next))
			require.Equal(t, "owned-2", gjson.Get(second, "Items.0.Id").String())
			require.False(t, gjson.Get(second, "NextToken").Exists())
			page := call(`"PageNumber":2,"PageSize":1`)
			require.Equal(t, "owned-2", gjson.Get(page, "Items.0.Id").String())
			require.Equal(t, 8, calls)
			bad := seedanceCall(r, "POST", path, prefix+`"NextToken":"private-upstream-cursor"}`, 1)
			require.Equal(t, 400, bad.Code)
			require.Equal(t, 8, calls)
		})
	}
}

func TestHanxingtuAssetErrorsAndUnsupportedOperations(t *testing.T) {
	var calls int
	status := 200
	response := `{"error":{"code":"SupplierFailure","message":"retry later"}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	defer upstream.Close()
	r := hanxingtuAssetTestRouter(t, upstream.URL, "api_key")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "asset-owned")
	seedAssetOwnershipForTest(t, 20, 1, "group", "group-owned")
	seedAssetOwnershipForTest(t, 20, 1, "group", "group-other")
	for _, code := range []int{200, 400, 401, 403, 429, 500} {
		status = code
		before := calls
		w := seedanceCall(r, "POST", "/v1/asset/DeleteAsset", `{"Id":"asset-owned"}`, 1)
		require.Equal(t, code, w.Code, w.Body.String())
		require.JSONEq(t, response, w.Body.String())
		require.Equal(t, "5", w.Header().Get("Retry-After"))
		require.Equal(t, before+1, calls, "never replay an asset operation")
	}
	before := calls
	for _, action := range []string{"GetAssetGroup", "UpdateAssetGroup", "UpdateAsset", "CreateVisualValidateSession"} {
		body := `{"Id":"unknown-handle"}`
		if action == "CreateVisualValidateSession" {
			body = `{}`
		}
		w := seedanceCall(r, "POST", "/?Action="+action+"&Version=2024-01-01", body, 1)
		require.Equal(t, 501, w.Code, "%s: %s", action, w.Body.String())
	}
	for _, tc := range []struct{ action, body string }{
		{"CreateAssetGroup", `{"Name":"human","GroupType":"REAL_PERSON"}`},
		{"CreateAssetGroup", `{"Name":"invalid-project","ProjectName":"video-project"}`},
		{"ListAssets", `{"Filter":{"GroupIds":["group-owned","group-other"]}}`},
		{"ListAssets", `{"Filter":{"groupids":["unowned-group"]}}`},
		{"ListAssets", `{}`},
		{"CreateAsset", `{"GroupId":"group-owned","URL":"https://example.com/a","AssetType":"Unknown"}`},
	} {
		w := seedanceCall(r, "POST", "/v1/asset/"+tc.action, tc.body, 1)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	require.Equal(t, before, calls)
	// Failed deletions keep ownership; a later success can still delete it.
	status, response = 200, `{}`
	w := seedanceCall(r, "POST", "/v1/asset/DeleteAsset", `{"Id":"asset-owned"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, before+1, calls)
}

func TestHanxingtuAssetHistoricalProjectAndMediaTypes(t *testing.T) {
	var mediaType string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.False(t, gjson.GetBytes(body, "ProjectName").Exists())
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1/asset/GetAsset" {
			require.JSONEq(t, `{"Id":"historical"}`, string(body))
			_, _ = w.Write([]byte(`{"Id":"historical","ProjectName":"default","Status":"Active"}`))
			return
		}
		require.Equal(t, mediaType, gjson.GetBytes(body, "AssetType").String())
		_, _ = fmt.Fprintf(w, `{"Id":"asset-%s"}`, mediaType)
	}))
	defer upstream.Close()
	r := hanxingtuAssetTestRouter(t, upstream.URL, "api_key")
	w := seedanceCall(r, "POST", "/v1/asset/GetAsset", `{"Id":"historical"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	bindings, err := model.FindAssetBindings(1, "asset", "historical")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "default", bindings[0].Project)
	seedAssetOwnershipForTest(t, 20, 1, "group", "group-owned")
	for _, mediaType = range []string{"", "Image", "Video", "Audio"} {
		body := `{"GroupId":"group-owned","URL":"https://example.com/media"}`
		if mediaType != "" {
			body = body[:len(body)-1] + fmt.Sprintf(`,"AssetType":%q}`, mediaType)
		}
		w = seedanceCall(r, "POST", "/v1/asset/CreateAsset", body, 1)
		require.Equal(t, 200, w.Code, w.Body.String())
	}
}
