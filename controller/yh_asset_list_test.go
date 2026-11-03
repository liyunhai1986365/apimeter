package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func seedYHListAsset(t *testing.T, id, group string, aliases ...string) {
	t.Helper()
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	binding := model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope,
		Kind: "asset", CanonicalID: id, GroupID: group, Project: "default"}
	for _, handle := range append([]string{id}, aliases...) {
		require.NoError(t, model.SaveAssetBinding(binding, handle))
	}
}

func TestYHAssetListFilters(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-list-filter-test.invalid/v1")
	router.POST("/v1/assets/list", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	for _, id := range []string{"42", "43", "44"} {
		seedAssetOwnershipForTest(t, 20, 1, "group", id)
	}
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope,
		Kind: "group", CanonicalID: "43", Project: "default"}, "group-alias"))
	seedYHListAsset(t, "asset-a", "42", "alias-a")
	seedYHListAsset(t, "asset-b", "43")
	seedYHListAsset(t, "asset-c", "43")
	seedYHListAsset(t, "asset-d", "44")
	seedYHListAsset(t, "asset-gone", "42")
	seedAssetOwnershipForTest(t, 20, 2, "group", "99")
	seedAssetOwnershipForTest(t, 20, 2, "asset", "foreign")

	rows := []struct{ group, body string }{
		{"42", `{"id":"asset-a","name":"alpha","status":"Active"}`}, // YH often omits group_id.
		{"42", `{"id":"alias-a","name":"alpha","status":"Active"}`},
		{"42", `{"id":"foreign","name":"alpha","status":"Active"}`},
		{"43", `{"id":"asset-b","name":"beta","status":"Pending","group_id":43}`},
		{"43", `{"id":"asset-c","name":"alpha","status":"Failed"}`},
		{"44", `{"id":"asset-d","name":"delta","status":"Active","group_id":44}`},
	}
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	var calls atomic.Int32
	var wantGroup string
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		require.Equal(t, "/v1/assets/list", req.URL.Path)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		for key := range gjson.ParseBytes(body).Map() {
			require.Contains(t, []string{"name", "group_id", "page_number", "page_size"}, key)
		}
		group, name := gjson.GetBytes(body, "group_id").String(), gjson.GetBytes(body, "name").String()
		require.Equal(t, wantGroup, group)
		items := []string{}
		for _, row := range rows {
			if (group == "" || group == row.group) && strings.Contains(gjson.Get(row.body, "name").String(), name) {
				items = append(items, row.body)
			}
		}
		response := fmt.Sprintf(`{"code":"success","data":{"items":[%s],"total_count":%d}}`, strings.Join(items, ","), len(items))
		return yhHTTPResponse(req, http.StatusOK, []byte(response), nil), nil
	})
	for _, tc := range []struct {
		name, body, group string
		ids               []string
		total             int
	}{
		{"unfiltered", `{}`, "", []string{"asset-a", "asset-b", "asset-c", "asset-d"}, 4},
		{"empty_arrays", `{"Filter":{"GroupIds":[],"Statuses":[],"AssetIds":[],"Ids":[]}}`, "", []string{"asset-a", "asset-b", "asset-c", "asset-d"}, 4},
		{"active", `{"Filter":{"Statuses":["Active"]}}`, "", []string{"asset-a", "asset-d"}, 2},
		{"pending_normalization", `{"Filter":{"Statuses":["Processing"]}}`, "", []string{"asset-b"}, 1},
		{"multiple_statuses", `{"Filter":{"Statuses":["Processing","Failed"]}}`, "", []string{"asset-b", "asset-c"}, 2},
		{"no_status_match", `{"Filter":{"Statuses":["Deleted"]}}`, "", nil, 0},
		{"asset_ids", `{"Filter":{"AssetIds":["asset-a","asset-c"]}}`, "", []string{"asset-a", "asset-c"}, 2},
		{"ids_alias", `{"Filter":{"Ids":["alias-a","asset-a","asset-a"]}}`, "", []string{"asset-a"}, 1},
		{"asset_ids_alias", `{"Filter":{"AssetIds":["alias-a"]}}`, "", []string{"asset-a"}, 1},
		{"id_filters_intersect", `{"Filter":{"AssetIds":["asset-a","asset-c"],"Ids":["asset-c","asset-d"]}}`, "", []string{"asset-c"}, 1},
		{"disjoint_id_filters", `{"Filter":{"AssetIds":["asset-a"],"Ids":["asset-c"]}}`, "", nil, 0},
		{"missing_upstream_asset", `{"Filter":{"Ids":["asset-gone"]}}`, "", nil, 0},
		{"single_group", `{"Filter":{"GroupIds":["42"]}}`, "42", []string{"asset-a"}, 1},
		{"group_alias_dedup", `{"Filter":{"GroupIds":["group-alias","43"]}}`, "43", []string{"asset-b", "asset-c"}, 2},
		{"multiple_groups", `{"Filter":{"GroupIds":["42","group-alias"]}}`, "", []string{"asset-a", "asset-b", "asset-c"}, 3},
		{"multiple_groups_page_two", `{"Filter":{"GroupIds":["42","43"]},"PageNumber":2,"PageSize":2}`, "", []string{"asset-c"}, 3},
		{"combined", `{"Filter":{"GroupIds":["42","43"],"AssetIds":["asset-a","asset-b","asset-c","asset-d"],"Statuses":["Active","Failed"],"Name":"alpha"}}`, "", []string{"asset-a", "asset-c"}, 2},
		{"top_group", `{"GroupId":"42"}`, "42", []string{"asset-a"}, 1},
		{"top_group_with_empty_filter", `{"group_id":"42","Filter":{"GroupIds":[]}}`, "42", []string{"asset-a"}, 1},
		{"group_conditions_intersect", `{"GroupId":"42","Filter":{"GroupIds":["42","43"]}}`, "42", []string{"asset-a"}, 1},
		{"disjoint_group_conditions", `{"GroupId":"42","Filter":{"GroupIds":["43"]}}`, "42", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantGroup = tc.group
			response := yhAction(router, "ListAssets", tc.body, 1)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			var ids []string
			for _, row := range gjson.GetBytes(response.Body.Bytes(), "Result.Items").Array() {
				ids = append(ids, row.Get("Id").String())
			}
			require.Equal(t, tc.ids, ids)
			require.EqualValues(t, tc.total, gjson.GetBytes(response.Body.Bytes(), "Result.TotalCount").Int())
		})
	}
	wantGroup = ""
	for _, tc := range []struct{ method, path, body string }{
		{"POST", "/v1/assets/list", `{"filter":{"Statuses":["Failed"],"GroupIds":["42","43"]}}`},
		{"GET", "/api/assets?Filter=" + url.QueryEscape(`{"Statuses":["Failed"],"GroupIds":["42","43"]}`), ""},
	} {
		response := seedanceCall(router, tc.method, tc.path, tc.body, 1)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		_, items, err := assetListItems(response.Body.Bytes())
		require.NoError(t, err)
		require.Len(t, items.Array(), 1)
		require.Equal(t, "asset-c", items.Array()[0].Get("Id").String())
	}
	for _, tc := range []struct {
		body         string
		user, status int
	}{
		{`{"Filter":{"Statuses":"Active"}}`, 1, 400},
		{`{"Filter":{"Statuses":[1]}}`, 1, 400},
		{`{"Filter":{"Statuses":[""]}}`, 1, 400},
		{`{"Filter":{"AssetIds":"asset-a"}}`, 1, 400},
		{`{"Filter":{"Ids":[null]}}`, 1, 400},
		{`{"Filter":{"GroupIds":[42]}}`, 1, 400},
		{`{"Filter":{"AssetIds":["foreign"]}}`, 1, 404},
		{`{"Filter":{"Ids":["asset-a","foreign"]}}`, 1, 404},
		{`{"Filter":{"Ids":["unregistered"]}}`, 1, 404},
		{`{"Filter":{"GroupIds":["42","99"]}}`, 1, 404},
		{`{"Filter":{"GroupIds":["unknown-group"]}}`, 1, 404},
		{`{"Filter":{"Ids":["asset-a"]}}`, 2, 404},
	} {
		before := calls.Load()
		response := yhAction(router, "ListAssets", tc.body, tc.user)
		require.Equal(t, tc.status, response.Code, response.Body.String())
		require.Equal(t, before, calls.Load(), "invalid/unauthorized filters must not reach upstream")
	}
}

