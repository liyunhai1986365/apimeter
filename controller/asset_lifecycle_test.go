package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// These synthetic responses verify passthrough, not which status the supplier
// will choose when a query races deletion.
func TestAssetLifecycleLateResponsesPreserveUpstreamWithoutRevivingOwnership(t *testing.T) {
	for _, backend := range []string{"volcengine-assets", "tgxmaas"} {
		for _, entry := range []string{"official", "rest"} {
			for _, scenario := range []struct {
				name, action, deletion, result string
				status                         int
			}{
				{"create", "CreateAsset", "DeleteAssetGroup", `{"Id":"asset-late","GroupId":"group"}`, 200},
				{"asset-alias", "GetAsset", "DeleteAsset", `{"Id":"known-child","upstream_asset_id":"asset-late","GroupId":"group"}`, 200},
				{"asset-same-id", "GetAsset", "DeleteAsset", `{"Id":"known-child","GroupId":"group","Status":"Processing"}`, 200},
				{"asset-group", "GetAsset", "DeleteAssetGroup", `{"Id":"known-child","upstream_asset_id":"asset-late","GroupId":"group"}`, 200},
				{"group", "GetAssetGroup", "DeleteAssetGroup", `{"Id":"group","upstream_group_id":"group-late"}`, 200},
				{"asset-failed", "GetAsset", "DeleteAsset", `{"Id":"known-child","Status":"Failed","Error":{"Code":"DownloadFailed","Message":"download failed"}}`, 200},
				{"upstream-business-error-200", "GetAsset", "DeleteAsset", "", 200},
				{"upstream-error-400", "GetAsset", "DeleteAsset", "", 400},
				{"upstream-error-404", "GetAsset", "DeleteAsset", "", 404},
				{"upstream-error-503", "GetAsset", "DeleteAsset", "", 503},
			} {
				t.Run(backend+"/"+entry+"/"+scenario.name, func(t *testing.T) {
					preserveSmartRetryTestConfiguration(t)
					started, release := make(chan struct{}), make(chan struct{})
					var once, signal sync.Once
					var operationCalls atomic.Int32
					body := `{"ResponseMetadata":{"RequestId":"late-upstream-request"},"Result":` + scenario.result + `}`
					if scenario.result == "" {
						body = `{"ResponseMetadata":{"RequestId":"late-upstream-request","Error":{"Code":"SupplierQueryError","Message":"supplier query failed"}}}`
					}
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if strings.HasPrefix(r.URL.Query().Get("Action"), "Delete") || r.Method == http.MethodDelete {
							_, _ = w.Write([]byte(`{"Result":{}}`))
							return
						}
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
						w.Header().Set("X-Request-Id", "late-upstream-request")
						w.WriteHeader(scenario.status)
						_, _ = w.Write([]byte(body))
					}))
					defer upstream.Close()
					router := assetSecurityRouter(t, upstream.URL, backend, "")
					seedAssetOwnershipForTest(t, 20, 1, "group", "group")
					seedAssetOwnershipForTest(t, 20, 1, "asset", "known-child")
					children, err := model.FindAssetBindings(1, "asset", "known-child")
					require.NoError(t, err)
					require.Len(t, children, 1)
					children[0].GroupID = "group"
					require.NoError(t, model.SaveAssetBindings(children))
					call := func(action string) *httptest.ResponseRecorder {
						id, path := "known-child", "/api/assets"
						if strings.HasSuffix(action, "Group") {
							id, path = "group", "/api/asset-groups"
						}
						requestBody := fmt.Sprintf(`{"Id":%q}`, id)
						if action == "CreateAsset" {
							requestBody = `{"GroupId":"group","URL":"https://example.com/ref.png","AssetType":"Image"}`
						}
						if entry == "official" {
							return seedanceCall(router, "POST", "/?Action="+action+"&Version=2024-01-01", requestBody, 1)
						}
						method := http.MethodGet
						if strings.HasPrefix(action, "Delete") {
							method = http.MethodDelete
						}
						if action == "CreateAsset" {
							method = http.MethodPost
						} else {
							path += "/" + id
						}
						return seedanceCall(router, method, path, requestBody, 1)
					}
					var writers sync.WaitGroup
					defer func() { once.Do(func() { close(release) }); writers.Wait() }()
					completed := make(chan *httptest.ResponseRecorder, 1)
					writers.Add(1)
					go func() { defer writers.Done(); completed <- call(scenario.action) }()
					select {
					case <-started:
					case <-time.After(10 * time.Second):
						t.Fatal("operation did not reach upstream")
					}
					deleted := call(scenario.deletion)
					require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
					once.Do(func() { close(release) })
					response := <-completed
					if scenario.action == "CreateAsset" {
						require.Equal(t, http.StatusBadGateway, response.Code, response.Body.String())
						require.Contains(t, response.Body.String(), "asset_binding_failed")
					} else {
						require.Equal(t, scenario.status, response.Code, response.Body.String())
						require.Equal(t, "late-upstream-request", response.Header().Get("X-Request-Id"))
						require.Equal(t, "late-upstream-request", gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.RequestId").String())
						if scenario.result != "" {
							require.JSONEq(t, scenario.result, gjson.GetBytes(response.Body.Bytes(), "Result").Raw)
							require.False(t, gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.Error").Exists())
						} else {
							require.Equal(t, "SupplierQueryError", gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.Error.Code").String())
							require.Equal(t, "supplier query failed", gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.Error.Message").String())
						}
					}
					require.EqualValues(t, 1, operationCalls.Load(), "late responses must not replay upstream")
					for _, ref := range []struct{ kind, id string }{{"asset", "asset-late"}, {"group", "group-late"}, {"asset", "known-child"}} {
						bindings, err := model.FindAssetBindings(1, ref.kind, ref.id)
						require.NoError(t, err)
						require.Empty(t, bindings, "late responses must not restore ownership: %s", ref.id)
					}
					var aliases int64
					require.NoError(t, model.DB.Model(&model.ConfigurableResourceState{}).Where("profile_id = ? AND resource_id = ?", "seedance-tgxmaas", "asset_handle").Count(&aliases).Error)
					require.Zero(t, aliases, "revoked queries must not write legacy handle mappings")
					// Later requests still undergo local ownership checks before forwarding.
					denied := call("GetAsset")
					require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
					require.EqualValues(t, 1, operationCalls.Load(), "revoked ownership cannot authorize a new request")
				})
			}
		}
	}
}

func TestAssetLifecycleDiscoveredFinalIDRetainsTaskIdentity(t *testing.T) {
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
	response := assetSecurityAction(router, "GetAsset", `{"Id":"upload-task"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assets, err := model.FindAssetBindings(1, "asset", "finished")
	require.NoError(t, err)
	require.Len(t, assets, 1)
	require.Equal(t, "upload-task", assets[0].CanonicalID)
	response = assetSecurityAction(router, "GetAsset", `{"Id":"finished"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = assetSecurityAction(router, "DeleteAsset", `{"Id":"finished"}`, 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	tasks, err := model.FindAssetBindings(1, "task", "upload-task")
	require.NoError(t, err)
	require.Empty(t, tasks, "deleting the discovered final ID must revoke the original task too")
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
