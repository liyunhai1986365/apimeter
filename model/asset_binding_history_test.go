package model

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetBindingOldPollingDoesNotMigrateRecords(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old-alias", CanonicalID: "canonical-without-handle", GroupID: "old-group"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	require.NoError(t, DB.Where("profile_id = ?", assetBindingLockProfile).Delete(&ConfigurableResourceState{}).Error)
	var before []ConfigurableResourceState
	require.NoError(t, DB.Order("id").Find(&before).Error)
	observer.inserts.Store(0)
	observer.updates.Store(0)
	observer.rowLocks.Store(0)
	for range 3 {
		require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	}
	var after []ConfigurableResourceState
	require.NoError(t, DB.Order("id").Find(&after).Error)
	require.Equal(t, before, after, "GET must not backfill markers, rewrite metadata or refresh timestamps")
	require.Zero(t, observer.inserts.Load())
	require.Zero(t, observer.updates.Load())
	require.Zero(t, observer.rowLocks.Load())

	// A historical revoked alias may be the only tombstone for a canonical
	// identity. The read-only shortcut must still honor it without backfill.
	var tombstone ConfigurableResourceState
	tombstone = before[0]
	tombstone.Id, tombstone.StateKey, tombstone.Status = 0, assetBindingKey("revoked-alias"), ConfigurableResourceStateStatusInvalid
	require.NoError(t, DB.Create(&tombstone).Error)
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetRevoked)
}

func TestAssetBindingDeletionAfterHistoryReadRejectsRegistration(t *testing.T) {
	for _, deleteGroup := range []bool{false, true} {
		t.Run(fmt.Sprintf("group=%t", deleteGroup), func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "parent", CanonicalID: "parent"}
			asset := group
			asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "original", "original", group.ID
			require.NoError(t, SaveAssetBindings([]AssetBinding{group, asset}))
			// Emulate old ownership without identity markers; registration must
			// pre-read historical tombstones, then recheck current locked state.
			require.NoError(t, DB.Where("profile_id = ?", assetBindingLockProfile).Delete(&ConfigurableResourceState{}).Error)
			fresh := asset
			fresh.ID = "late-alias"
			paused, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			var workers sync.WaitGroup
			var reads, inTransaction atomic.Int32
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:history_before_locks", func(tx *gorm.DB) {
				if !strings.Contains(tx.Statement.SQL.String(), "metadata LIKE") {
					return
				}
				if _, ok := tx.Statement.ConnPool.(gorm.TxCommitter); ok {
					inTransaction.Add(1)
				}
				if reads.Add(1) != 1 {
					return
				}
				close(paused)
				select {
				case <-release:
				case <-time.After(10 * time.Second):
					tx.AddError(fmt.Errorf("history barrier timed out"))
				}
			}))
			defer func() {
				once.Do(func() { close(release) })
				workers.Wait()
				DB.Callback().Query().Remove("test:history_before_locks")
			}()
			done := make(chan error, 1)
			workers.Add(1)
			go func() { defer workers.Done(); done <- SaveAssetBindings([]AssetBinding{fresh}) }()
			select {
			case <-paused:
			case <-time.After(10 * time.Second):
				t.Fatal("registration did not read history")
			}
			target := asset
			if deleteGroup {
				target = group
			}
			require.NoError(t, InvalidateAssetBindings(target, deleteGroup), "history parsing must not hold registration locks")
			once.Do(func() { close(release) })
			require.ErrorIs(t, <-done, ErrAssetRevoked)
			require.Zero(t, inTransaction.Load(), "historical scans must run before the ownership transaction")
			found, err := FindAssetBindings(fresh.UserID, fresh.Kind, fresh.ID)
			require.NoError(t, err)
			require.Empty(t, found)
		})
	}
}

func TestAssetBindingBatchConflictsRollback(t *testing.T) {
	for _, conflict := range []string{"channel", "task-owner", "claim-scope"} {
		t.Run(conflict, func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "shared", CanonicalID: "shared"}
			other := asset
			save := SaveAssetBindings
			switch conflict {
			case "channel":
				other.ChannelID++
			case "task-owner":
				other.Kind, other.UserID = "task", 45
			case "claim-scope":
				other.Scope = "other-account"
				save = ClaimLegacyAssetBindings
			}
			require.ErrorIs(t, save([]AssetBinding{asset, other}), ErrAssetNotOwned)
			var count int64
			require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("profile_id = ?", assetBindingProfile).Count(&count).Error)
			require.Zero(t, count, "a conflict inside one batch must roll back all its ownership rows")
		})
	}
}
