package model

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetBindingLargeGroupRevocationBatches(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group", CanonicalID: "group"}
	bindings := []AssetBinding{group}
	for i := 0; i < 1000; i++ {
		asset := group
		asset.Kind, asset.ID = "asset", fmt.Sprintf("asset-%d", i)
		asset.CanonicalID, asset.GroupID = asset.ID, group.ID
		bindings = append(bindings, asset)
	}
	unrelated := group
	unrelated.Kind, unrelated.ID, unrelated.CanonicalID = "asset", "unrelated", "unrelated"
	require.NoError(t, SaveAssetBindings(append(bindings, unrelated)))
	observer.selects.Store(0)
	observer.updates.Store(0)
	observer.inserts.Store(0)
	observer.deadlocks.Store(0)
	require.NoError(t, InvalidateAssetBindings(group, true))
	statements := observer.selects.Load() + observer.updates.Load() + observer.inserts.Load()
	t.Logf("1000 assets + group: SELECT=%d UPDATE=%d INSERT=%d, total=%d", observer.selects.Load(), observer.updates.Load(), observer.inserts.Load(), statements)
	limit := int64(13) // Discovery/revalidation, batched identity locks and tombstone/binding updates.
	if DB.Dialector.Name() == "sqlite" {
		limit = 16 // SQLite also takes its write reservation before reading each lock batch.
	}
	require.LessOrEqual(t, statements, limit, "group deletion must use bounded batches, not one round trip per binding")
	require.Zero(t, observer.inserts.Load(), "existing mutexes must not be reinserted")
	require.Zero(t, observer.deadlocks.Load())
	remaining, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Equal(t, []AssetBinding{unrelated}, remaining)
	groups, err := FindAssetBindings(44, "group", group.ID)
	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestAssetBindingConcurrentBulkRevocationAndPolling(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	var groups []AssetBinding
	var bindings []AssetBinding
	var polls [][]AssetBinding
	for n := 0; n < 2; n++ {
		group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: fmt.Sprintf("group-%d", n)}
		group.CanonicalID = group.ID
		groups = append(groups, group)
		bindings = append(bindings, group)
		var poll []AssetBinding
		for i := 0; i < assetBindingBatchSize+2; i++ {
			asset := group
			asset.Kind, asset.ID = "asset", fmt.Sprintf("asset-%d-%d", n, i)
			asset.CanonicalID, asset.GroupID = asset.ID, group.ID
			bindings = append(bindings, asset)
			if i == 0 || i == assetBindingBatchSize+1 {
				poll = append([]AssetBinding{asset}, poll...) // Reverse binding PK order.
			}
		}
		polls = append(polls, poll)
	}
	require.NoError(t, SaveAssetBindings(bindings))
	observer.deadlocks.Store(0)
	runConcurrentAssetWrites(t, 8, 12, func(worker, round int) error {
		if worker < 2 && round == 3 {
			return InvalidateAssetBindings(groups[worker], true)
		}
		err := SaveAssetBindings(polls[worker%2])
		if errors.Is(err, ErrAssetNotOwned) {
			return nil
		}
		return err
	})
	require.Zero(t, observer.deadlocks.Load(), "bulk deletion and reverse-order polling must not deadlock")
	remaining, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Empty(t, remaining)
}

func TestAssetBindingConcurrentBatchInitialization(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	runConcurrentAssetWrites(t, 8, 1, func(worker, round int) error {
		bindings := make([]AssetBinding, 120)
		for i := range bindings {
			bindings[i] = AssetBinding{ChannelID: 91, UserID: worker + 1, Scope: "account", Kind: "asset", ID: fmt.Sprintf("contended-%d", i)}
		}
		err := ClaimLegacyAssetBindings(bindings)
		if errors.Is(err, ErrAssetNotOwned) {
			return nil
		}
		return err
	})
	var stored []ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Find(&stored).Error)
	require.Len(t, stored, 120)
	for _, state := range stored {
		require.Equal(t, stored[0].StateValue, state.StateValue, "all handles must belong to the same winning transaction")
	}
	require.Zero(t, observer.deadlocks.Load(), "batched first-use initialization must not reintroduce deadlocks")
}

func TestAssetBindingLegacyRevocationInitializesMutexesInBatches(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group", CanonicalID: "group"}
	bindings := []AssetBinding{group}
	for i := 0; i < 120; i++ {
		asset := group
		asset.Kind, asset.ID = "asset", fmt.Sprintf("legacy-%d", i)
		asset.CanonicalID, asset.GroupID = asset.ID, group.ID
		bindings = append(bindings, asset)
	}
	require.NoError(t, SaveAssetBindings(bindings))
	// Simulate bindings persisted before per-handle mutex records existed.
	require.NoError(t, DB.Where("profile_id = ?", assetBindingLockProfile).Delete(&ConfigurableResourceState{}).Error)
	observer.selects.Store(0)
	observer.updates.Store(0)
	observer.inserts.Store(0)
	require.NoError(t, InvalidateAssetBindings(group, true))
	statements := observer.selects.Load() + observer.updates.Load() + observer.inserts.Load()
	require.LessOrEqual(t, statements, int64(10), "legacy mutex initialization must also be batched")
	remaining, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Empty(t, remaining)
}

