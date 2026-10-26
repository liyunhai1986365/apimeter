package model

import (
	"context"
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

func observeAssetRepairScans(t *testing.T) *atomic.Int64 {
	t.Helper()
	var scans atomic.Int64
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:repair_scans", func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "metadata LIKE") {
			return
		}
		scans.Add(1)
		if _, transaction := tx.Statement.ConnPool.(gorm.TxCommitter); transaction {
			tx.AddError(errors.New("repair scanned history inside a transaction"))
		}
	}))
	t.Cleanup(func() { DB.Callback().Query().Remove("test:repair_scans") })
	return &scans
}

func TestAssetBindingRepairPersistsFamilyAndParents(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group-alias", CanonicalID: "canonical-group"}
	asset := group
	asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "asset-alias", "canonical-asset", group.ID
	task := asset
	task.Kind, task.ID, task.GroupID = "task", "task-alias", ""
	for _, binding := range []AssetBinding{group, asset, task} {
		row := insertAssetRevocationFilterFixture(t, binding, ConfigurableResourceStateStatusActive)
		require.NoError(t, DB.Model(&row).Updates(map[string]any{"user_id": 7, "token_id": 8}).Error)
	}
	other := asset
	other.Scope = "other-account"
	insertAssetRevocationFilterFixture(t, other, ConfigurableResourceStateStatusInvalid)
	var before, after []ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Order("id").Find(&before).Error)
	scans := observeAssetRepairScans(t)

	// Even an alias without GroupID must inherit the whole family's parents.
	found, err := FindAssetBindings(task.UserID, task.Kind, task.ID)
	require.NoError(t, err)
	require.Equal(t, []AssetBinding{task}, found)
	require.Positive(t, scans.Load())
	refs := make(map[string]AssetBinding)
	assetIdentityRefs(refs, task)
	history, err := loadAssetBindingHistory(DB, refs)
	require.NoError(t, err)
	require.Len(t, history, 3)
	for _, node := range history {
		require.Equal(t, assetLifecycleRelated, node.state.StateValue)
	}

	scans.Store(0)
	observer.inserts.Store(0)
	observer.updates.Store(0)
	observer.rowLocks.Store(0)
	// Reopening the GORM session demonstrates that completion is durable.
	DB = DB.Session(&gorm.Session{NewDB: true})
	for range 3 {
		found, err = FindAssetBindings(task.UserID, task.Kind, task.ID)
		require.NoError(t, err)
		require.Equal(t, []AssetBinding{task}, found)
	}
	require.Zero(t, scans.Load(), "complete relationships must not rediscover history")
	require.Zero(t, observer.inserts.Load())
	require.Zero(t, observer.updates.Load())
	require.Zero(t, observer.rowLocks.Load())
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Order("id").Find(&after).Error)
	require.Equal(t, before, after)

	// A subsequently lost relationship falls back again, even after earlier repair.
	key := assetLifecycleKey(task, task.Kind, task.CanonicalID)
	require.NoError(t, assetIdentityStates(DB, []string{key}).Delete(&ConfigurableResourceState{}).Error)
	found, err = FindAssetBindings(task.UserID, task.Kind, task.ID)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Positive(t, scans.Load())
	scans.Store(0)
	require.NoError(t, InvalidateAssetBindings(group, true))
	found, err = FindAssetBindings(task.UserID, task.Kind, task.ID)
	require.NoError(t, err)
	require.Empty(t, found, "the repaired task must still follow parent revocation")
	require.Zero(t, scans.Load())
}

func TestAssetBindingRepairPersistsHistoricalTombstone(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "active-alias", CanonicalID: "family"}
	insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	old := asset
	old.Kind, old.ID = "task", "revoked-alias"
	row := insertAssetRevocationFilterFixture(t, old, ConfigurableResourceStateStatusInvalid)
	require.NoError(t, DB.Model(&row).Updates(map[string]any{"user_id": 7, "token_id": 8}).Error)
	scans := observeAssetRepairScans(t)
	found, err := FindAssetBindings(asset.UserID, asset.Kind, asset.ID)
	require.NoError(t, err)
	require.Empty(t, found)
	require.Positive(t, scans.Load())
	var identity ConfigurableResourceState
	require.NoError(t, assetIdentityStates(DB, []string{assetLifecycleKey(asset, asset.Kind, asset.CanonicalID)}).First(&identity).Error)
	require.Equal(t, ConfigurableResourceStateStatusInvalid, identity.Status)
	scans.Store(0)
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetRevoked)
	asset.ID = "newly-discovered-alias"
	require.ErrorIs(t, ClaimLegacyAssetBindings([]AssetBinding{asset}), ErrAssetRevoked)
	require.Zero(t, scans.Load(), "a repaired tombstone must reject aliases without another historical scan")
}

func TestAssetBindingRepairConcurrentMissingRelations(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family", GroupID: "parent"}
	insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	runConcurrentAssetWrites(t, 16, 3, func(_, _ int) error {
		found, err := FindAssetBindings(asset.UserID, asset.Kind, asset.ID)
		if err == nil && len(found) != 1 {
			return fmt.Errorf("repair returned %d bindings", len(found))
		}
		return err
	})
	require.Zero(t, observer.deadlocks.Load())
	var identities int64
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).
		Where("profile_id = ? AND state_value = ?", assetBindingLockProfile, assetLifecycleRelated).Count(&identities).Error)
	require.EqualValues(t, 2, identities)
}

