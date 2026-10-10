package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAssetBindingUnchangedPollingIsReadOnly(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "existing", CanonicalID: "existing", Project: "project", GroupID: "group"}
	alias := asset
	alias.ID = "alias"
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset, alias}))
	observer.inserts.Store(0)
	observer.updates.Store(0)
	observer.rowLocks.Store(0)
	runConcurrentAssetWrites(t, 20, 5, func(worker, round int) error {
		return SaveAssetBindings([]AssetBinding{asset, alias, asset})
	})
	require.Zero(t, observer.inserts.Load(), "polling must not create mutex or ownership records")
	require.Zero(t, observer.updates.Load(), "polling must not update ownership or mutex rows")
	require.Zero(t, observer.rowLocks.Load(), "unchanged assets must not acquire shared or exclusive row locks")
}

func TestAssetBindingHistoricalTupleRegistration(t *testing.T) {
	for _, kind := range []string{"asset", "task", "group"} {
		t.Run(kind, func(t *testing.T) {
			observer := setupAssetBindingConcurrencyTest(t)
			binding := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: kind, ID: "old-handle", CanonicalID: "old-handle"}
			row := insertAssetRevocationFilterFixture(t, binding, ConfigurableResourceStateStatusActive)
			require.NoError(t, DB.Model(&row).Updates(map[string]any{"user_id": 7, "token_id": 8}).Error)
			observer.inserts.Store(0)
			observer.updates.Store(0)
			observer.rowLocks.Store(0)
			for range 3 {
				require.NoError(t, SaveAssetBindings([]AssetBinding{binding}))
			}
			require.Zero(t, observer.inserts.Load())
			require.Zero(t, observer.updates.Load())
			require.Zero(t, observer.rowLocks.Load())
			// A genuinely new field must update the existing row in place, even
			// when registration enters its write transaction instead of the fast path.
			binding.Project = "discovered-project"
			require.NoError(t, SaveAssetBindings([]AssetBinding{binding}))
			var stored []ConfigurableResourceState
			require.NoError(t, DB.Where("profile_id = ? AND state_key = ?", assetBindingProfile, assetBindingKey(binding.ID)).Find(&stored).Error)
			require.Len(t, stored, 1)
			require.Equal(t, row.Id, stored[0].Id)
			require.Equal(t, 7, stored[0].UserID)
			require.Equal(t, 8, stored[0].TokenID)
			found, err := FindAssetBindings(binding.UserID, kind, binding.ID)
			require.NoError(t, err)
			require.Equal(t, []AssetBinding{binding}, found)
			other := binding
			other.UserID++
			require.ErrorIs(t, SaveAssetBindings([]AssetBinding{other}), ErrAssetNotOwned)
			require.NoError(t, InvalidateAssetBindings(binding, kind == "group"))
			require.ErrorIs(t, SaveAssetBindings([]AssetBinding{binding}), ErrAssetRevoked)
		})
	}
}

func TestAssetBindingPollingBypassesHeldGroupLockMySQL(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires actual MySQL row locks")
	}
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "existing", CanonicalID: "existing", GroupID: "group"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	var mutex ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ? AND state_key = ?", assetBindingLockProfile, assetLifecycleKey(asset, "group", asset.GroupID)).First(&mutex).Error)
	tx := DB.Begin()
	require.NoError(t, tx.Error)
	t.Cleanup(func() { _ = tx.Rollback().Error })
	require.NoError(t, lockForUpdate(tx).First(&mutex, mutex.Id).Error)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, SaveAssetBindingsContext(ctx, []AssetBinding{asset, asset}), "a held group lock must not block unchanged polling")

	// An actually new alias still coordinates with deletion, and cancellation
	// must release its connection instead of leaving an abandoned lock waiter.
	alias := asset
	alias.ID = "new-alias"
	for _, save := range []func(context.Context, []AssetBinding) error{SaveAssetBindingsContext, ClaimLegacyAssetBindingsContext} {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		err := save(ctx, []AssetBinding{alias, asset})
		cancel()
		require.ErrorIs(t, err, context.DeadlineExceeded)
	}
	require.NoError(t, tx.Rollback().Error)
	found, err := FindAssetBindings(asset.UserID, "asset", alias.ID)
	require.NoError(t, err)
	require.Empty(t, found)
	require.NoError(t, SaveAssetBindings([]AssetBinding{alias, asset}), "the same alias can be saved after the lock is released")
}

func TestAssetLegacyClaimChecksAllScopesBeforeAcceptingExistingBindings(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "shared", CanonicalID: "shared"}
	other := asset
	other.ChannelID, other.UserID, other.Scope = 92, 45, "other-account"
	// Creation permits provider-local IDs reused by unrelated accounts.
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	require.NoError(t, SaveAssetBindings([]AssetBinding{other}))
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	// A historical claim cannot use the scoped read-only shortcut: its global
	// conflict check must still reject the other registered account.
	require.ErrorIs(t, ClaimLegacyAssetBindings([]AssetBinding{asset}), ErrAssetNotOwned)
}

func TestAssetBindingReadOnlyPollingChecksRevocationAndMetadata(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "existing", CanonicalID: "existing", Project: "project", GroupID: "group"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	conflict := asset
	conflict.UserID++
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{conflict}), ErrAssetNotOwned)
	conflict = asset
	conflict.GroupID = "different-group"
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{conflict}), ErrAssetNotOwned)
	require.NoError(t, InvalidateAssetBindings(asset, false))
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetNotOwned)
}