func TestYHAssetListFilteredPagination(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-filter-pages-test.invalid/v1")
	for _, id := range []string{"42", "43", "44"} {
		seedAssetOwnershipForTest(t, 20, 1, "group", id)
	}
	seedYHListAsset(t, "asset-early", "44")
	seedYHListAsset(t, "asset-a", "42")
	seedYHListAsset(t, "asset-b", "43")
	seedYHListAsset(t, "asset-gone", "42")
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	var calls atomic.Int32
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.EqualValues(t, 100, gjson.GetBytes(body, "page_size").Int())
		rows := []string{}
		switch gjson.GetBytes(body, "page_number").Int() {
		case 1:
			rows = append(rows, `{"id":"asset-early","status":"Failed"}`)
			for i := 1; i < 100; i++ {
				rows = append(rows, fmt.Sprintf(`{"id":"foreign-%d","status":"Active"}`, i))
			}
		case 2:
			rows = []string{`{"id":"asset-a","status":"Active"}`, `{"id":"asset-b","status":"Active"}`}
		default:
			t.Errorf("unexpected page: %s", body)
		}
		response := fmt.Sprintf(`{"code":"success","data":{"items":[%s],"total_count":%d}}`, strings.Join(rows, ","), len(rows))
		return yhHTTPResponse(req, http.StatusOK, []byte(response), nil), nil
	})
	for _, filter := range []string{`{"Statuses":["Active"]}`, `{"AssetIds":["asset-a","asset-b"]}`, `{"Ids":["asset-a","asset-b"]}`, `{"GroupIds":["42","43"]}`} {
		before := calls.Load()
		first := yhAction(router, "ListAssets", `{"Filter":`+filter+`,"MaxResults":1}`, 1)
		require.Equal(t, http.StatusOK, first.Code, first.Body.String())
		require.Equal(t, "asset-a", gjson.GetBytes(first.Body.Bytes(), "Result.Items.0.Id").String())
		require.EqualValues(t, 2, gjson.GetBytes(first.Body.Bytes(), "Result.TotalCount").Int())
		cursor := gjson.GetBytes(first.Body.Bytes(), "Result.NextToken").String()
		require.NotEmpty(t, cursor)
		for _, pagination := range []string{`"PageNumber":2,"PageSize":1`, fmt.Sprintf(`"MaxResults":1,"NextToken":%q`, cursor)} {
			second := yhAction(router, "ListAssets", `{"Filter":`+filter+`,`+pagination+`}`, 1)
			require.Equal(t, http.StatusOK, second.Code, second.Body.String())
			require.Equal(t, "asset-b", gjson.GetBytes(second.Body.Bytes(), "Result.Items.0.Id").String())
			require.EqualValues(t, 2, gjson.GetBytes(second.Body.Bytes(), "Result.TotalCount").Int())
			require.Empty(t, gjson.GetBytes(second.Body.Bytes(), "Result.NextToken").String())
		}
		require.EqualValues(t, 6, calls.Load()-before, "a full page with no matches must not truncate the scan")
	}
}

