package model

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetAccessLookupDistinguishesUnknownRevokedAndTaskHandles(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	task := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "task", ID: "upload", CanonicalID: "upload"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{task}))
	for _, tc := range []struct {
		user      int
		allowTask bool
		want      int
	}{{44, true, 1}, {44, false, 0}, {55, true, 0}} {
		observer.selects.Store(0)
		found, registered, err := FindAssetAccessBindingsContext(context.Background(), tc.user, "asset", task.ID, tc.allowTask)
		require.NoError(t, err)
		require.True(t, registered, "a task handle cannot be claimed again by another user or a non-task operation")
		require.Len(t, found, tc.want)
		require.EqualValues(t, 2+tc.want, observer.selects.Load(), "task fallback shares the handle lookup; active results also check revocation")
	}
	require.NoError(t, InvalidateAssetBindings(task, false))
	observer.selects.Store(0)
	found, registered, err := FindAssetAccessBindingsContext(context.Background(), 44, "asset", task.ID, true)
	require.NoError(t, err)
	require.True(t, registered, "revoked handles remain ineligible for historical claiming")
	require.Empty(t, found)
	require.EqualValues(t, 3, observer.selects.Load(), "revocation is checked on the identity without rewriting the task handle")
	found, registered, err = FindAssetAccessBindingsContext(context.Background(), 44, "asset", "unknown", true)
	require.NoError(t, err)
	require.False(t, registered)
	require.Empty(t, found)
}

func TestAssetBindingCandidateLookupIncludesNewScopesAndBatchedHandles(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	var want []int
	for i := 0; i < 12; i++ {
		row := ConfigurableResourceState{ChannelID: 91 + i%3, ProfileID: assetBindingProfile,
			ResourceID: []string{"asset", "task"}[i%2], PreRequestID: fmt.Sprintf("account-%d", i),
			UserID: i % 2, TokenID: i % 3, StateKey: assetBindingKey(fmt.Sprintf("handle-%d", i%2)),
			StateValue: "44", Status: ConfigurableResourceStateStatusInvalid}
		require.NoError(t, DB.Create(&row).Error)
		want = append(want, row.Id)
		found, err := assetHandleCandidateIDs(DB, []string{"asset", "task"}, []string{assetBindingKey("handle-0"), assetBindingKey("handle-1")})
		require.NoError(t, err)
		require.ElementsMatch(t, want, found, "each lookup must see newly added accounts, including revoked and legacy scope tuples")
	}
	for _, digests := range [][]string{nil, {assetBindingKey("unknown")}} {
		found, err := assetHandleCandidateIDs(DB, []string{"asset", "task"}, digests)
		require.NoError(t, err)
		require.Empty(t, found)
	}
}

func TestAssetBindingCandidateLookupMySQLUsesCompleteIndex(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != common.DatabaseTypeMySQL {
		t.Skip("requires MySQL optimizer")
	}
	for start := 0; start < 12000; start += 200 {
		rows := make([]ConfigurableResourceState, 200)
		for i := range rows {
			rows[i] = ConfigurableResourceState{ChannelID: 91, ProfileID: assetBindingProfile, ResourceID: "asset", PreRequestID: "account", StateKey: assetBindingKey(fmt.Sprint(start + i))}
		}
		require.NoError(t, DB.Create(&rows).Error)
	}
	require.NoError(t, DB.Exec("ANALYZE TABLE configurable_resource_states").Error)
	for _, handles := range [][]string{{assetBindingKey("42")}, {assetBindingKey("42"), assetBindingKey("43")}} {
		stmt := assetHandleCandidateQuery(DB, []string{"asset", "task"}, handles).
			Session(&gorm.Session{DryRun: true}).Pluck("id", new([]int)).Statement
		var plan []struct {
			Table string `gorm:"column:table"`
			Type  string `gorm:"column:type"`
			Ref   string `gorm:"column:ref"`
			Extra string `gorm:"column:Extra"`
		}
		require.NoError(t, DB.Raw("EXPLAIN "+stmt.SQL.String(), stmt.Vars...).Scan(&plan).Error)
		var loose, exact bool
		for _, step := range plan {
			loose = loose || strings.Contains(step.Extra, "Using index for group-by")
			if step.Table == "asset_candidates" {
				exact = true
				require.Contains(t, []string{"ref", "eq_ref"}, step.Type)
				require.Len(t, strings.Split(step.Ref, ","), 7, "all seven columns must participate, including batched digests")
			}
		}
		require.True(t, loose, "prefix enumeration must skip repeated scope entries: %+v", plan)
		require.True(t, exact)
		ids, err := assetHandleCandidateIDs(DB, []string{"asset", "task"}, handles)
		require.NoError(t, err)
		require.Len(t, ids, len(handles))
	}
}