func TestAssetBindingPreservesSessionConfigurationMySQL(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires MySQL session configuration")
	}
	require.NoError(t, DB.Connection(func(conn *gorm.DB) error {
		original := DB
		DB = conn.Session(&gorm.Session{NewDB: true})
		defer func() { DB = original }()
		type configuration struct {
			BinlogFormat string
			Isolation    string
			LogBin       bool
			LockWait     int
		}
		var version string
		if err := DB.Raw("SELECT VERSION()").Scan(&version).Error; err != nil {
			return err
		}
		isolationVariable := "@@session.transaction_isolation"
		if strings.HasPrefix(version, "5.7.") {
			isolationVariable = "@@session.tx_isolation"
		}
		read := func(config *configuration) error {
			return DB.Raw("SELECT @@session.binlog_format AS binlog_format, @@global.log_bin AS log_bin, @@session.innodb_lock_wait_timeout AS lock_wait, " + isolationVariable + " AS isolation").Scan(config).Error
		}
		var before, after configuration
		if err := read(&before); err != nil {
			return err
		}
		asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "session-config", CanonicalID: "session-config"}
		if err := SaveAssetBindings([]AssetBinding{asset}); err != nil {
			return err
		}
		if err := InvalidateAssetBindings(asset, false); err != nil {
			return err
		}
		if err := read(&after); err != nil {
			return err
		}
		require.Equal(t, before, after, "asset writes must preserve the configured isolation and binlog mode")
		t.Logf("asset create/delete preserved log_bin=%t, binlog_format=%s", after.LogBin, after.BinlogFormat)
		return nil
	}))
}

func TestAssetBindingBulkRevocationDoesNotLockUnrelatedRowsMySQL(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires MySQL to verify batched PK range locks and query plans")
	}
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "delete-group", CanonicalID: "delete-group"}
	bindings := []AssetBinding{group}
	var keep AssetBinding
	for i := 0; i < assetBindingBatchSize+2; i++ {
		for _, target := range []bool{true, false} {
			asset := group
			asset.Kind, asset.ID = "asset", fmt.Sprintf("asset-%d-%t", i, target)
			asset.CanonicalID = asset.ID
			asset.GroupID = "keep-group"
			if target {
				asset.GroupID = group.ID
			} else {
				keep = asset
			}
			bindings = append(bindings, asset) // Interleave target and unrelated PKs.
		}
	}
	require.NoError(t, SaveAssetBindings(bindings))
	var mutexRows []ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ?", assetBindingLockProfile).Order("id").Limit(assetBindingBatchSize).Find(&mutexRows).Error)
	ids := make([]int, len(mutexRows))
	for i, row := range mutexRows {
		ids[i] = row.Id
	}
	stmt := lockForUpdate(assetBindingPrimaryRows(DB, ids)).Session(&gorm.Session{DryRun: true}).
		Select("id").Order("id").Find(&[]ConfigurableResourceState{}).Statement
	assertPrimaryIndex := func(sql string, args []any) {
		t.Helper()
		var plan []struct{ Table, Type, Key string }
		require.NoError(t, DB.Raw("EXPLAIN "+sql, args...).Scan(&plan).Error)
		for _, row := range plan {
			if row.Table == DB.NamingStrategy.TableName("ConfigurableResourceState") {
				require.Contains(t, []string{"const", "eq_ref"}, row.Type, "each ID must use a unique lookup, never an index scan")
				require.Equal(t, "PRIMARY", row.Key)
				return
			}
		}
		t.Fatal("missing ownership table in query plan")
	}
	assertPrimaryIndex(stmt.SQL.String(), stmt.Vars)
	type pausedStatement struct {
		sql  string
		args []any
	}
	paused := make(chan pausedStatement, 1)
	release := make(chan struct{})
	var once sync.Once
	var writers sync.WaitGroup
	var captured atomic.Bool
	require.NoError(t, DB.Callback().Update().After("gorm:update").Register("test:pause_bulk_revoke", func(tx *gorm.DB) {
		values, ok := tx.Statement.Dest.(map[string]any)
		if !ok || values["status"] != ConfigurableResourceStateStatusInvalid || tx.Error != nil || !captured.CompareAndSwap(false, true) {
			return
		}
		paused <- pausedStatement{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)}
		select {
		case <-release:
		case <-time.After(15 * time.Second):
			tx.AddError(fmt.Errorf("timed out waiting for unrelated write"))
		}
	}))
	defer func() {
		once.Do(func() { close(release) })
		writers.Wait()
		DB.Callback().Update().Remove("test:pause_bulk_revoke")
	}()
	revoked := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		revoked <- InvalidateAssetBindings(group, true)
	}()
	var statement pausedStatement
	select {
	case statement = <-paused:
	case <-time.After(15 * time.Second):
		t.Fatal("revocation did not reach its first batch")
	}
	// While deletion holds its identity locks, a neighboring asset must remain
	// writable and a brand-new binding must be insertable.
	unrelated := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		newAsset := keep
		newAsset.ID, newAsset.CanonicalID = "brand-new", "brand-new"
		unrelated <- SaveAssetBindings([]AssetBinding{keep, newAsset})
	}()
	select {
	case err := <-unrelated:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("bulk deletion locked an unrelated row or insertion gap")
	}
	once.Do(func() { close(release) })
	require.NoError(t, <-revoked)
	// MySQL's EXPLAIN UPDATE can wait on the updated row. Inspect the captured
	// statement after commit so the plan check cannot exhaust the held transaction.
	assertPrimaryIndex(statement.sql, statement.args)
	require.Zero(t, observer.deadlocks.Load())
}
