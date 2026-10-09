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
	"gorm.io/gorm/clause"
)

func TestAssetLifecycleDeleteIncludesAliasesCommittedAfterDiscovery(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		group bool
	}{{"asset_aliases", false}, {"group_aliases", true}, {"new_child", true}, {"hydrate_child", true}, {"new_group_alias", true}} {
		t.Run(scenario.name, func(t *testing.T) {
			deleteGroup := scenario.group
			setupAssetBindingConcurrencyTest(t)
			group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group", CanonicalID: "group"}
			asset := group
			asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "original", "original", "group"
			require.NoError(t, SaveAssetBindings([]AssetBinding{group, asset}))
			lateChild := asset
			lateChild.ID, lateChild.CanonicalID, lateChild.GroupID = "late-child", "late-child", ""
			if scenario.name == "hydrate_child" {
				require.NoError(t, SaveAssetBindings([]AssetBinding{lateChild}))
			}
			target := asset
			if deleteGroup {
				target = group
			}
			paused := make(chan struct{})
			release := make(chan struct{})
			var captured atomic.Bool
			var once sync.Once
			var writers sync.WaitGroup
			require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:pause_delete_discovery", func(tx *gorm.DB) {
				// Inspect the original clause: PostgreSQL renders placeholders as
				// $1, $2, ... while MySQL and SQLite keep question marks.
				where, ok := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
				matched := false
				if ok {
					for _, expression := range where.Exprs {
						if predicate, ok := expression.(clause.Expr); ok && strings.Contains(predicate.SQL, "channel_id = ? AND profile_id = ? AND pre_request_id = ? AND state_value = ?") {
							matched = true
						}
					}
				}
				if !matched || !captured.CompareAndSwap(false, true) {
					return
				}
				close(paused)
				select {
				case <-release:
				case <-time.After(10 * time.Second):
					tx.AddError(fmt.Errorf("timed out at deletion discovery barrier"))
				}
			}))
			defer func() {
				once.Do(func() { close(release) })
				writers.Wait()
				DB.Callback().Query().Remove("test:pause_delete_discovery")
			}()
			done := make(chan error, 1)
			writers.Add(1)
			go func() {
				defer writers.Done()
				done <- InvalidateAssetBindings(target, deleteGroup)
			}()
			select {
			case <-paused:
			case <-time.After(10 * time.Second):
				t.Fatal("deletion did not reach discovery barrier")
			}
			alias := asset
			alias.ID = "discovered-alias"
			task := asset
			task.Kind, task.ID = "task", "discovered-task"
			// A get response saves both the original handle and its newly returned
			// aliases, just like controller.rememberAssetResponse.
			expected := []AssetBinding{asset, alias, task}
			switch scenario.name {
			case "new_child", "hydrate_child":
				lateChild.GroupID = group.ID
				require.NoError(t, SaveAssetBindings([]AssetBinding{lateChild}))
				expected = []AssetBinding{asset, lateChild}
			case "new_group_alias":
				groupAlias := group
				groupAlias.ID = "new-group-alias"
				lateChild.GroupID = groupAlias.ID
				require.NoError(t, SaveAssetBindings([]AssetBinding{groupAlias, lateChild}))
				expected = []AssetBinding{groupAlias, lateChild, asset}
			default:
				require.NoError(t, SaveAssetBindings([]AssetBinding{alias, task, asset}))
			}
			once.Do(func() { close(release) })
			require.NoError(t, <-done)
			for _, binding := range expected {
				found, err := FindAssetBindings(binding.UserID, binding.Kind, binding.ID)
				require.NoError(t, err)
				if len(found) != 0 {
					t.Errorf("delete succeeded but %s %q is still active", binding.Kind, binding.ID)
				}
			}
		})
	}
}

func TestAssetLifecycleDeletedGroupRejectsLateChildren(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group", CanonicalID: "group"}
			asset := group
			asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "late-child", "late-child", ""
			require.NoError(t, SaveAssetBindings([]AssetBinding{group}))
			if existing {
				require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
			}
			require.NoError(t, InvalidateAssetBindings(group, true))
			// An upstream create/get that started before group deletion returns
			// afterward. The registration must check the parent's current state.
			asset.GroupID = group.ID
			err := SaveAssetBindings([]AssetBinding{asset})
			require.ErrorIs(t, err, ErrAssetNotOwned, "a deleted group must reject late registration and metadata hydration")
		})
	}
}

func TestAssetLifecycleRevokedCanonicalRejectsFreshHandle(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "old-handle", CanonicalID: "canonical-without-own-handle"}
	// Supplier-local canonical IDs do not always have their own ownership row.
	// Revocation must cover the resource identity, not only observed handles.
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	require.NoError(t, InvalidateAssetBindings(asset, false))
	asset.ID = "fresh-handle"
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetNotOwned)
}

func TestAssetLifecycleGroupDeletionIncludesUnhydratedAliases(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group-alias", CanonicalID: "group-canonical"}
	asset := group
	asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "asset", "known", "asset-canonical", group.CanonicalID
	alias := asset
	alias.ID, alias.GroupID = "unhydrated-alias", ""
	task := asset
	task.Kind, task.ID, task.GroupID = "task", "processing", group.ID
	require.NoError(t, SaveAssetBindings([]AssetBinding{group, asset, alias, task}))
	require.NoError(t, InvalidateAssetBindings(group, true))
	for _, item := range []AssetBinding{group, asset, alias, task} {
		found, err := FindAssetBindings(item.UserID, item.Kind, item.ID)
		require.NoError(t, err)
		require.Empty(t, found, "one hydrated alias must be enough to delete its entire family")
	}
	for _, parent := range []string{group.ID, group.CanonicalID} {
		child := asset
		child.ID, child.CanonicalID, child.GroupID = "late-"+parent, "late-"+parent, parent
		require.ErrorIs(t, SaveAssetBindings([]AssetBinding{child}), ErrAssetNotOwned)
	}
	alias.ID = "fresh-unhydrated-alias"
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{alias}), ErrAssetNotOwned, "the child's identity remains revoked even without a parent in the request")
}

