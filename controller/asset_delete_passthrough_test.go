package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetDeletionRetryMissingOnlyPassesThrough(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			var calls atomic.Int32
			const missing = `{"ResponseMetadata":{"Error":{"Code":"AssetNotFound","Message":"asset is already deleted"}}}`
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "upstream-delete")
				if calls.Add(1) == 1 {
					_, _ = w.Write([]byte(`{"Result":{}}`))
					return
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(missing))
			}))
			defer upstream.Close()
			router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "unrelated")
			bindings, err := model.FindAssetBindings(1, "asset", "owned")
			require.NoError(t, err)
			require.Len(t, bindings, 1)
			alias := bindings[0]
			alias.ID = "alias"
			task := alias
			task.Kind, task.ID = "task", "upload-task"
			require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{alias, task}))
			var attempts atomic.Int32
			db := model.DB
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_asset_revoke", func(tx *gorm.DB) {
				values, ok := tx.Statement.Dest.(map[string]any)
				if ok && values["status"] == model.ConfigurableResourceStateStatusInvalid {
					attempts.Add(1)
					tx.AddError(errors.New("injected local revocation failure"))
				}
			}))
			t.Cleanup(func() { db.Callback().Update().Remove("test:fail_asset_revoke") })
			first := seedanceCall(router, "DELETE", "/api/assets/owned", "", 1)
			require.Equal(t, http.StatusBadGateway, first.Code, first.Body.String())
			require.Contains(t, first.Body.String(), "contact the administrator")
			require.NotContains(t, first.Body.String(), "retry the deletion")
			var before, after []model.ConfigurableResourceState
			require.NoError(t, db.Order("id").Find(&before).Error)
			for range 2 {
				retried := seedanceCall(router, "DELETE", "/api/assets/owned", "", 1)
				require.Equal(t, status, retried.Code, retried.Body.String())
				require.Equal(t, missing, retried.Body.String())
				require.Empty(t, retried.Header().Get("X-Asset-Revocation-Status"))
				require.Equal(t, "upstream-delete", retried.Header().Get("X-Request-Id"))
			}
			require.EqualValues(t, 1, attempts.Load(), "404/410 must not attempt local revocation")
			require.NoError(t, db.Order("id").Find(&after).Error)
			require.Equal(t, before, after)
			for kind, handles := range map[string][]string{"asset": {"owned", "alias"}, "task": {"upload-task"}} {
				for _, handle := range handles {
					found, err := model.FindAssetBindings(1, kind, handle)
					require.NoError(t, err)
					require.Len(t, found, 1, "404/410 must preserve ownership of aliases and tasks")
				}
			}
			found, err := model.FindAssetBindings(1, "asset", "unrelated")
			require.NoError(t, err)
			require.Len(t, found, 1)
			require.EqualValues(t, 3, calls.Load())
		})
	}
}

func TestAssetDeletionErrorsDoNotRevoke(t *testing.T) {
	for _, tc := range []struct {
		name, method, body string
		status             int
	}{
		{"asset missing", "DELETE", `{"ResponseMetadata":{"Error":{"Code":"AssetNotFound"}}}`, 404},
		{"asset gone", "DELETE", `{"Error":{"Code":"InvalidAsset.NotFound"}}`, 410},
		{"generic missing", "DELETE", `{"error":{"code":"ResourceNotFound"}}`, 404},
		{"json gateway 404", "DELETE", `{"error":{"code":"not_found","message":"route not found"}}`, 404},
		{"gateway 404", "DELETE", "gateway route not found", 404},
		{"permission failure", "DELETE", `{"error":{"code":"AssetNotFound"}}`, 403},
		{"wrong resource kind", "DELETE", `{"error":{"code":"AssetGroupNotFound"}}`, 404},
		{"query is not deletion", "GET", `{"error":{"code":"AssetNotFound"}}`, 404},
		{"business error", "DELETE", `{"ResponseMetadata":{"Error":{"Code":"AssetNotFound"}}}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Id", "upstream-delete-error")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			var before, after []model.ConfigurableResourceState
			require.NoError(t, model.DB.Order("id").Find(&before).Error)
			response := seedanceCall(router, tc.method, "/api/assets/owned", "", 1)
			require.Equal(t, tc.status, response.Code)
			require.Equal(t, tc.body, response.Body.String())
			require.Equal(t, "upstream-delete-error", response.Header().Get("X-Request-Id"))
			require.Empty(t, response.Header().Get("X-Asset-Revocation-Status"))
			require.NoError(t, model.DB.Order("id").Find(&after).Error)
			require.Equal(t, before, after, "an upstream error must not change local asset state")
			found, err := model.FindAssetBindings(1, "asset", "owned")
			require.NoError(t, err)
			require.Len(t, found, 1)
		})
	}
}
