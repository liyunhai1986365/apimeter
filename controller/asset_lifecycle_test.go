package controller

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestAssetLifecycleLateResponsesCannotRegisterInDeletedGroup(t *testing.T) {
	for _, action := range []string{"CreateAsset", "GetAsset"} {
		t.Run(action, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			started := make(chan struct{})
			release := make(chan struct{})
			var once, signal sync.Once
			var operationCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("Action") {
				case "CreateAsset", "GetAsset":
					operationCalls.Add(1)
					signal.Do(func() { close(started) })
					select {
					case <-release:
					case <-r.Context().Done():
						return
					case <-time.After(10 * time.Second):
						w.WriteHeader(http.StatusGatewayTimeout)
						return
					}
					_, _ = w.Write([]byte(`{"Result":{"Id":"late-child","GroupId":"group","ProjectName":"test-project"}}`))
				case "DeleteAssetGroup":
					_, _ = w.Write([]byte(`{"Result":{}}`))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer upstream.Close()
			router := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			seedAssetOwnershipForTest(t, 20, 1, "group", "group")
			body := `{"GroupId":"group","URL":"https://example.com/ref.png","AssetType":"Image"}`
			if action == "GetAsset" {
				seedAssetOwnershipForTest(t, 20, 1, "asset", "known-child")
				body = `{"Id":"known-child"}`
			}
			var writers sync.WaitGroup
			defer func() {
				once.Do(func() { close(release) })
				writers.Wait()
			}()
			created := make(chan *httptest.ResponseRecorder, 1)
			writers.Add(1)
			go func() {
				defer writers.Done()
				created <- assetSecurityAction(router, action, body, 1)
			}()
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("create did not reach upstream")
			}
			deleted := assetSecurityAction(router, "DeleteAssetGroup", `{"Id":"group"}`, 1)
			require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
			groups, err := model.FindAssetBindings(1, "group", "group")
			require.NoError(t, err)
			require.Empty(t, groups)
			once.Do(func() { close(release) })
			response := <-created
			require.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "asset_binding_failed")
			require.EqualValues(t, 1, operationCalls.Load(), "a rejected late response must not replay the upstream operation")
			children, err := model.FindAssetBindings(1, "asset", "late-child")
			require.NoError(t, err)
			require.Empty(t, children, "late upstream response must not register a child in a deleted group")
		})
	}
}

func TestAssetLifecycleClaimFinalIDRetainsTaskIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("Action") == "DeleteAsset" {
			_, _ = w.Write([]byte(`{"Result":{}}`))
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"Id":"finished","TaskId":"upload-task"}}`))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "task", "upload-task")
	response := assetSecurityAction(router, "GetAsset", `{"Id":"finished"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assets, err := model.FindAssetBindings(1, "asset", "finished")
	require.NoError(t, err)
	require.Len(t, assets, 1)
	require.Equal(t, "upload-task", assets[0].CanonicalID)
	response = assetSecurityAction(router, "DeleteAsset", `{"Id":"finished"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	tasks, err := model.FindAssetBindings(1, "task", "upload-task")
	require.NoError(t, err)
	require.Empty(t, tasks, "deleting a claimed final ID must revoke the original task too")
}