func TestAssetLifecycleHistoricalRevocationWithoutIdentityRows(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "old-group-alias", CanonicalID: "group-without-handle"}
	asset := group
	asset.Kind, asset.ID, asset.CanonicalID, asset.GroupID = "task", "old-task", "asset-without-handle", group.ID
	require.NoError(t, SaveAssetBindings([]AssetBinding{group, asset}))
	// Simulate pre-lifecycle rows, where only aliases retained the tombstones.
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("profile_id = ?", assetBindingProfile).
		Update("status", ConfigurableResourceStateStatusInvalid).Error)
	require.NoError(t, DB.Where("profile_id = ? AND state_key LIKE ?", assetBindingLockProfile, "identity:%").Delete(&ConfigurableResourceState{}).Error)
	asset.Kind, asset.ID, asset.GroupID = "asset", "new-alias", ""
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetNotOwned)
	for _, parent := range []string{group.ID, group.CanonicalID} {
		child := asset
		child.ID, child.CanonicalID, child.GroupID = "new-child-"+parent, "new-child-"+parent, parent
		require.ErrorIs(t, SaveAssetBindings([]AssetBinding{child}), ErrAssetNotOwned)
	}
	group.ID = "new-group-alias"
	require.ErrorIs(t, ClaimLegacyAssetBindings([]AssetBinding{group}), ErrAssetNotOwned)
	asset.ID, asset.Scope = "independent-account-alias", "other-account"
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}), "revocation must not leak into another account")
}

func TestAssetLifecycleStoredDependenciesExpandAsOnePlan(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	bindings := make([]AssetBinding, 120)
	for i := range bindings {
		bindings[i] = AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: fmt.Sprintf("alias-%d", i), CanonicalID: fmt.Sprintf("canonical-%d", i), GroupID: "parent"}
	}
	require.NoError(t, SaveAssetBindings(bindings))
	polled := append([]AssetBinding(nil), bindings...)
	for i := range polled {
		polled[i].CanonicalID, polled[i].GroupID = "new-"+polled[i].ID, ""
	}
	require.NoError(t, SaveAssetBindings(polled), "all stored canonical/parent dependencies must be discovered together within the retry limit")
	found, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.ElementsMatch(t, bindings, found, "polling must preserve established canonical IDs and parent groups")
}

func TestAssetLifecycleNewAliasesUseStoredCanonical(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("legacy=%t", legacy), func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			task := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "task", ID: "upload-task", CanonicalID: "upload-task"}
			require.NoError(t, SaveAssetBindings([]AssetBinding{task}))
			// A detail response for a previously unknown final ID also returns
			// the already registered task. Both must retain the task's identity.
			asset := task
			asset.Kind, asset.ID, asset.CanonicalID = "asset", "finished", "finished"
			polledTask := task
			polledTask.CanonicalID = asset.CanonicalID
			register := SaveAssetBindings
			if legacy {
				register = ClaimLegacyAssetBindings
			}
			require.NoError(t, register([]AssetBinding{asset, polledTask}))
			found, err := FindAssetBindings(task.UserID, asset.Kind, asset.ID)
			require.NoError(t, err)
			require.Len(t, found, 1)
			require.Equal(t, task.CanonicalID, found[0].CanonicalID)
			require.NoError(t, InvalidateAssetBindings(found[0], false))
			remaining, err := FindAssetBindings(task.UserID, task.Kind, task.ID)
			require.NoError(t, err)
			require.Empty(t, remaining, "deleting the final ID must also revoke the original task")
			fresh := task
			fresh.ID = "late-alias"
			require.ErrorIs(t, SaveAssetBindings([]AssetBinding{fresh}), ErrAssetNotOwned)
		})
	}
}

func TestAssetLifecycleRejectsMergingEstablishedIdentities(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	first := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "first", CanonicalID: "first"}
	second := first
	second.ID, second.CanonicalID = "second", "second"
	require.NoError(t, SaveAssetBindings([]AssetBinding{first, second}))
	alias := first
	alias.ID = "new-alias"
	second.CanonicalID = first.CanonicalID
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{alias, first, second}), ErrAssetNotOwned)
	exists, err := HasAssetBinding(alias.Kind, alias.ID)
	require.NoError(t, err)
	require.False(t, exists, "an ambiguous alias family must not leave partial registration")
	found, err := FindAssetBindings(second.UserID, second.Kind, second.ID)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, "second", found[0].CanonicalID)
}

func TestAssetLifecycleLateRegistrationRollsBackWholeAliasSet(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "parent", CanonicalID: "parent"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{group}))
	require.NoError(t, InvalidateAssetBindings(group, true))
	valid := group
	valid.Kind, valid.ID, valid.CanonicalID = "asset", "independent", "independent"
	late := valid
	late.ID, late.CanonicalID, late.GroupID = "late", "late", group.ID
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{valid, late}), ErrAssetNotOwned)
	exists, err := HasAssetBinding(valid.Kind, valid.ID)
	require.NoError(t, err)
	require.False(t, exists, "a rejected parent must roll back earlier inserts in the batch")
	require.NoError(t, SaveAssetBindings([]AssetBinding{valid}))
	otherChannel := late
	otherChannel.ChannelID, otherChannel.ID = 92, "other-channel"
	require.NoError(t, SaveAssetBindings([]AssetBinding{otherChannel}), "independent channels must retain their own lifecycle scope")
}
