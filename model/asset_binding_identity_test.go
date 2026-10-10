package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetGroupRevocationLeavesChildrenUnchanged(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "alias-group", CanonicalID: "canonical-group"}
	bindings := []AssetBinding{group}
	for i := 0; i < 24; i++ {
		asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: fmt.Sprint("child-", i), GroupID: group.ID}
		asset.CanonicalID = asset.ID
		bindings = append(bindings, asset)
	}
	require.NoError(t, SaveAssetBindings(bindings))
	var before, after []ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Order("id").Find(&before).Error)
	var metadataScans int
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:group_revocation_scans", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "metadata LIKE") {
			metadataScans++
		}
	}))
	require.NoError(t, InvalidateAssetBindings(group, true))
	DB.Callback().Query().Remove("test:group_revocation_scans")
	require.Zero(t, metadataScans, "deleting a group must not discover or scan its child families")
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Order("id").Find(&after).Error)
	require.Equal(t, before, after, "revocation must not rewrite historical group/child ownership")
	// Public reads use durable database markers, even on a fresh GORM session.
	DB = DB.Session(&gorm.Session{NewDB: true})
	active, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Empty(t, active)
	for _, binding := range bindings {
		found, registered, err := FindAssetAccessBindingsContext(context.Background(), 44, binding.Kind, binding.ID, true)
		require.NoError(t, err)
		require.True(t, registered, "revoked children cannot become historical-claim candidates")
		require.Empty(t, found)
	}
}

func TestAssetGroupRevocationCoversHistoricalUnhydratedAlias(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "old-parent-alias", CanonicalID: "parent-without-handle"}
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old-child", CanonicalID: "child-without-handle", GroupID: group.ID}
	alias := asset
	alias.Kind, alias.ID, alias.GroupID = "task", "unhydrated-task", ""
	for _, binding := range []AssetBinding{group, asset, alias} {
		row := insertAssetRevocationFilterFixture(t, binding, ConfigurableResourceStateStatusActive)
		require.NoError(t, DB.Model(&row).Updates(map[string]any{"user_id": 8, "token_id": 9}).Error)
	}
	require.NoError(t, InvalidateAssetBindings(group, true))
	found, err := FindAssetBindings(44, "task", alias.ID)
	require.NoError(t, err)
	require.Empty(t, found)
	alias.Kind, alias.ID = "asset", "newly-seen-alias"
	require.ErrorIs(t, ClaimLegacyAssetBindings([]AssetBinding{alias}), ErrAssetRevoked)
}

func TestAssetIdentityHistoryTargetsFamiliesAndEscapedTombstones(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old-alias", CanonicalID: "素材!%_canonical"}
	row := insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusInvalid)
	require.NoError(t, DB.Model(&row).Updates(map[string]any{
		"metadata": strings.ReplaceAll(row.Metadata, "素材", `\u7d20\u6750`), "user_id": 7, "token_id": 8,
	}).Error)
	for i := 0; i < 32; i++ {
		other := asset
		other.ID, other.CanonicalID = fmt.Sprint("unrelated-", i), fmt.Sprint("unrelated-", i)
		insertAssetRevocationFilterFixture(t, other, ConfigurableResourceStateStatusInvalid)
	}
	var returned int64
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:history_candidates", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "metadata LIKE") {
			returned += tx.RowsAffected
			_, transaction := tx.Statement.ConnPool.(gorm.TxCommitter)
			require.False(t, transaction, "compatibility discovery cannot hold transaction locks")
		}
	}))
	t.Cleanup(func() { DB.Callback().Query().Remove("test:history_candidates") })
	asset.ID = "new-alias"
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetRevoked)
	require.EqualValues(t, 1, returned, "only the relevant escaped tombstone is returned/decoded")
}

func TestAssetIdentityHistoryChecksExpandedStoredCanonical(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	task := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "task", ID: "upload-task", CanonicalID: "old-canonical"}
	row := insertAssetRevocationFilterFixture(t, task, ConfigurableResourceStateStatusActive)
	require.NoError(t, DB.Model(&row).Updates(map[string]any{"user_id": 8, "token_id": 9}).Error)
	old := task
	old.ID = "deleted-alias"
	insertAssetRevocationFilterFixture(t, old, ConfigurableResourceStateStatusInvalid)
	task.CanonicalID = "new-canonical"
	asset := task
	asset.Kind, asset.ID = "asset", "new-final-handle"
	require.ErrorIs(t, ClaimLegacyAssetBindings([]AssetBinding{asset, task}), ErrAssetRevoked,
		"a newly discovered canonical identity must be checked before its marker is initialized")
	registered, err := HasAssetBinding("asset", asset.ID)
	require.NoError(t, err)
	require.False(t, registered)
}

func TestAssetIdentityParentChangedAfterSnapshotCannotRevive(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	key := assetLifecycleKey(asset, "asset", asset.CanonicalID)
	changed := false
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:change_identity_after_snapshot", func(tx *gorm.DB) {
		states, ok := tx.Statement.Dest.(*[]ConfigurableResourceState)
		if !ok || changed || tx.Error != nil {
			return
		}
		for _, state := range *states {
			if state.StateKey != key {
				continue
			}
			changed = true
			hydrated := asset
			hydrated.GroupID = "late-parent"
			if err := SaveAssetBindings([]AssetBinding{hydrated}); err != nil {
				tx.AddError(err)
				return
			}
			parent := asset
			parent.Kind, parent.ID, parent.CanonicalID = "group", "late-parent", "late-parent"
			tx.AddError(InvalidateAssetBindings(parent, true))
			return
		}
	}))
	t.Cleanup(func() { DB.Callback().Query().Remove("test:change_identity_after_snapshot") })
	alias := asset
	alias.ID = "new-alias-without-group"
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{alias}), ErrAssetRevoked)
	require.True(t, changed)
	registered, err := HasAssetBinding("asset", alias.ID)
	require.NoError(t, err)
	require.False(t, registered)
}

func TestAssetIdentityV1PollingDoesNotBackfillParents(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old", CanonicalID: "family", GroupID: "parent"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("profile_id = ?", assetBindingLockProfile).
		Updates(map[string]any{"state_value": assetLifecycleInitialized, "metadata": ""}).Error)
	var before, after []ConfigurableResourceState
	require.NoError(t, DB.Order("id").Find(&before).Error)
	observer.inserts.Store(0)
	observer.updates.Store(0)
	observer.rowLocks.Store(0)
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	found, err := FindAssetBindings(44, "asset", asset.ID)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.NoError(t, DB.Order("id").Find(&after).Error)
	require.Equal(t, before, after)
	require.Zero(t, observer.inserts.Load())
	require.Zero(t, observer.updates.Load())
	require.Zero(t, observer.rowLocks.Load())
}