func TestAssetBindingScopedLookupIsolation(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "group", CanonicalID: "canonical", Project: "project"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{group}))
	for _, tc := range []struct {
		channel, user int
		scope, handle string
		want          int
	}{{91, 44, "account", "group", 1}, {92, 44, "account", "group", 0}, {91, 55, "account", "group", 0}, {91, 44, "old-account", "group", 0}, {91, 44, "account", "unknown", 0}} {
		found, err := FindScopedAssetBindingsContext(context.Background(), tc.channel, tc.user, tc.scope, "group", tc.handle)
		require.NoError(t, err)
		require.Len(t, found, tc.want)
	}
	require.NoError(t, InvalidateAssetBindings(group, true))
	found, err := FindScopedAssetBindingsContext(context.Background(), 91, 44, "account", "group", "group")
	require.ErrorIs(t, err, ErrAssetRevoked)
	require.Empty(t, found)
}

func TestAssetBindingLookupRechecksRevocationAfterDiscoveringIDs(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "lookup", CanonicalID: "lookup"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	var revoked atomic.Bool
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:revoke_lookup_candidates", func(tx *gorm.DB) {
		ids, ok := tx.Statement.Dest.(*[]int)
		if tx.Error != nil || !ok || len(*ids) != 1 || !revoked.CompareAndSwap(false, true) {
			return
		}
		// Another request revokes ownership after the ID probe, before the
		// lookup fetches metadata. A candidate must not act as cached access.
		if err := DB.Model(&ConfigurableResourceState{}).Where("id = ?", (*ids)[0]).
			Update("status", ConfigurableResourceStateStatusInvalid).Error; err != nil {
			tx.AddError(err)
		}
	}))
	defer DB.Callback().Query().Remove("test:revoke_lookup_candidates")
	found, err := FindAssetBindings(asset.UserID, asset.Kind, asset.ID)
	require.NoError(t, err)
	require.True(t, revoked.Load())
	require.Empty(t, found)
}

func TestAssetBindingLookupPreservesDistinctOwnersAndScopes(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	first := AssetBinding{ChannelID: 91, UserID: 44, Scope: "first", Kind: "asset", ID: "same-id", CanonicalID: "same-id"}
	second := first
	second.ChannelID, second.Scope = 92, "second"
	foreign := first
	foreign.ChannelID, foreign.Scope, foreign.UserID = 93, "foreign", 55
	for _, binding := range []AssetBinding{first, second, foreign} {
		require.NoError(t, SaveAssetBindings([]AssetBinding{binding}))
	}
	found, err := FindAssetBindings(first.UserID, first.Kind, first.ID)
	require.NoError(t, err)
	require.ElementsMatch(t, []AssetBinding{first, second}, found, "ID lookup must retain routing ambiguity across accounts")
	found, err = FindAssetBindings(foreign.UserID, foreign.Kind, foreign.ID)
	require.NoError(t, err)
	require.Equal(t, []AssetBinding{foreign}, found)
	require.NoError(t, InvalidateAssetBindings(first, false))
	found, err = FindAssetBindings(first.UserID, first.Kind, first.ID)
	require.NoError(t, err)
	require.Equal(t, []AssetBinding{second}, found)
}