func TestYHAssetListStopsAfterRequestedIDs(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-id-scan-test.invalid/v1")
	for _, id := range []string{"42", "43", "44"} {
		seedAssetOwnershipForTest(t, 20, 1, "group", id)
	}
	seedYHListAsset(t, "asset-first", "42", "alias-first")
	seedYHListAsset(t, "asset-later", "43")
	seedYHListAsset(t, "asset-unrelated", "44")
	seedYHListAsset(t, "asset-missing", "42")
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	var calls atomic.Int32
	lastAvailablePage := 0
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		page := int(gjson.GetBytes(body, "page_number").Int())
		if page > lastAvailablePage {
			return yhHTTPResponse(req, 503, []byte(`{"code":"Unavailable","message":"later page failed"}`), nil), nil
		}
		rows := []string{}
		switch page {
		case 1:
			rows = []string{`{"id":"asset-first","status":"Active"}`, `{"id":"alias-first","status":"Active"}`}
		case 2:
			rows = []string{`{"id":"asset-later","status":"Failed"}`}
		case 3:
			rows = []string{`{"id":"asset-unrelated","status":"Active"}`}
		default:
			t.Errorf("unexpected page %d", page)
		}
		if page < 3 {
			for len(rows) < assetListBatchSize {
				rows = append(rows, fmt.Sprintf(`{"id":"foreign-%d-%d","status":"Active"}`, page, len(rows)))
			}
		}
		response := fmt.Sprintf(`{"code":"success","data":{"items":[%s],"total_count":%d}}`, strings.Join(rows, ","), len(rows))
		return yhHTTPResponse(req, http.StatusOK, []byte(response), nil), nil
	})
	for _, tc := range []struct {
		name, body                    string
		availablePages, status, total int
		ids                           []string
	}{
		{"ids", `{"Filter":{"Ids":["asset-first"]}}`, 1, 200, 1, []string{"asset-first"}},
		{"asset_ids", `{"Filter":{"AssetIds":["asset-first"]}}`, 1, 200, 1, []string{"asset-first"}},
		{"canonical_aliases", `{"Filter":{"Ids":["alias-first","asset-first","alias-first"]}}`, 1, 200, 1, []string{"asset-first"}},
		{"id_intersection", `{"Filter":{"Ids":["asset-first","asset-missing"],"AssetIds":["alias-first","asset-unrelated"]}}`, 1, 200, 1, []string{"asset-first"}},
		{"empty_intersection", `{"Filter":{"Ids":["asset-first"],"AssetIds":["asset-later"]}}`, 1, 200, 0, nil},
		{"empty_ids_with_asset_ids", `{"Filter":{"Ids":[],"AssetIds":["asset-first"]}}`, 1, 200, 1, []string{"asset-first"}},
		{"empty_asset_ids_with_ids", `{"Filter":{"Ids":["asset-first"],"AssetIds":[]}}`, 1, 200, 1, []string{"asset-first"}},
		{"status_excludes_target", `{"Filter":{"Ids":["asset-first"],"Statuses":["Processing"]}}`, 1, 200, 0, nil},
		{"groups_exclude_target", `{"Filter":{"Ids":["asset-first"],"GroupIds":["43","44"]}}`, 1, 200, 0, nil},
		{"later_target", `{"Filter":{"Ids":["asset-first","asset-later"]}}`, 2, 200, 2, []string{"asset-first", "asset-later"}},
		{"page_two", `{"Filter":{"Ids":["asset-first","asset-later"]},"PageNumber":2,"PageSize":1}`, 2, 200, 2, []string{"asset-later"}},
		{"status_excludes_later_target", `{"Filter":{"Ids":["asset-first","asset-later"],"Statuses":["Active"]}}`, 2, 200, 1, []string{"asset-first"}},
		{"missing_target", `{"Filter":{"Ids":["asset-first","asset-missing"]}}`, 3, 200, 1, []string{"asset-first"}},
		{"missing_target_page_failure", `{"Filter":{"Ids":["asset-first","asset-later"]}}`, 1, 503, 0, nil},
		{"no_id_filter", `{}`, 3, 200, 3, []string{"asset-first", "asset-later", "asset-unrelated"}},
		{"empty_id_filters", `{"Filter":{"Ids":[],"AssetIds":[]}}`, 3, 200, 3, []string{"asset-first", "asset-later", "asset-unrelated"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lastAvailablePage = tc.availablePages
			before := calls.Load()
			response := yhAction(router, "ListAssets", tc.body, 1)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			wantCalls := tc.availablePages
			if tc.status >= 400 {
				wantCalls++
			} else {
				var ids []string
				for _, row := range gjson.GetBytes(response.Body.Bytes(), "Result.Items").Array() {
					ids = append(ids, row.Get("Id").String())
				}
				require.Equal(t, tc.ids, ids)
				require.EqualValues(t, tc.total, gjson.GetBytes(response.Body.Bytes(), "Result.TotalCount").Int())
			}
			require.EqualValues(t, wantCalls, calls.Load()-before)
		})
	}
}

