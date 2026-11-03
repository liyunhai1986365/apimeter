package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func yhAssetTestRouter(t *testing.T, base string) *gin.Engine {
	r := tgxMaasResourceTestRouter(t, base, "yh-test-key", tgxRegressionModel)
	registerTgxConversionTestRoutes(t, r)
	r.POST("/v1/yh/assets/uploads", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	r.GET("/v1/yh/assets/uploads/:id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	r.GET("/api/v3/contents/generations/tasks", middleware.TokenAuth(), ListYHVideoTasks)
	r.GET("/v1/assets/:id/content", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-yh", AssetLibrary: &dto.AssetLibrarySettings{Backend: "yh", AuthMode: "channel_key"}}})
	require.NoError(t, ch.Update())
	return r
}

func yhAction(r http.Handler, action, body string, user int) *httptest.ResponseRecorder {
	return seedanceCall(r, "POST", "/?Action="+action+"&Version=2024-01-01", body, user)
}

func TestYHGroupDescriptionPassthrough(t *testing.T) {
	for _, tc := range []struct{ field, value string }{
		{"Description", "素材组描述"},
		{"description", "group description"},
		{"desc", "short description alias"},
		{"Description", ""},
	} {
		t.Run(tc.field+"/"+tc.value, func(t *testing.T) {
			router := yhAssetTestRouter(t, "https://yh-description-test.invalid/v1")
			client := service.GetHttpClient()
			oldTransport := client.Transport
			t.Cleanup(func() { client.Transport = oldTransport })
			calls := 0
			client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				require.Equal(t, http.MethodPost, req.Method)
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				want := map[string]any{"name": "group", "description": tc.value}
				switch calls {
				case 1:
					require.Equal(t, "/v1/assets/groups", req.URL.Path)
				case 2:
					require.Equal(t, "/v1/assets/groups/update", req.URL.Path)
					want["id"] = 42
				default:
					t.Fatalf("unexpected upstream call %d", calls)
				}
				encoded, err := common.Marshal(want)
				require.NoError(t, err)
				require.JSONEq(t, string(encoded), string(body))
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"code":"success","data":{"group_id":42}}`)), Request: req}, nil
			})
			created := yhAction(router, "CreateAssetGroup", fmt.Sprintf(`{"Name":"group","GroupType":"AIGC",%q:%q}`, tc.field, tc.value), 1)
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			require.Equal(t, "42", gjson.GetBytes(created.Body.Bytes(), "Result.Id").String())
			updated := yhAction(router, "UpdateAssetGroup", fmt.Sprintf(`{"Id":"42","Name":"group",%q:%q}`, tc.field, tc.value), 1)
			require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
			require.Equal(t, 2, calls)
			for _, payload := range []string{`{"Name":"group","Description":123}`, `{"Name":"group","Description":"one","desc":"two"}`} {
				denied := yhAction(router, "CreateAssetGroup", payload, 1)
				require.Equal(t, http.StatusBadRequest, denied.Code, denied.Body.String())
				require.Equal(t, 2, calls, "invalid or ambiguous descriptions must not reach upstream")
			}
		})
	}
}

func TestYHAssetBusinessValidationUsesUpstream(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-validation-test.invalid/v1")
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	var wantPath, wantBody string
	calls := 0
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		status, response := http.StatusOK, `{"code":"success","data":{"group_id":42}}`
		if req.URL.Path != "/v1/assets/groups" {
			require.Equal(t, wantPath, req.URL.Path)
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.JSONEq(t, wantBody, string(body))
			status, response = http.StatusUnprocessableEntity, `{"code":"InvalidParameter","message":"provider validation failed"}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
	})
	created := yhAction(router, "CreateAssetGroup", `{"Name":"owned"}`, 1)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	manyURLs, err := common.Marshal(strings.Fields(strings.Repeat("https://example.com/a.png ", 51)))
	require.NoError(t, err)
	for _, tc := range []struct{ name, path, body, upstreamPath, upstreamBody string }{
		{"asset_type", "/?Action=CreateAsset&Version=2024-01-01", `{"GroupId":"42","URL":"https://example.com/a.png","AssetType":"FutureMedia"}`, "/v1/assets/async", `{"group_id":42,"url":"https://example.com/a.png","asset_type":"FutureMedia"}`},
		{"upload_mode", "/v1/yh/assets/uploads", `{"group_id":"42","url":"https://example.com/a.png","upload_mode":"future_mode"}`, "/v1/assets", `{"group_id":42,"url":"https://example.com/a.png","upload_mode":"future_mode"}`},
		{"batch_sync", "/v1/yh/assets/uploads", `{"group_id":"42","urls":["https://example.com/a.png"],"upload_mode":"sync"}`, "/v1/assets", `{"group_id":42,"urls":["https://example.com/a.png"],"upload_mode":"sync"}`},
		{"missing_url", "/v1/yh/assets/uploads", `{"group_id":"42"}`, "/v1/assets", `{"group_id":42}`},
		{"empty_url", "/v1/yh/assets/uploads", `{"group_id":"42","url":""}`, "/v1/assets", `{"group_id":42,"url":""}`},
		{"empty_batch", "/v1/yh/assets/uploads", `{"group_id":"42","urls":[]}`, "/v1/assets", `{"group_id":42,"urls":[]}`},
		{"empty_batch_url", "/v1/yh/assets/uploads", `{"group_id":"42","urls":[""]}`, "/v1/assets", `{"group_id":42,"urls":[""]}`},
		{"large_batch", "/v1/yh/assets/uploads", `{"group_id":"42","urls":` + string(manyURLs) + `}`, "/v1/assets", `{"group_id":42,"urls":` + string(manyURLs) + `}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantPath, wantBody = tc.upstreamPath, tc.upstreamBody
			before := calls
			response := seedanceCall(router, http.MethodPost, tc.path, tc.body, 1)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "provider validation failed")
			require.Equal(t, before+1, calls, "provider business rules must be evaluated upstream")
		})
	}
}

func TestYHUploadAcceptsUpstreamBatchSize(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-batch-test.invalid/v1")
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		response := []byte(`{"code":"success","data":{"group_id":42}}`)
		if req.URL.Path != "/v1/assets/groups" {
			require.Contains(t, []string{"/v1/assets", "/v1/assets/batch/get"}, req.URL.Path)
			items := make([]map[string]any, 51)
			for i := range items {
				items[i] = map[string]any{"id": "task-batch", "status": "Processing"}
				if req.URL.Path == "/v1/assets/batch/get" {
					items[i] = map[string]any{"id": fmt.Sprintf("asset-batch-%d", i), "status": "Active", "group_id": 42}
				}
			}
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			if req.URL.Path == "/v1/assets" {
				require.Len(t, gjson.GetBytes(body, "urls").Array(), 51)
			} else {
				require.JSONEq(t, `{"batch_id":"task-batch"}`, string(body))
			}
			response, err = common.Marshal(map[string]any{"code": "success", "data": map[string]any{"batch_id": "task-batch", "items": items}})
			require.NoError(t, err)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(response))), Request: req}, nil
	})
	created := yhAction(router, "CreateAssetGroup", `{"Name":"owned"}`, 1)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	body, err := common.Marshal(map[string]any{"group_id": "42", "urls": strings.Fields(strings.Repeat("https://example.com/a.png ", 51))})
	require.NoError(t, err)
	upload := seedanceCall(router, http.MethodPost, "/v1/yh/assets/uploads", string(body), 1)
	require.Equal(t, http.StatusOK, upload.Code, upload.Body.String())
	require.Equal(t, "Processing", gjson.GetBytes(upload.Body.Bytes(), "Status").String())
	require.Len(t, gjson.GetBytes(upload.Body.Bytes(), "Items").Array(), 51)
	poll := seedanceCall(router, http.MethodGet, "/v1/yh/assets/uploads/task-batch", "", 1)
	require.Equal(t, http.StatusOK, poll.Code, poll.Body.String())
	require.Equal(t, "Active", gjson.GetBytes(poll.Body.Bytes(), "Status").String())
	require.Len(t, gjson.GetBytes(poll.Body.Bytes(), "Items").Array(), 51)
	owned, err := model.FindAssetBindings(1, "asset", "asset-batch-50")
	require.NoError(t, err)
	require.Len(t, owned, 1)
	foreign, err := model.FindAssetBindings(2, "asset", "asset-batch-50")
	require.NoError(t, err)
	require.Empty(t, foreign)
}

func TestYHOfficialAssetLifecycle(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "Bearer yh-test-key", r.Header.Get("Authorization"))
		body, _ := io.ReadAll(r.Body)
		var response string
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/assets/groups":
			require.JSONEq(t, `{"name":"mine"}`, string(body))
			response = `{"code":"success","data":{"group_id":42,"group_name":"private-account-prefix-mine"}}`
		case "GET /v1/assets/groups":
			response = `{"code":"success","data":[{"id":999,"name":"foreign"},{"id":42,"name":"mine"}]}`
		case "GET /v1/assets/real-person/groups":
			response = `{"code":"success","data":{"items":[]}}`
		case "POST /v1/assets/async":
			require.JSONEq(t, `{"url":"https://example.com/a.png","group_id":42,"name":"asset","asset_type":"Image"}`, string(body))
			response = `{"code":"success","data":{"id":"asset-owned","group_id":42,"status":"Pending"}}`
		case "POST /v1/assets/get":
			require.JSONEq(t, `{"id":"asset-owned"}`, string(body))
			response = `{"code":"success","data":{"id":"asset-owned","group_id":42,"status":"Active","asset_type":"Image","url":"https://example.com/a.png"}}`
		case "GET /v1/assets/asset-owned/content":
			require.Equal(t, "bytes=0-2", r.Header.Get("Range"))
			require.Equal(t, "etag", r.Header.Get("If-Range"))
			w.Header().Set("Content-Range", "bytes 0-2/10")
			w.Header().Set("Set-Cookie", "upstream-private=1")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("abc"))
			return
		case "POST /v1/assets/list":
			require.Equal(t, int64(100), gjson.GetBytes(body, "page_size").Int())
			require.False(t, gjson.GetBytes(body, "PageSize").Exists())
			response = `{"code":"success","data":{"items":[{"id":"foreign","status":"Active"},{"id":"asset-owned","status":"Active"}],"total_count":2}}`
		case "POST /v1/assets/update":
			response = `{"code":"success","data":{"id":"asset-owned","name":"renamed"}}`
		case "POST /v1/assets/groups/update":
			response = `{"code":"success","data":{"id":42,"group_name":"private-account-name"}}`
		case "POST /v1/assets/delete", "POST /v1/assets/groups/delete":
			response = `{"code":"success","message":"deleted"}`
		default:
			t.Errorf("unexpected upstream request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			response = `{}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL+"/v1/")
	check := func(action, body string) *httptest.ResponseRecorder {
		w := yhAction(r, action, body, 1)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "private-account")
		return w
	}
	created := check("CreateAssetGroup", `{"Name":"mine","GroupType":"AIGC"}`)
	require.Equal(t, "42", gjson.GetBytes(created.Body.Bytes(), "Result.Id").String())
	for _, action := range []string{"ListAssetGroups", "GetAssetGroup"} {
		body := `{}`
		if action == "GetAssetGroup" {
			body = `{"Id":"42"}`
		}
		w := check(action, body)
		require.NotContains(t, w.Body.String(), "foreign")
	}
	created = check("CreateAsset", `{"GroupId":"42","URL":"https://example.com/a.png","Name":"asset","AssetType":"Image"}`)
	require.Equal(t, "Processing", gjson.GetBytes(created.Body.Bytes(), "Result.Status").String())
	check("GetAsset", `{"Id":"asset-owned"}`)
	query := seedanceCall(r, "GET", "/v1/assets/get?id=asset-owned", "", 1)
	require.Equal(t, 200, query.Code, query.Body.String())
	for _, user := range []int{1, 2} {
		req := httptest.NewRequest("GET", "/v1/assets/asset-owned/content", nil)
		req.Header.Set("Authorization", fmt.Sprintf("Bearer seedancetest%d", user))
		req.Header.Set("Range", "bytes=0-2")
		req.Header.Set("If-Range", "etag")
		out := httptest.NewRecorder()
		r.ServeHTTP(out, req)
		if user == 1 {
			require.Equal(t, 206, out.Code, out.Body.String())
			require.Equal(t, "abc", out.Body.String())
			require.Equal(t, "bytes 0-2/10", out.Header().Get("Content-Range"))
			require.Equal(t, "private, no-store", out.Header().Get("Cache-Control"))
			require.Empty(t, out.Header().Get("Set-Cookie"))
		} else {
			require.Equal(t, 404, out.Code)
		}
	}
	for _, payload := range []string{`{"Id":"asset-owned","URL":"https://example.com/new.png"}`, `{"Id":"asset-owned","Description":"ignored?"}`} {
		before := calls.Load()
		denied := yhAction(r, "UpdateAsset", payload, 1)
		require.Equal(t, 400, denied.Code, denied.Body.String())
		require.Equal(t, before, calls.Load())
	}
	w := check("ListAssets", `{"MaxResults":1}`)
	require.Equal(t, int64(1), gjson.GetBytes(w.Body.Bytes(), "Result.TotalCount").Int())
	require.NotContains(t, w.Body.String(), "foreign")
	for _, body := range []string{`{"Id":"asset-owned"}`, `{"Id":"not-registered"}`} {
		before := calls.Load()
		denied := yhAction(r, "GetAsset", body, 2)
		require.Equal(t, 404, denied.Code, denied.Body.String())
		require.Equal(t, before, calls.Load())
	}
	check("UpdateAsset", `{"Id":"asset-owned","Name":"renamed"}`)
	check("UpdateAssetGroup", `{"Id":"42","Name":"renamed"}`)
	check("DeleteAsset", `{"Id":"asset-owned"}`)
	require.Equal(t, 404, yhAction(r, "GetAsset", `{"Id":"asset-owned"}`, 1).Code)
	check("DeleteAssetGroup", `{"Id":"42"}`)
}

func TestYHGroupListFilter(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-group-filter-test.invalid/v1")
	for _, id := range []string{"5525", "5526", "5527", "5528"} {
		seedAssetOwnershipForTest(t, 20, 1, "group", id)
	}
	seedAssetOwnershipForTest(t, 20, 2, "group", "999")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	human := model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope,
		Kind: "group", CanonicalID: "128", GroupType: "LivenessFace", Project: "default"}
	for _, id := range []string{"128", "group-human"} {
		require.NoError(t, model.SaveAssetBinding(human, id))
	}
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	calls := 0
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, http.MethodGet, req.Method)
		require.Empty(t, req.URL.Query().Get("group_id"), "group-list filtering happens locally")
		response := ""
		switch req.URL.Path {
		case "/v1/assets/groups":
			response = `{"code":"success","data":[{"id":999,"name":"foreign"},{"id":5527,"name":"other"},{"id":5525,"name":"match-a"},{"id":5526,"name":"match-b"},{"id":998,"name":"unowned"}]}`
		case "/v1/assets/real-person/groups":
			response = `{"code":"success","data":{"items":[{"id":128,"group_id":"group-human","artist_name":"match-human"}]}}`
		default:
			t.Fatalf("unexpected upstream request %s", req.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
	})
	for _, tc := range []struct {
		name, body string
		ids        []string
		total      int
	}{
		{"reported_request", `{"Filter":{"GroupType":"AIGC","GroupIds":["5525"]},"PageNumber":1,"PageSize":100}`, []string{"5525"}, 1},
		{"multiple_ids", `{"Filter":{"GroupIds":["5525","5526"]}}`, []string{"5525", "5526"}, 2},
		{"pagination_after_filter", `{"Filter":{"GroupIds":["5525","5526"]},"PageNumber":2,"PageSize":1}`, []string{"5526"}, 2},
		{"name_intersection", `{"Filter":{"GroupIds":["5525","5526"],"Name":"match-b"}}`, []string{"5526"}, 1},
		{"type_intersection", `{"Filter":{"GroupIds":["5525","128"],"GroupType":"AIGC"}}`, []string{"5525"}, 1},
		{"type_mismatch", `{"Filter":{"GroupIds":["5525"],"GroupType":"LivenessFace"}}`, nil, 0},
		{"missing_upstream_group", `{"Filter":{"GroupIds":["5528"]}}`, nil, 0},
		{"human_alias", `{"Filter":{"GroupIds":["group-human"],"GroupType":"LivenessFace"}}`, []string{"128"}, 1},
		{"alias_deduplication", `{"Filter":{"GroupIds":["group-human","128","128"]}}`, []string{"128"}, 1},
		{"no_id_filter", `{}`, []string{"5527", "5525", "5526", "128"}, 4},
		{"empty_id_filter", `{"Filter":{"GroupIds":[]}}`, []string{"5527", "5525", "5526", "128"}, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := yhAction(router, "ListAssetGroups", tc.body, 1)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var ids []string
			for _, item := range gjson.GetBytes(w.Body.Bytes(), "Result.Items").Array() {
				ids = append(ids, item.Get("Id").String())
			}
			require.Equal(t, tc.ids, ids)
			require.EqualValues(t, tc.total, gjson.GetBytes(w.Body.Bytes(), "Result.TotalCount").Int())
			require.NotContains(t, w.Body.String(), "foreign")
			require.NotContains(t, w.Body.String(), "unowned")
		})
	}
	for _, tc := range []struct {
		name, action, body string
		user, status       int
	}{
		{"foreign_group", "ListAssetGroups", `{"Filter":{"GroupIds":["999"]}}`, 1, 404},
		{"mixed_ownership", "ListAssetGroups", `{"Filter":{"GroupIds":["5525","999"]}}`, 1, 404},
		{"other_user", "ListAssetGroups", `{"Filter":{"GroupIds":["5525"]}}`, 2, 404},
		{"invalid_group_id_type", "ListAssetGroups", `{"Filter":{"GroupIds":[5525]}}`, 1, 400},
		{"invalid_filter_type", "ListAssetGroups", `{"Filter":{"GroupIds":"5525"}}`, 1, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls
			w := yhAction(router, tc.action, tc.body, tc.user)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Equal(t, before, calls, "rejected filters must not reach upstream")
		})
	}
}

