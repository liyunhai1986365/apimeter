package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestAssetResponseHistoricalAssetTuplePollingIsReadOnly(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Id":"old-asset","Status":"Processing"}}`))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "old-asset")
	bindings, err := model.FindAssetBindings(1, "asset", "old-asset")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	bindings[0].Project = "test-project"
	require.NoError(t, model.SaveAssetBindings(bindings))
	db := model.DB
	require.NoError(t, db.Model(&model.ConfigurableResourceState{}).
		Where("profile_id = ? AND resource_id = ?", "asset-access-v1", "asset").
		Updates(map[string]any{"user_id": 7, "token_id": 8}).Error)
	// A historical asset may predate lifecycle markers; GET must not backfill it.
	require.NoError(t, db.Where("profile_id = ?", "asset-access-lock-v1").Delete(&model.ConfigurableResourceState{}).Error)
	var before, after []model.ConfigurableResourceState
	require.NoError(t, db.Order("id").Find(&before).Error)
	var writes, locks atomic.Int32
	recordWrite := func(tx *gorm.DB) {
		if tx.Statement.Table == "configurable_resource_states" {
			writes.Add(1)
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:old_asset_create", recordWrite))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:old_asset_update", recordWrite))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:old_asset_lock", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking); ok {
			locks.Add(1)
		}
	}))
	t.Cleanup(func() {
		db.Callback().Create().Remove("test:old_asset_create")
		db.Callback().Update().Remove("test:old_asset_update")
		db.Callback().Query().Remove("test:old_asset_lock")
	})
	for range 3 {
		response := seedanceCall(router, "GET", "/api/assets/old-asset", "", 1)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "Processing")
	}
	require.NoError(t, db.Order("id").Find(&after).Error)
	require.Equal(t, before, after, "historical polling must not create duplicate ownership or rewrite old rows")
	require.Zero(t, writes.Load())
	require.Zero(t, locks.Load())
	denied := seedanceCall(router, "GET", "/api/assets/old-asset", "", 2)
	require.Equal(t, http.StatusNotFound, denied.Code)
	require.EqualValues(t, 3, upstreamCalls.Load())
}

func TestAssetResponseHistoricalGroupTuplePreservesSuccessfulGET(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Id":"owned","GroupId":"original-parent","Status":"Processing"}}`))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "volcengine-assets", "")
	seedAssetOwnershipForTest(t, 20, 1, "group", "original-parent")
	groups, err := model.FindAssetBindings(1, "group", "original-parent")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	group := groups[0]
	group.CanonicalID = "canonical-parent"
	// Emulate the existing historical representation without migration.
	metadata, err := common.Marshal(group)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.ConfigurableResourceState{}).
		Where("profile_id = ? AND state_key = ?", "asset-access-v1", fmt.Sprintf("%x", sha256.Sum256([]byte(group.ID)))).
		Updates(map[string]any{"user_id": 7, "token_id": 8, "metadata": string(metadata)}).Error)
	require.NoError(t, model.DB.Where("profile_id = ?", "asset-access-lock-v1").Delete(&model.ConfigurableResourceState{}).Error)
	asset := group
	asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "owned", "owned", group.CanonicalID
	require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{asset}))
	response := seedanceCall(router, "GET", "/api/assets/owned", "", 1)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), "Processing")
}

func TestAssetResponseGroupLookupDeduplicatesAndPreservesProjects(t *testing.T) {
	assetSecurityRouter(t, "http://127.0.0.1:1", "youniyouju", "")
	group := model.AssetBinding{ChannelID: 20, UserID: 1, Scope: "account", Kind: "group", ID: "original-group", CanonicalID: "local-group", Project: "project"}
	other := group
	other.Scope, other.CanonicalID, other.Project = "other-account", "other-canonical", "other-project"
	require.NoError(t, model.SaveAssetBindings([]model.AssetBinding{group, other}))
	var reads int
	require.NoError(t, model.DB.Callback().Query().After("gorm:query").Register("test:response_group_reads", func(tx *gorm.DB) {
		// GORM also invokes this callback while building MySQL subqueries.
		// Dry runs generate SQL without issuing a database query.
		if !tx.DryRun {
			reads++
		}
	}))
	t.Cleanup(func() { model.DB.Callback().Query().Remove("test:response_group_reads") })
	for _, tc := range []struct {
		name, body, project string
		known               []model.AssetBinding
		wantReads           int
		conflict            bool
	}{
		{name: "repeated fields", body: `{"GroupId":"original-group","group_id":"original-group","Result":{"GroupId":"original-group"},"data":{"group_id":"original-group"}}`, wantReads: 2},
		{name: "verified group", body: `{"Result":{"GroupId":"original-group"}}`, known: []model.AssetBinding{group}},
		{name: "different scope is not reusable", body: `{"Result":{"GroupId":"original-group"}}`, known: []model.AssetBinding{other}, wantReads: 2},
		{name: "project conflict", body: `{"Result":{"GroupId":"original-group"}}`, project: "conflicting-project", wantReads: 2, conflict: true},
		{name: "reused project conflict", body: `{"Result":{"GroupId":"original-group"}}`, project: "conflicting-project", known: []model.AssetBinding{group}, conflict: true},
		{name: "different groups", body: `{"GroupId":"original-group","Result":{"GroupId":"different-group"}}`, wantReads: 3, conflict: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := model.AssetBinding{ChannelID: 20, UserID: 1, Scope: "account", Kind: "asset", Project: tc.project}
			reads = 0
			err := applyAssetResponseMetadata(context.Background(), &binding, []byte(tc.body), tc.known...)
			require.Equal(t, tc.wantReads, reads)
			if tc.conflict {
				require.ErrorIs(t, err, model.ErrAssetNotOwned)
			} else {
				require.NoError(t, err)
				require.Equal(t, "local-group", binding.GroupID)
				require.Equal(t, "project", binding.Project)
			}
		})
	}
	// A previously empty project may have been hydrated by another request.
	incomplete := group
	incomplete.Project = ""
	reads = 0
	binding := model.AssetBinding{ChannelID: 20, UserID: 1, Scope: "account", Kind: "asset"}
	require.NoError(t, applyAssetResponseMetadata(context.Background(), &binding, []byte(`{"GroupId":"original-group"}`), incomplete))
	require.Equal(t, 2, reads)
	require.Equal(t, "project", binding.Project)
}