func TestYHAssetListNameConflict(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-name-conflict-test.invalid/v1")
	seedAssetListAliases(t, "asset-owned")
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	var calls atomic.Int32
	var sentName string
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		sentName = gjson.GetBytes(body, "name").String()
		return yhHTTPResponse(req, http.StatusOK, []byte(`{"code":"success","data":{"items":[{"id":"asset-owned","status":"Active"}],"total_count":1}}`), nil), nil
	})
	const actionPath = "/?Action=ListAssets&Version=2024-01-01"
	for _, tc := range []struct {
		name, method, path, body, upstreamName string
		status                                 int
	}{
		{"top_only", "POST", actionPath, `{"Name":"cat"}`, "cat", 200},
		{"filter_only", "POST", actionPath, `{"Filter":{"Name":"cat"}}`, "cat", 200},
		{"same_name", "POST", actionPath, `{"Name":"cat","Filter":{"Name":"cat"}}`, "cat", 200},
		{"same_lowercase_name", "POST", actionPath, `{"name":"cat","filter":{"Name":"cat"}}`, "cat", 200},
		{"both_empty", "POST", actionPath, `{"Name":"","Filter":{"Name":""}}`, "", 200},
		{"different_names", "POST", actionPath, `{"Name":"cat","Filter":{"Name":"dog"}}`, "", 400},
		{"different_lowercase_names", "POST", actionPath, `{"name":"cat","filter":{"Name":"dog"}}`, "", 400},
		{"empty_filter_name", "POST", actionPath, `{"Name":"cat","Filter":{"Name":""}}`, "", 400},
		{"empty_top_name", "POST", actionPath, `{"Name":"","Filter":{"Name":"cat"}}`, "", 400},
		{"query_conflict", "GET", "/v1/assets?name=cat&Filter=" + url.QueryEscape(`{"Name":"dog"}`), "", "", 400},
		{"query_same_name", "GET", "/v1/assets?name=cat&Filter=" + url.QueryEscape(`{"Name":"cat"}`), "", "cat", 200},
		{"query_body_conflict", "GET", "/v1/assets?name=cat", `{"Filter":{"Name":"dog"}}`, "", 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls.Load()
			response := seedanceCall(router, tc.method, tc.path, tc.body, 1)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status == http.StatusOK {
				require.Equal(t, before+1, calls.Load())
				require.Equal(t, tc.upstreamName, sentName)
			} else {
				require.Equal(t, before, calls.Load(), "conflicting selectors must not contact upstream")
				require.Contains(t, response.Body.String(), "Name/name and Filter.Name must match")
			}
		})
	}
}