func TestYHListPageAlias(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-page-alias-test.invalid/v1")
	router.POST("/v1/assets/list", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	for _, id := range []string{"42", "43"} {
		seedAssetOwnershipForTest(t, 20, 1, "group", id)
	}
	for _, id := range []string{"asset-first", "asset-second"} {
		seedAssetListAliases(t, id)
	}
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	calls := 0
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		response := ""
		switch req.URL.Path {
		case "/v1/assets/list":
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.JSONEq(t, `{"page_number":1,"page_size":100}`, string(body), "client pagination must be applied after ownership filtering")
			response = `{"code":"success","data":{"items":[{"id":"foreign","status":"Active"},{"id":"asset-first","status":"Active"},{"id":"asset-second","status":"Active"}],"total_count":3}}`
		case "/v1/assets/groups":
			response = `{"code":"success","data":[{"id":999,"name":"foreign"},{"id":42,"name":"first"},{"id":43,"name":"second"}]}`
		case "/v1/assets/real-person/groups":
			response = `{"code":"success","data":[]}`
		default:
			t.Fatalf("unexpected upstream request %s", req.URL.Path)
		}
		return yhHTTPResponse(req, http.StatusOK, []byte(response), nil), nil
	})
	for _, tc := range []struct{ method, path, body, id string }{
		{"GET", "/api/assets?page=2&page_size=1", "", "asset-second"},
		{"GET", "/v1/assets?page=2&page_size=1", "", "asset-second"},
		{"GET", "/api/asset-groups?page=2&page_size=1", "", "43"},
		{"GET", "/v1/asset-groups?page=2&page_size=1", "", "43"},
		{"POST", "/v1/assets/list", `{"page":2,"page_size":1}`, "asset-second"},
		{"POST", "/?Action=ListAssetGroups&Version=2024-01-01", `{"page":2,"page_size":1}`, "43"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			response := seedanceCall(router, tc.method, tc.path, tc.body, 1)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			path, items, err := assetListItems(response.Body.Bytes())
			require.NoError(t, err)
			require.Len(t, items.Array(), 1)
			require.Equal(t, tc.id, items.Array()[0].Get("Id").String())
			totalPath := strings.TrimSuffix(path, "Items") + "TotalCount"
			require.EqualValues(t, 2, gjson.GetBytes(response.Body.Bytes(), totalPath).Int())
		})
	}
	for _, path := range []string{"/v1/assets", "/v1/asset-groups"} {
		for _, page := range []string{"0", "-1", "1.5", "invalid"} {
			before := calls
			response := seedanceCall(router, http.MethodGet, path+"?page="+page, "", 1)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			require.Equal(t, before, calls, "invalid pagination must fail before contacting upstream")
		}
	}
}

