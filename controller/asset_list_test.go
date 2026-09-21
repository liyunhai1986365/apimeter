package controller

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetListLargeSharedAccount(t *testing.T) {
	for _, backend := range []string{"youniyouju", "volcengine-assets", "tgxmaas", "material", "api-assets"} {
		for _, tc := range []struct {
			name                    string
			ownedPage, total, calls int
			filtered                bool
		}{
			{"stop-after-owned-alias", 1, 20001, 1, false},
			{"owned-beyond-100-pages", 101, 20001, 101, false},
			{"filtered-owned-item-absent", 101, 10101, 102, true},
		} {
			t.Run(backend+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					page, _ := strconv.Atoi(r.URL.Query().Get("page"))
					filterPresent := r.URL.Query().Get("statuses") == "Active"
					if r.Method == http.MethodPost {
						body, _ := io.ReadAll(r.Body)
						page = int(gjson.GetBytes(body, "PageNumber").Int())
						filterPresent = gjson.GetBytes(body, "Filter.Statuses.0").String() == "Active"
					}
					if page < 1 || (tc.filtered && !filterPresent) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					rows := make([]string, 0, 100)
					for i := 0; i < 100 && (page-1)*100+i < tc.total; i++ {
						id := fmt.Sprintf("foreign-%d", (page-1)*100+i)
						if page == tc.ownedPage && i == 0 {
							id = "owned"
						}
						rows = append(rows, fmt.Sprintf(`{"Id":%q,"Status":"Active"}`, id))
					}
					w.Header().Set("Content-Type", "application/json")
					if r.Method == http.MethodGet {
						_, _ = fmt.Fprintf(w, `{"data":{"items":[%s],"total":%d}}`, strings.Join(rows, ","), tc.total)
					} else {
						_, _ = fmt.Fprintf(w, `{"Result":{"Items":[%s],"TotalCount":%d}}`, strings.Join(rows, ","), tc.total)
					}
				}))
				defer upstream.Close()
				r := assetSecurityRouter(t, upstream.URL, backend, "")
				seedAssetListAliases(t, "owned", "original-owned")
				path, body := "/api/assets?page=1&page_size=20", ""
				if tc.filtered {
					// A locally owned item excluded by the upstream filter must not
					// inflate the total or prevent scanning to the actual final page.
					seedAssetOwnershipForTest(t, 20, 1, "asset", "filtered-out")
					if backend == "material" || backend == "api-assets" {
						path += "&statuses=Active"
					} else {
						body = `{"Filter":{"Statuses":["Active"]}}`
					}
				}
				got := seedanceCall(r, http.MethodGet, path, body, 1)
				require.Equal(t, http.StatusOK, got.Code, got.Body.String())
				items, total := "Result.Items", "Result.TotalCount"
				if backend == "material" || backend == "api-assets" {
					items, total = "data.items", "data.total"
				}
				require.Equal(t, "owned", gjson.GetBytes(got.Body.Bytes(), items+".0.Id").String())
				require.Len(t, gjson.GetBytes(got.Body.Bytes(), items).Array(), 1)
				require.EqualValues(t, 1, gjson.GetBytes(got.Body.Bytes(), total).Int())
				require.NotContains(t, got.Body.String(), "foreign")
				require.NotContains(t, got.Body.String(), "filtered-out")
				require.EqualValues(t, tc.calls, calls.Load())
			})
		}
	}
}

func seedAssetListAliases(t *testing.T, canonical string, aliases ...string) {
	t.Helper()
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, ok := configurable.AssetProfile(ch.GetSetting().Protocol)
	require.True(t, ok)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	binding := model.AssetBinding{ChannelID: 20, UserID: 1, Backend: profile.ID, Scope: scope, Kind: "asset", CanonicalID: canonical}
	for _, id := range append([]string{canonical}, aliases...) {
		require.NoError(t, model.SaveAssetBinding(binding, id))
	}
}

func TestAssetListCursorDeduplicatesAliases(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		id, next := "original-owned", "page2"
		switch gjson.GetBytes(body, "NextToken").String() {
		case "page2":
			id, next = "owned", "page3"
		case "page3":
			id, next = "owned-second", "unused-page4"
		case "unused-page4":
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"Result":{"Items":[{"Id":%q}],"NextToken":%q,"TotalCount":20001}}`, id, next)
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "")
	seedAssetListAliases(t, "owned", "original-owned")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned-second")
	got := assetSecurityAction(r, "ListAssets", `{"MaxResults":1}`, 1)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	require.Equal(t, "original-owned", gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
	require.EqualValues(t, 2, gjson.GetBytes(got.Body.Bytes(), "Result.TotalCount").Int())
	cursor := gjson.GetBytes(got.Body.Bytes(), "Result.NextToken").String()
	require.True(t, strings.HasPrefix(cursor, "asset-v1-"))
	got = assetSecurityAction(r, "ListAssets", `{"MaxResults":1,"NextToken":"`+cursor+`"}`, 1)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	require.Equal(t, "owned-second", gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
	require.EqualValues(t, 2, gjson.GetBytes(got.Body.Bytes(), "Result.TotalCount").Int())
	require.Empty(t, gjson.GetBytes(got.Body.Bytes(), "Result.NextToken").String())
	require.EqualValues(t, 6, calls.Load())
}

func TestAssetListPaginationBeyondOldLimitAndOverflow(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"owned"}],"TotalCount":1}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
	got := seedanceCall(r, http.MethodGet, "/api/assets?page=10002&page_size=1", "", 1)
	require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	require.Empty(t, gjson.GetBytes(got.Body.Bytes(), "Result.Items").Array())
	require.EqualValues(t, 1, gjson.GetBytes(got.Body.Bytes(), "Result.TotalCount").Int())
	for _, query := range []string{
		"page=0", "page_size=101", "page=-1", "page=99999999999999999999999999",
		fmt.Sprintf("page=%d&page_size=100", math.MaxInt),
	} {
		got = seedanceCall(r, http.MethodGet, "/api/assets?"+query, "", 1)
		require.Equal(t, http.StatusBadRequest, got.Code, got.Body.String())
	}
	require.EqualValues(t, 1, calls.Load(), "invalid pagination must fail before contacting upstream")
}

func TestAssetListCancellationDoesNotReturnPartialResults(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Drain the POST body so net/http can observe the client disconnect
		// while the handler waits for cancellation.
		_, _ = io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 2 {
			cancel()
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"owned-first"}],"TotalCount":20001}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned-first")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned-later")
	req := httptest.NewRequest(http.MethodGet, "/api/assets", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer seedancetest1")
	got := httptest.NewRecorder()
	r.ServeHTTP(got, req)
	require.Equal(t, http.StatusBadGateway, got.Code, got.Body.String())
	require.Contains(t, got.Body.String(), "asset_list_failed")
	require.NotContains(t, got.Body.String(), "owned-first")
	require.EqualValues(t, 2, calls.Load())
}