func TestYHAssetListConcurrentFilters(t *testing.T) {
	router := yhAssetTestRouter(t, "https://yh-concurrent-filter-test.invalid/v1")
	for _, id := range []string{"42", "43", "44"} {
		seedAssetOwnershipForTest(t, 20, 1, "group", id)
	}
	seedYHListAsset(t, "asset-a", "42")
	seedYHListAsset(t, "asset-b", "43")
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	for _, tc := range []struct{ name, first, second string }{
		{"statuses", `{"Statuses":["Active"]}`, `{"Statuses":["Failed"]}`},
		{"asset_ids", `{"AssetIds":["asset-a"]}`, `{"AssetIds":["asset-b"]}`},
		{"ids", `{"Ids":["asset-a"]}`, `{"Ids":["asset-b"]}`},
		{"groups", `{"GroupIds":["42","44"]}`, `{"GroupIds":["43","44"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached, release := make(chan struct{}, 2), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				rows := []string{}
				if gjson.GetBytes(body, "page_number").Int() == 1 {
					for i := 0; i < 100; i++ {
						rows = append(rows, fmt.Sprintf(`{"id":"foreign-%d","status":"Active"}`, i))
					}
				} else {
					reached <- struct{}{}
					select {
					case <-release:
					case <-req.Context().Done():
						return nil, req.Context().Err()
					}
					rows = []string{`{"id":"asset-a","status":"Active"}`, `{"id":"asset-b","status":"Failed"}`}
				}
				response := fmt.Sprintf(`{"code":"success","data":{"items":[%s],"total_count":%d}}`, strings.Join(rows, ","), len(rows))
				return yhHTTPResponse(req, http.StatusOK, []byte(response), nil), nil
			})
			responses := make(chan *httptest.ResponseRecorder, 2)
			var workers sync.WaitGroup
			defer func() { once.Do(func() { close(release) }); workers.Wait() }()
			for _, filter := range []string{tc.first, tc.second} {
				workers.Add(1)
				go func(filter string) {
					defer workers.Done()
					responses <- yhAction(router, "ListAssets", `{"Filter":`+filter+`}`, 1)
				}(filter)
				select {
				case <-reached:
				case <-time.After(3 * time.Second):
					t.Fatal("different local filters were incorrectly joined to the same scan")
				}
			}
			once.Do(func() { close(release) })
			var ids []string
			for i := 0; i < 2; i++ {
				select {
				case response := <-responses:
					require.Equal(t, http.StatusOK, response.Code, response.Body.String())
					require.EqualValues(t, 1, gjson.GetBytes(response.Body.Bytes(), "Result.TotalCount").Int())
					ids = append(ids, gjson.GetBytes(response.Body.Bytes(), "Result.Items.0.Id").String())
				case <-time.After(3 * time.Second):
					t.Fatal("list scan did not complete")
				}
			}
			require.ElementsMatch(t, []string{"asset-a", "asset-b"}, ids)
		})
	}
}