func TestYHGroupListTypeIsolation(t *testing.T) {
	for i, tc := range []struct {
		name, groupType                         string
		aigcStatus, humanStatus, responseStatus int
		aigcCalls, humanCalls                   int
		ids                                     []string
	}{
		{"aigc_ignores_human_failure", "AIGC", 200, 503, 200, 1, 0, []string{"42", "43"}},
		{"human_ignores_aigc_failure", "LivenessFace", 503, 200, 200, 0, 1, []string{"128"}},
		{"aigc_failure_propagates", "AIGC", 503, 200, 503, 1, 0, nil},
		{"human_failure_propagates", "LivenessFace", 200, 503, 503, 0, 1, nil},
		{"all_groups", "", 200, 200, 200, 1, 1, []string{"42", "43", "128"}},
		{"all_groups_human_failure", "", 200, 503, 503, 1, 1, nil},
		{"all_groups_aigc_failure", "", 503, 200, 503, 1, 0, nil},
	} {
		for _, official := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/official=%t", tc.name, official), func(t *testing.T) {
				router := yhAssetTestRouter(t, fmt.Sprintf("https://yh-type-%d-%t.invalid/v1", i, official))
				for _, id := range []string{"42", "43"} {
					seedAssetOwnershipForTest(t, 20, 1, "group", id)
				}
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
				scope, err := assetAccountScope(ch, profile)
				require.NoError(t, err)
				require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope,
					Kind: "group", CanonicalID: "128", GroupType: "LivenessFace", Project: "default"}, "128"))
				client := service.GetHttpClient()
				oldTransport := client.Transport
				t.Cleanup(func() { client.Transport = oldTransport })
				aigcCalls, humanCalls := 0, 0
				client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
					require.Equal(t, http.MethodGet, req.Method)
					status, response := 200, ""
					switch req.URL.Path {
					case "/v1/assets/groups":
						aigcCalls++
						status = tc.aigcStatus
						response = `{"code":"success","data":[{"id":42,"name":"first"},{"id":43,"name":"second"},{"id":999,"name":"foreign"}]}`
					case "/v1/assets/real-person/groups":
						humanCalls++
						status = tc.humanStatus
						response = `{"code":"success","data":{"items":[{"id":128,"artist_name":"human"},{"id":998,"artist_name":"foreign"}]}}`
					default:
						t.Fatalf("unexpected upstream request %s", req.URL.Path)
					}
					if status >= 400 {
						response = `{"code":"Unavailable","message":"selected group service unavailable"}`
					}
					return yhHTTPResponse(req, status, []byte(response), nil), nil
				})
				filter := `{}`
				if tc.groupType != "" {
					filter = fmt.Sprintf(`{"GroupType":%q}`, tc.groupType)
				}
				var response *httptest.ResponseRecorder
				if official {
					response = yhAction(router, "ListAssetGroups", `{"Filter":`+filter+`}`, 1)
				} else {
					response = seedanceCall(router, http.MethodGet, "/v1/asset-groups?Filter="+url.QueryEscape(filter), "", 1)
				}
				require.Equal(t, tc.responseStatus, response.Code, response.Body.String())
				require.Equal(t, tc.aigcCalls, aigcCalls)
				require.Equal(t, tc.humanCalls, humanCalls)
				if response.Code == http.StatusOK {
					path, items, err := assetListItems(response.Body.Bytes())
					require.NoError(t, err)
					var ids []string
					for _, item := range items.Array() {
						ids = append(ids, item.Get("Id").String())
						if tc.groupType != "" {
							require.Equal(t, tc.groupType, item.Get("GroupType").String())
						}
					}
					require.Equal(t, tc.ids, ids)
					totalPath := strings.TrimSuffix(path, "Items") + "TotalCount"
					require.EqualValues(t, len(tc.ids), gjson.GetBytes(response.Body.Bytes(), totalPath).Int())
				}
			})
		}
	}
}