func TestAssetBindingRepairDeletionAfterScanCannotRevive(t *testing.T) {
	for _, deleteGroup := range []bool{false, true} {
		t.Run(fmt.Sprintf("group=%t", deleteGroup), func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family", GroupID: "parent"}
			insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
			paused, release := make(chan struct{}), make(chan struct{})
			var captured atomic.Bool
			var unblock sync.Once
			var workers sync.WaitGroup
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:repair_delete_race", func(tx *gorm.DB) {
				if !strings.Contains(tx.Statement.SQL.String(), "metadata LIKE") || !captured.CompareAndSwap(false, true) {
					return
				}
				close(paused)
				select {
				case <-release:
				case <-time.After(10 * time.Second):
					tx.AddError(errors.New("repair barrier timed out"))
				}
			}))
			defer func() {
				unblock.Do(func() { close(release) })
				workers.Wait()
				DB.Callback().Query().Remove("test:repair_delete_race")
			}()
			type result struct {
				found []AssetBinding
				err   error
			}
			done := make(chan result, 1)
			workers.Add(1)
			go func() {
				defer workers.Done()
				found, err := FindAssetBindings(asset.UserID, asset.Kind, asset.ID)
				done <- result{found, err}
			}()
			select {
			case <-paused:
			case <-time.After(10 * time.Second):
				t.Fatal("repair did not reach history discovery")
			}
			target := asset
			if deleteGroup {
				target.Kind, target.ID, target.CanonicalID = "group", asset.GroupID, asset.GroupID
			}
			require.NoError(t, InvalidateAssetBindings(target, deleteGroup))
			unblock.Do(func() { close(release) })
			got := <-done
			require.NoError(t, got.err)
			require.Empty(t, got.found)
			var identity ConfigurableResourceState
			require.NoError(t, assetIdentityStates(DB, []string{assetLifecycleKey(target, target.Kind, target.CanonicalID)}).First(&identity).Error)
			require.Equal(t, ConfigurableResourceStateStatusInvalid, identity.Status)
		})
	}
}

func TestAssetBindingRepairFailureRollsBackAndCanRetry(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("canceled=%t", canceled), func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family", GroupID: "parent"}
			insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("injected relationship persistence failure")
			var updates atomic.Int32
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:repair_rollback", func(tx *gorm.DB) {
				values, ok := tx.Statement.Dest.(map[string]any)
				if !ok || values["state_value"] != assetLifecycleRelated || updates.Add(1) != 2 {
					return
				}
				if canceled {
					cancel()
				} else {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { DB.Callback().Update().Remove("test:repair_rollback") })
			found, err := FindAssetBindingsContext(ctx, asset.UserID, asset.Kind, asset.ID)
			if canceled {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, failure)
			}
			require.Empty(t, found, "failed repair must not grant access")
			var completed int64
			require.NoError(t, DB.Model(&ConfigurableResourceState{}).
				Where("profile_id = ? AND state_value = ?", assetBindingLockProfile, assetLifecycleRelated).Count(&completed).Error)
			require.Zero(t, completed, "completion markers must commit atomically")
			require.NoError(t, DB.Callback().Update().Remove("test:repair_rollback"))
			found, err = FindAssetBindings(asset.UserID, asset.Kind, asset.ID)
			require.NoError(t, err)
			require.Equal(t, []AssetBinding{asset}, found)
		})
	}
}

func TestAssetBindingRepairConcurrentParentChangeUsesCurrentRelations(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family"}
	insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	key := assetLifecycleKey(asset, asset.Kind, asset.CanonicalID)
	_, err := prepareAssetBindingLocks(DB, assetBindingLockPlan{key: assetBindingExclusiveLock})
	require.NoError(t, err)
	var changed atomic.Bool
	// Lock planning runs after history discovery, but before the transaction.
	// Another writer now attaches and revokes a parent; repair must reload it.
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:repair_parent_change", func(tx *gorm.DB) {
		if strings.Join(tx.Statement.Selects, ",") != "id,state_key,state_value" || !changed.CompareAndSwap(false, true) {
			return
		}
		hydrated := asset
		hydrated.GroupID = "late-parent"
		if err := SaveAssetBindings([]AssetBinding{hydrated}); err != nil {
			tx.AddError(err)
			return
		}
		parent := asset
		parent.Kind, parent.ID, parent.CanonicalID = "group", hydrated.GroupID, hydrated.GroupID
		tx.AddError(InvalidateAssetBindings(parent, true))
	}))
	t.Cleanup(func() { DB.Callback().Query().Remove("test:repair_parent_change") })
	found, err := FindAssetBindings(asset.UserID, asset.Kind, asset.ID)
	require.NoError(t, err)
	require.True(t, changed.Load())
	require.Empty(t, found, "repair cannot overwrite a concurrently attached parent")
	var identity ConfigurableResourceState
	require.NoError(t, assetIdentityStates(DB, []string{key}).First(&identity).Error)
	require.JSONEq(t, `{"parents":["late-parent"]}`, identity.Metadata)
}

func TestAssetBindingRepairDoesNotClaimUnknownOrOtherUsers(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family"}
	insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	observer.inserts.Store(0)
	observer.updates.Store(0)
	for _, request := range []struct {
		user int
		id   string
	}{{44, "unknown"}, {45, asset.ID}} {
		found, err := FindAssetBindings(request.user, asset.Kind, request.id)
		require.NoError(t, err)
		require.Empty(t, found)
	}
	require.Zero(t, observer.inserts.Load())
	require.Zero(t, observer.updates.Load())
}
