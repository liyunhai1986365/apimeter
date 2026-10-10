package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestAssetGroupDeletionRetryMissingOnlyPassesThrough(t *testing.T) {
	var calls atomic.Int32
	const missing = `{"ResponseMetadata":{"Error":{"Code":"AssetGroupNotFound","Message":"group is already deleted"}}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "upstream-group-delete")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(`{"Result":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(missing))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
	seedAssetOwnershipForTest(t, 20, 1, "group", "parent")
	groups, err := model.FindAssetBindings(1, "group", "parent")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	child := groups[0]
	child.Kind, child.ID, child.CanonicalID, child.GroupID = "asset", "child", "child", "parent"
	require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{child}))
	var attempts atomic.Int32
	db := model.DB
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail_first_group_revoke", func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if ok && values["status"] == model.ConfigurableResourceStateStatusInvalid {
			attempts.Add(1)
			tx.AddError(errors.New("injected local revocation failure"))
		}
	}))
	t.Cleanup(func() { db.Callback().Update().Remove("test:fail_first_group_revoke") })
	first := seedanceCall(router, "DELETE", "/api/asset-groups/parent", "", 1)
	require.Equal(t, http.StatusBadGateway, first.Code, first.Body.String())
	require.Contains(t, first.Body.String(), "contact the administrator")
	require.NotContains(t, first.Body.String(), "retry the deletion")
	var before, after []model.ConfigurableResourceState
	require.NoError(t, db.Order("id").Find(&before).Error)
	for range 2 {
		retried := seedanceCall(router, "DELETE", "/api/asset-groups/parent", "", 1)
		require.Equal(t, http.StatusNotFound, retried.Code, retried.Body.String())
		require.Equal(t, missing, retried.Body.String())
		require.Equal(t, "upstream-group-delete", retried.Header().Get("X-Request-Id"))
		require.Empty(t, retried.Header().Get("X-Asset-Revocation-Status"))
	}
	require.EqualValues(t, 1, attempts.Load(), "404 must not attempt local revocation")
	require.NoError(t, db.Order("id").Find(&after).Error)
	require.Equal(t, before, after)
	foundGroups, err := model.FindAssetBindings(1, "group", "parent")
	require.NoError(t, err)
	require.Len(t, foundGroups, 1)
	found, err := model.FindAssetBindings(1, "asset", "child")
	require.NoError(t, err)
	require.Len(t, found, 1, "404 must not revoke children")
	require.EqualValues(t, 3, calls.Load())
}

func TestAssetGroupDeletionErrorsDoNotRevoke(t *testing.T) {
	for _, entry := range []string{"rest", "official"} {
		for _, tc := range []struct {
			name, body, code string
			status           int
		}{
			{"group missing", `{"ResponseMetadata":{"Error":{"Code":"AssetGroupNotFound"}}}`, "AssetGroupNotFound", 404},
			{"group gone", `{"Error":{"Code":"InvalidAssetGroup.NotFound"}}`, "InvalidAssetGroup.NotFound", 410},
			{"generic missing", `{"error":{"code":"ResourceNotFound"}}`, "ResourceNotFound", 404},
			{"json gateway 404", `{"error":{"code":"not_found","message":"route not found"}}`, "not_found", 404},
			{"gateway 404", "gateway route not found", "asset_request_failed", 404},
			{"business error", `{"ResponseMetadata":{"Error":{"Code":"AssetGroupNotFound"}}}`, "AssetGroupNotFound", 200},
		} {
			t.Run(entry+"/"+tc.name, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Request-Id", "upstream-group-delete-error")
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				}))
				defer upstream.Close()
				router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
				seedAssetOwnershipForTest(t, 20, 1, "group", "parent")
				groups, err := model.FindAssetBindings(1, "group", "parent")
				require.NoError(t, err)
				require.Len(t, groups, 1)
				child := groups[0]
				child.Kind, child.ID, child.CanonicalID, child.GroupID = "asset", "child", "child", "parent"
				require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{child}))
				var before, after []model.ConfigurableResourceState
				require.NoError(t, model.DB.Order("id").Find(&before).Error)
				method, path, body := "DELETE", "/api/asset-groups/parent", ""
				if entry == "official" {
					method, path, body = "POST", "/?Action=DeleteAssetGroup&Version=2024-01-01", `{"Id":"parent"}`
				}
				response := seedanceCall(router, method, path, body, 1)
				require.Equal(t, tc.status, response.Code, response.Body.String())
				if entry == "official" {
					// The Action endpoint retains its existing official envelope.
					require.Equal(t, tc.code, gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.Error.Code").String())
					require.Equal(t, "upstream-group-delete-error", gjson.GetBytes(response.Body.Bytes(), "ResponseMetadata.RequestId").String())
					require.False(t, gjson.GetBytes(response.Body.Bytes(), "Result").Exists())
				} else {
					require.Equal(t, tc.body, response.Body.String())
				}
				require.Equal(t, "upstream-group-delete-error", response.Header().Get("X-Request-Id"))
				require.Empty(t, response.Header().Get("X-Asset-Revocation-Status"))
				require.NoError(t, model.DB.Order("id").Find(&after).Error)
				require.Equal(t, before, after, "an upstream error must not change group or child state")
				groups, err = model.FindAssetBindings(1, "group", "parent")
				require.NoError(t, err)
				require.Len(t, groups, 1)
				children, err := model.FindAssetBindings(1, "asset", "child")
				require.NoError(t, err)
				require.Len(t, children, 1)
			})
		}
	}
}