func TestYHAssetListWithPageLocalTotal(t *testing.T) {
	for _, count := range []int{200, 201} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				require.Equal(t, "/v1/assets/list", r.URL.Path)
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				page := int(gjson.GetBytes(body, "page_number").Int())
				require.Positive(t, page)
				require.EqualValues(t, 100, gjson.GetBytes(body, "page_size").Int())
				rows := make([]string, 0, 100)
				for i := (page - 1) * 100; i < min(page*100, count); i++ {
					id := fmt.Sprintf("foreign-%d", i)
					if i == 0 {
						id = "asset-first"
					} else if i == count-1 {
						id = "asset-last"
					}
					rows = append(rows, fmt.Sprintf(`{"id":%q,"status":"Active"}`, id))
				}
				w.Header().Set("Content-Type", "application/json")
				// Live YH responses report this page's size as total_count.
				_, _ = fmt.Fprintf(w, `{"code":"success","data":{"items":[%s],"total_count":%d}}`, strings.Join(rows, ","), len(rows))
			}))
			defer upstream.Close()
			r := yhAssetTestRouter(t, upstream.URL)
			for _, id := range []string{"asset-first", "asset-last", "asset-filtered-out"} {
				seedAssetListAliases(t, id)
			}
			w := yhAction(r, "ListAssets", `{"PageNumber":2,"PageSize":1}`, 1)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.EqualValues(t, 2, gjson.GetBytes(w.Body.Bytes(), "Result.TotalCount").Int())
			require.Len(t, gjson.GetBytes(w.Body.Bytes(), "Result.Items").Array(), 1)
			require.Equal(t, "asset-last", gjson.GetBytes(w.Body.Bytes(), "Result.Items.0.Id").String())
			require.NotContains(t, w.Body.String(), "foreign-")
			require.NotContains(t, w.Body.String(), "asset-filtered-out")
			require.EqualValues(t, 3, calls.Load())
		})
	}
}

