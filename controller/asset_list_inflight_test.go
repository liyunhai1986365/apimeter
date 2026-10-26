package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetListPagesSeparateAuthAndFiltersAndDoNotCache(t *testing.T) {
	blocked, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") == "account-one" && string(body) == "base-filter" {
			once.Do(func() { close(blocked) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Items":[]}}`))
	}))
	defer upstream.Close()
	defer releaseOnce.Do(func() { close(release) })
	client := upstream.Client()
	request := func(auth, filter string) *http.Request {
		req, err := http.NewRequest(http.MethodPost, upstream.URL, bytes.NewReader([]byte(filter)))
		require.NoError(t, err)
		req.Header.Set("Authorization", auth)
		return req
	}
	first := make(chan error, 1)
	base := request("account-one", "base-filter")
	go func() { _, _, err := readAssetListPage(client, base); first <- err }()
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("first page did not start")
	}
	for _, req := range []*http.Request{request("account-two", "base-filter"), request("account-one", "different-filter")} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		resp, _, err := readAssetListPage(client, req.WithContext(ctx))
		cancel()
		require.NoError(t, err, "different credentials or filters must not join a blocked page")
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	releaseOnce.Do(func() { close(release) })
	require.NoError(t, <-first)
	require.EqualValues(t, 3, calls.Load())
	_, _, err := readAssetListPage(client, base)
	require.NoError(t, err)
	require.EqualValues(t, 4, calls.Load(), "a completed page must be fetched again")
}

func TestAssetListInFlightSeparatesUsersAndFilters(t *testing.T) {
	blocked, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if gjson.GetBytes(body, "Filter.Statuses.0").String() == "Active" {
			_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"first"}],"TotalCount":1}}`))
			return
		}
		if gjson.GetBytes(body, "PageNumber").Int() == 2 {
			once.Do(func() { close(blocked) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"second"}],"TotalCount":3}}`))
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"first"},{"Id":"other-user"}],"TotalCount":3}}`))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "first")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "second")
	seedAssetOwnershipForTest(t, 20, 2, "asset", "other-user")
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- seedanceCall(router, "GET", "/api/assets", "", 1) }()
	defer func() { close(release); <-done }()
	select {
	case <-blocked:
	case <-time.After(3 * time.Second):
		t.Fatal("first user's list did not reach page two")
	}
	for _, tc := range []struct {
		user     int
		body, id string
	}{{2, "", "other-user"}, {1, `{"Filter":{"Statuses":["Active"]}}`, "first"}} {
		response := make(chan *httptest.ResponseRecorder, 1)
		go func() { response <- seedanceCall(router, "GET", "/api/assets", tc.body, tc.user) }()
		select {
		case got := <-response:
			require.Equal(t, 200, got.Code, got.Body.String())
			require.Equal(t, tc.id, gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
			require.EqualValues(t, 1, gjson.GetBytes(got.Body.Bytes(), "Result.TotalCount").Int())
		case <-time.After(3 * time.Second):
			t.Fatal("unrelated user/filter was incorrectly joined to the blocked scan")
		}
	}
}

func TestAssetListPageRechecksRevocationWithCanonicalAlias(t *testing.T) {
	assetSecurityRouter(t, "http://unused.invalid", "youniyouju", "")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	group := model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "group", ID: "parent", CanonicalID: "parent"}
	asset := model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "asset", ID: "alias", CanonicalID: "canonical-without-handle", GroupID: "parent"}
	require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{group, asset}))
	a := &assetAccessRequest{channel: ch, scope: scope, op: assetOperation{kind: "asset", action: "list"}}
	proofs := map[string]string{"alias": "canonical-without-handle"}
	require.NoError(t, revalidateAssetListPage(context.Background(), a, 1, proofs))
	require.ErrorIs(t, revalidateAssetListPage(context.Background(), a, 2, proofs), model.ErrAssetRevoked)
	require.NoError(t, model.InvalidateAssetBindings(group, true))
	require.ErrorIs(t, revalidateAssetListPage(context.Background(), a, 1, proofs), model.ErrAssetRevoked)
}
