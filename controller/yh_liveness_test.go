package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHGroupQueriesCoalesceWithoutHoldingCacheLock(t *testing.T) {
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/slow" {
			close(entered)
			<-release
		}
		_, _ = w.Write([]byte(`{"code":"success","data":{"items":[]}}`))
	}))
	defer upstream.Close()
	client := upstream.Client()
	request, _ := http.NewRequest("GET", upstream.URL+"/slow", nil)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _, err := readYHGroups(client, request.Clone(context.Background()))
			if resp != nil {
				_ = resp.Body.Close()
			}
			results <- err
		}()
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	other, _ := http.NewRequestWithContext(ctx, "GET", upstream.URL+"/independent", nil)
	resp, _, err := readYHGroups(client, other)
	// Release the blocked server before assertions/cleanup, even on failure.
	close(release)
	wg.Wait()
	require.NoError(t, err, "another account/query must not wait for slow network work")
	if resp != nil {
		_ = resp.Body.Close()
	}
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	require.EqualValues(t, 2, calls.Load())
}

func TestYHSessionPendingConflictingAndFaceMismatch(t *testing.T) {
	for _, scenario := range []string{"pending", "duplicate", "old", "human"} {
		t.Run(scenario, func(t *testing.T) {
			s := yhAssetRequest{binding: model.AssetBinding{ChannelID: 20, UserID: 1, Scope: "test"}, session: model.AssetBinding{SessionName: "name", SessionCreatedAt: time.Now().Unix()}}
			rows := `{"items":[]}`
			if scenario == "duplicate" {
				rows = `{"items":[{"artist_name":"name"},{"artist_name":"name"}]}`
			}
			if scenario == "old" {
				rows = `{"items":[{"artist_name":"name","authorized_at":"2020-01-01T00:00:00Z"}]}`
			}
			if scenario == "human" {
				row, err := yhAssetRow(gjson.Parse(`{"id":"asset-human","status":"Failed","error":{"code":"FaceMismatch","message":"different person"}}`))
				require.NoError(t, err)
				require.Equal(t, "Failed", row["Status"])
				return
			}
			request := httptest.NewRequest("GET", "/", nil)
			resp, err := s.completeSession(request, gjson.Parse(rows))
			require.NoError(t, err)
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			require.Equal(t, 409, resp.StatusCode, string(body))
			if scenario == "pending" {
				require.Equal(t, "2", resp.Header.Get("Retry-After"))
			}
		})
	}
}