func TestYHH5SessionOwnershipAndCompletion(t *testing.T) {
	var name string
	var groups atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/assets/real-person/auth/link":
			name = gjson.GetBytes(body, "artist_name").String()
			require.Len(t, name, 8)
			_, _ = w.Write([]byte(`{"code":"success","data":{"h5_url":"https://example.com/h5?secret=private"}}`))
		case "/v1/assets/real-person/groups":
			groups.Add(1)
			response, _ := common.Marshal(map[string]any{"code": "success", "data": map[string]any{"items": []any{map[string]any{"id": 128, "group_id": "group-human", "artist_name": name, "authorized_at": time.Now().Format(time.RFC3339)}}}})
			_, _ = w.Write(response)
		case "/v1/assets/async":
			require.Equal(t, "128", gjson.GetBytes(body, "group_id").Raw)
			_, _ = w.Write([]byte(`{"code":"success","data":{"id":"asset-human","group_id":128,"status":"Pending"}}`))
		case "/v1/assets/get":
			_, _ = w.Write([]byte(`{"code":"success","data":{"id":"asset-human","group_id":128,"status":"Failed","error":{"code":"FaceMismatch","message":"different person"}}}`))
		case "/v1/assets/real-person/assets/delete":
			require.JSONEq(t, `{"id":"asset-human"}`, string(body))
			_, _ = w.Write([]byte(`{"code":"success"}`))
		case "/v1/assets/real-person/groups/delete":
			require.JSONEq(t, `{"id":128}`, string(body))
			_, _ = w.Write([]byte(`{"code":"success"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(500)
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL)
	bad := yhAction(r, "CreateVisualValidateSession", `{"CallbackURL":"https://example.com/callback"}`, 1)
	require.Equal(t, 400, bad.Code)
	w := yhAction(r, "CreateVisualValidateSession", `{}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	token := gjson.GetBytes(w.Body.Bytes(), "Result.BytedToken").String()
	require.True(t, strings.HasPrefix(token, "yh-"))
	query := fmt.Sprintf(`{"BytedToken":%q}`, token)
	denied := yhAction(r, "GetVisualValidateResult", query, 2)
	require.Equal(t, 404, denied.Code)
	require.Zero(t, groups.Load())
	for i := 0; i < 2; i++ {
		w = yhAction(r, "GetVisualValidateResult", query, 1)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "128", gjson.GetBytes(w.Body.Bytes(), "Result.GroupId").String())
	}
	require.Equal(t, int32(1), groups.Load(), "completed sessions reuse their persisted result")
	var states []model.ConfigurableResourceState
	require.NoError(t, model.DB.Find(&states).Error)
	for _, state := range states {
		require.NotContains(t, state.Metadata, token)
		require.NotContains(t, state.Metadata, "secret=private")
	}
	w = yhAction(r, "CreateAsset", `{"GroupId":"group-human","URL":"https://example.com/person.png","AssetType":"Image"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = yhAction(r, "GetAsset", `{"Id":"asset-human"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "FaceMismatch", gjson.GetBytes(w.Body.Bytes(), "Result.Error.Code").String(), w.Body.String())
	w = yhAction(r, "DeleteAsset", `{"Id":"asset-human"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = yhAction(r, "DeleteAssetGroup", `{"Id":"128"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	w = yhAction(r, "GetVisualValidateResult", query, 1)
	require.Equal(t, 404, w.Code, w.Body.String())
	require.Equal(t, int32(1), groups.Load())
}
