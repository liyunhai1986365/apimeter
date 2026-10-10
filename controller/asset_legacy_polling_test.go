package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestAssetLegacyClaimFollowupPollingIsReadOnly(t *testing.T) {
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		_, _ = w.Write([]byte(`{"Result":{"Id":"legacy-poll","TaskId":"legacy-task","GroupId":"legacy-group"}}`))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	first := seedanceCall(router, "GET", "/api/assets/legacy-poll", "", 1)
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	for kind, id := range map[string]string{"asset": "legacy-poll", "task": "legacy-task"} {
		owned, err := model.FindAssetBindings(1, kind, id)
		require.NoError(t, err)
		require.Len(t, owned, 1, "first use must persist the verified aliases")
	}

	var writes, rowLocks atomic.Int32
	db := model.DB
	recordWrite := func(tx *gorm.DB) {
		if tx.Statement.Table == "configurable_resource_states" || tx.Statement.Table == "retry_route_events" {
			writes.Add(1)
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:legacy_poll_create", recordWrite))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:legacy_poll_update", recordWrite))
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:legacy_poll_lock", func(tx *gorm.DB) {
		_, locking := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if strings.Contains(tx.Statement.SQL.String(), "configurable_resource_states") && locking {
			rowLocks.Add(1)
		}
	}))
	t.Cleanup(func() {
		db.Callback().Create().Remove("test:legacy_poll_create")
		db.Callback().Update().Remove("test:legacy_poll_update")
		db.Callback().Query().Remove("test:legacy_poll_lock")
	})
	before := upstreamCalls.Load()
	for range 5 {
		got := seedanceCall(router, "GET", "/api/assets/legacy-poll", "", 1)
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	}
	require.Equal(t, before+5, upstreamCalls.Load(), "registered polling must not repeat the verification probe")
	require.Zero(t, writes.Load(), "registered polling must not write ownership, mutex or empty retry-event rows")
	require.Zero(t, rowLocks.Load(), "registered polling must not acquire shared or exclusive row locks")
	denied := seedanceCall(router, "GET", "/api/assets/legacy-poll", "", 2)
	require.Equal(t, http.StatusNotFound, denied.Code)
	require.Equal(t, before+5, upstreamCalls.Load(), "another user cannot claim registered ownership")
}

func TestTgxMaasAssetPollingDoesNotRewriteMappings(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Result":{"Id":"asset-local","upstream_asset_id":"asset-original","ProjectName":"test-project"}}`))
	}))
	defer upstream.Close()
	router := assetSecurityRouter(t, upstream.URL, "tgxmaas", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "asset-local")
	warm := seedanceCall(router, "GET", "/api/assets/asset-local", "", 1)
	require.Equal(t, http.StatusOK, warm.Code, warm.Body.String())
	db := model.DB
	var before []model.ConfigurableResourceState
	require.NoError(t, db.Order("id").Find(&before).Error)
	var writes atomic.Int32
	countWrite := func(tx *gorm.DB) {
		if tx.Statement.Table == "configurable_resource_states" || tx.Statement.Table == "retry_route_events" {
			writes.Add(1)
		}
	}
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:tgx_poll_create", countWrite))
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:tgx_poll_update", countWrite))
	t.Cleanup(func() {
		db.Callback().Create().Remove("test:tgx_poll_create")
		db.Callback().Update().Remove("test:tgx_poll_update")
	})
	for range 5 {
		got := seedanceCall(router, "GET", "/api/assets/asset-local", "", 1)
		require.Equal(t, http.StatusOK, got.Code, got.Body.String())
	}
	var after []model.ConfigurableResourceState
	require.NoError(t, db.Order("id").Find(&after).Error)
	require.Equal(t, before, after)
	require.Zero(t, writes.Load(), "unchanged upstream IDs must not cause alias UPSERTs")
}
