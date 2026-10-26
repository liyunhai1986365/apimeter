package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAssetListPageBufferLifetimeIsolationAndBudget(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") == "failed" {
			w.WriteHeader(502)
			return
		}
		_, _ = w.Write([]byte(`{"Items":[{"Id":"asset","GroupId":"parent"},{"GroupId":"parent-only"}],"total":2}`))
	}))
	defer upstream.Close()
	client := upstream.Client()
	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	first := acquireAssetListPages(t.Name())
	second := acquireAssetListPages(t.Name())
	require.Same(t, first, second)
	page, err := first.read(client, req, "asset")
	require.NoError(t, err)
	require.Equal(t, "asset", page.rows[0].id)
	require.Empty(t, page.rows[1].id, "an asset's parent is not ownership proof")
	first.release()
	_, err = second.read(client, req, "asset")
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load(), "active readers must reuse completed raw pages")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = second.read(client, req.WithContext(ctx), "asset")
	require.ErrorIs(t, err, context.Canceled)
	other := req.Clone(context.Background())
	other.Header.Set("Authorization", "different-account")
	_, err = second.read(client, other, "asset")
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
	other.Header.Set("Authorization", "failed")
	for range 2 {
		page, err = second.read(client, other, "asset")
		require.NoError(t, err)
		require.Equal(t, 502, page.response.StatusCode)
	}
	require.EqualValues(t, 4, calls.Load(), "upstream errors must not be retained")
	second.release()
	require.Zero(t, assetListPageSets.bytes)
	require.Empty(t, assetListPageSets.sets)
	third := acquireAssetListPages(t.Name())
	defer third.release()
	_, err = third.read(client, req, "asset")
	require.NoError(t, err)
	require.EqualValues(t, 5, calls.Load(), "a new scan must fetch fresh data")
	// A full payload budget falls back to fetching without retaining more data.
	assetListPageSets.bytes = assetListPageBufferLimit
	defer func() { assetListPageSets.bytes = third.bytes }()
	other.Header.Set("Authorization", "uncached")
	for range 2 {
		_, err = third.read(client, other, "asset")
		require.NoError(t, err)
	}
	require.EqualValues(t, 7, calls.Load())
	require.Equal(t, assetListPageBufferLimit, assetListPageSets.bytes)
}

func TestAssetListPageBufferDoesNotExtendWithContinuousReaders(t *testing.T) {
	old := acquireAssetListPages(t.Name())
	old.expires = time.Now().Add(-time.Second)
	fresh := acquireAssetListPages(t.Name())
	require.NotSame(t, old, fresh)
	old.release()
	require.Same(t, fresh, assetListPageSets.sets[t.Name()], "releasing an expired scan must not discard its replacement")
	fresh.release()
	require.Empty(t, assetListPageSets.sets)
}
