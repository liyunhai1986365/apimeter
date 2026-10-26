package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// A late generation must neither replay cached pages nor join a previous
// generation's in-flight HTTP fetch, even if both first pages are identical.
func TestAssetListPagesFreezeBeforeFetch(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var once sync.Once
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprintf(w, `{"Items":[{"Id":"version-%d"}]}`, call)
	}))
	defer upstream.Close()
	defer once.Do(func() { close(release) })
	req, err := http.NewRequest(http.MethodGet, upstream.URL, nil)
	require.NoError(t, err)
	first := acquireAssetListPages(t.Name())
	sibling := acquireAssetListPages(t.Name())
	defer first.release()
	defer sibling.release()
	require.Same(t, first, sibling)
	done := make(chan error, 1)
	go func() { _, err := first.read(upstream.Client(), req, "asset"); done <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("first page did not start")
	}
	late := acquireAssetListPages(t.Name())
	defer late.release()
	require.NotSame(t, first, late)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	page, err := late.read(upstream.Client(), req.WithContext(ctx), "asset")
	require.NoError(t, err, "fresh generation must not wait for the old page fetch")
	require.Equal(t, "version-2", page.rows[0].id)
	once.Do(func() { close(release) })
	require.NoError(t, <-done)
	page, err = sibling.read(upstream.Client(), req, "asset")
	require.NoError(t, err)
	require.Equal(t, "version-1", page.rows[0].id)
	require.EqualValues(t, 2, calls.Load(), "already-admitted readers still share completed pages")
	newer := acquireAssetListPages(t.Name())
	defer newer.release()
	require.NotSame(t, late, newer)
	page, err = newer.read(upstream.Client(), req, "asset")
	require.NoError(t, err)
	require.Equal(t, "version-3", page.rows[0].id)
}

func TestAssetListLateReaderAfterPrepend(t *testing.T) {
	for _, sameUser := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_user=%t", sameUser), func(t *testing.T) {
			var mutated atomic.Bool
			blocked, release, freshSecond := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce, blockOnce, freshOnce sync.Once
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				page := int(gjson.GetBytes(body, "PageNumber").Int())
				after := mutated.Load()
				total, rows := 5, `{"Id":"a0"},{"Id":"a1"}`
				if after {
					total, rows = 6, `{"Id":"new"},{"Id":"a0"}`
				}
				switch page {
				case 2:
					rows = `{"Id":"b0"},{"Id":"b1"}`
					if after {
						rows = `{"Id":"a1"},{"Id":"b0"}`
						freshOnce.Do(func() { close(freshSecond) })
					}
				case 3:
					rows = `{"Id":"tail"}`
					if after {
						rows = `{"Id":"b1"},{"Id":"tail"}`
					}
					blockOnce.Do(func() { close(blocked) })
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"Result":{"Items":[%s],"TotalCount":%d}}`, rows, total)
			}))
			defer upstream.Close()
			defer releaseOnce.Do(func() { close(release) })
			router := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			secondUser := 2
			if sameUser {
				secondUser = 1
			}
			seedAssetOwnershipForTest(t, 20, 1, "asset", "tail")
			seedAssetOwnershipForTest(t, 20, secondUser, "asset", "a1")
			first := make(chan *httptest.ResponseRecorder, 1)
			go func() { first <- seedanceCall(router, "GET", "/api/assets", "", 1) }()
			select {
			case <-blocked:
			case <-time.After(3 * time.Second):
				t.Fatal("first scan did not reach page three")
			}
			mutated.Store(true)
			second := make(chan *httptest.ResponseRecorder, 1)
			go func() { second <- seedanceCall(router, "GET", "/api/assets", "", secondUser) }()
			select {
			case <-freshSecond:
			case <-time.After(3 * time.Second):
				t.Fatal("late scan reused old pages instead of fetching its new second page")
			}
			if !sameUser {
				select {
				case got := <-second:
					require.Equal(t, 200, got.Code, got.Body.String())
					require.Equal(t, "a1", gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
					require.EqualValues(t, 1, gjson.GetBytes(got.Body.Bytes(), "Result.TotalCount").Int())
				case <-time.After(3 * time.Second):
					t.Fatal("late user should finish independently of the first scan")
				}
			}
			releaseOnce.Do(func() { close(release) })
			require.Equal(t, 200, (<-first).Code)
			if sameUser {
				select {
				case got := <-second:
					require.Equal(t, 200, got.Code, got.Body.String())
					require.Equal(t, "a1", gjson.GetBytes(got.Body.Bytes(), "Result.Items.0.Id").String())
					require.EqualValues(t, 2, gjson.GetBytes(got.Body.Bytes(), "Result.TotalCount").Int())
				case <-time.After(3 * time.Second):
					t.Fatal("late scan did not finish")
				}
			}
		})
	}
}

func TestAssetListFirstPageIdentity(t *testing.T) {
	first, err := assetListFirstPageFingerprint([]byte(`{"ResponseMetadata":{"RequestId":"one"},"Result":{"Items":[{"Id":"a"}],"TotalCount":5,"NextToken":"next"}}`))
	require.NoError(t, err)
	same, err := assetListFirstPageFingerprint([]byte(`{"ResponseMetadata":{"RequestId":"two"},"Result":{"Items":[{"Id":"a"}],"TotalCount":5,"NextToken":"next"}}`))
	require.NoError(t, err)
	require.Equal(t, first, same, "per-request metadata must not disable sharing")
	for _, body := range []string{
		`{"Result":{"Items":[{"Id":"b"}],"TotalCount":5,"NextToken":"next"}}`,
		`{"Result":{"Items":[{"Id":"a"}],"TotalCount":6,"NextToken":"next"}}`,
		`{"Result":{"Items":[{"Id":"a"}],"TotalCount":5,"NextToken":"changed"}}`,
	} {
		changed, err := assetListFirstPageFingerprint([]byte(body))
		require.NoError(t, err)
		require.NotEqual(t, first, changed, "a changed first page must not join an older scan")
	}
}
