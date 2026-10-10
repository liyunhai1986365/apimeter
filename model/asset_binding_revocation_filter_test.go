package model

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func insertAssetRevocationFilterFixture(t *testing.T, binding AssetBinding, status string) ConfigurableResourceState {
	t.Helper()
	metadata, err := common.Marshal(binding)
	require.NoError(t, err)
	row := ConfigurableResourceState{
		ChannelID: binding.ChannelID, ProfileID: assetBindingProfile, ResourceID: binding.Kind,
		PreRequestID: binding.Scope, StateKey: assetBindingKey(binding.ID), StateValue: strconv.Itoa(binding.UserID),
		Status: status, Metadata: string(metadata),
	}
	require.NoError(t, DB.Create(&row).Error)
	return row
}

func requireAssetRevocationFilterStatus(t *testing.T, id int, status string) {
	t.Helper()
	var row ConfigurableResourceState
	require.NoError(t, DB.First(&row, id).Error)
	require.Equal(t, status, row.Status)
}

func TestAssetRevocationFilterResolvesStoredIdentityAndKeepsScope(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "finished-handle", CanonicalID: "original-upload", GroupID: "parent"}
	assetRow := insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	// Historical rows may use an older scope tuple. Resolving the target must
	// retain the same visibility as the existing owner/account discovery.
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("id = ?", assetRow.Id).
		Updates(map[string]any{"user_id": 7, "token_id": 8}).Error)
	task := asset
	task.Kind, task.ID, task.GroupID = "task", "upload-handle", ""
	insertAssetRevocationFilterFixture(t, task, ConfigurableResourceStateStatusActive)
	alias := asset
	alias.ID, alias.GroupID = "unhydrated-handle", ""
	insertAssetRevocationFilterFixture(t, alias, ConfigurableResourceStateStatusActive)
	history := alias
	history.ID = "historical-handle"
	insertAssetRevocationFilterFixture(t, history, ConfigurableResourceStateStatusInvalid)

	otherScope := asset
	otherScope.Scope = "other-account"
	otherUser := asset
	otherUser.ID, otherUser.UserID = "another-owner-handle", 55
	otherChannel := asset
	otherChannel.ChannelID = 92
	protected := []ConfigurableResourceState{
		insertAssetRevocationFilterFixture(t, otherScope, ConfigurableResourceStateStatusActive),
		insertAssetRevocationFilterFixture(t, otherUser, ConfigurableResourceStateStatusActive),
		insertAssetRevocationFilterFixture(t, otherChannel, ConfigurableResourceStateStatusActive),
	}

	stale := asset
	stale.CanonicalID = "newly-reported-id"
	var before, after []ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Order("id").Find(&before).Error)
	require.NoError(t, InvalidateAssetBindings(stale, false))
	for _, binding := range []AssetBinding{asset, task, alias, history} {
		found, err := FindScopedAssetBindingsContext(context.Background(), binding.ChannelID, binding.UserID, binding.Scope, binding.Kind, binding.ID)
		if binding.ID == history.ID {
			require.NoError(t, err) // An already invalid ownership row is excluded before identity checks.
		} else {
			require.ErrorIs(t, err, ErrAssetRevoked)
		}
		require.Empty(t, found, "all aliases must follow the stored canonical tombstone")
	}
	require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Order("id").Find(&after).Error)
	require.Equal(t, before, after, "deletion must not rewrite historical ownership")
	for _, row := range protected {
		requireAssetRevocationFilterStatus(t, row.Id, ConfigurableResourceStateStatusActive)
	}
}

func TestAssetRevocationFilterIncludesEscapedHistoricalAliases(t *testing.T) {
	for _, tc := range []struct {
		name, canonical, original, escaped string
	}{
		{"unicode", "素材-original", "素材", `\u7d20\u6750`},
		{"quote", `quote"original`, `\"`, `\u0022`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "current-handle", CanonicalID: tc.canonical}
			assetRow := insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
			alias := asset
			alias.Kind, alias.ID = "task", "historical-task"
			aliasRow := insertAssetRevocationFilterFixture(t, alias, ConfigurableResourceStateStatusActive)
			metadata := strings.ReplaceAll(aliasRow.Metadata, tc.original, tc.escaped)
			require.NotEqual(t, aliasRow.Metadata, metadata)
			var decoded AssetBinding
			require.NoError(t, common.UnmarshalJsonStr(metadata, &decoded))
			require.Equal(t, alias, decoded, "the fixture changes JSON representation, not ownership")
			require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("id = ?", aliasRow.Id).Update("metadata", metadata).Error)

			require.NoError(t, InvalidateAssetBindings(asset, false))
			requireAssetRevocationFilterStatus(t, assetRow.Id, ConfigurableResourceStateStatusActive)
			requireAssetRevocationFilterStatus(t, aliasRow.Id, ConfigurableResourceStateStatusActive)
			for _, binding := range []AssetBinding{asset, alias} {
				found, err := FindAssetBindings(binding.UserID, binding.Kind, binding.ID)
				require.NoError(t, err)
				require.Empty(t, found)
			}
		})
	}
}

func TestAssetRevocationFilterBoundsReturnedCandidatesAndComparesExactly(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "current-handle", CanonicalID: "wanted!%_identity"}
	assetRow := insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	alias := asset
	alias.Kind, alias.ID = "task", "upload-handle"
	aliasRow := insertAssetRevocationFilterFixture(t, alias, ConfigurableResourceStateStatusActive)
	var protected []ConfigurableResourceState
	for i := 0; i < 20; i++ {
		unrelated := asset
		unrelated.ID = fmt.Sprintf("unrelated-%d", i)
		unrelated.CanonicalID = unrelated.ID
		protected = append(protected, insertAssetRevocationFilterFixture(t, unrelated, ConfigurableResourceStateStatusActive))
	}
	for i, canonical := range []string{"wanted!anything_identity", "wanted!%Xidentity"} {
		nearMatch := asset
		nearMatch.ID, nearMatch.CanonicalID = fmt.Sprintf("near-match-%d", i), canonical
		protected = append(protected, insertAssetRevocationFilterFixture(t, nearMatch, ConfigurableResourceStateStatusActive))
	}
	falsePositive := asset
	falsePositive.ID, falsePositive.CanonicalID, falsePositive.Project = "project-match", "independent-identity", asset.CanonicalID
	protected = append(protected, insertAssetRevocationFilterFixture(t, falsePositive, ConfigurableResourceStateStatusActive))

	var metadataScans int
	const callback = "test:revocation_candidate_rows"
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register(callback, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "metadata LIKE") {
			metadataScans++
		}
	}))
	t.Cleanup(func() { require.NoError(t, DB.Callback().Query().Remove(callback)) })
	require.NoError(t, InvalidateAssetBindings(asset, false))
	require.Zero(t, metadataScans, "deletion must resolve only the exact identity, without metadata scans")
	requireAssetRevocationFilterStatus(t, assetRow.Id, ConfigurableResourceStateStatusActive)
	requireAssetRevocationFilterStatus(t, aliasRow.Id, ConfigurableResourceStateStatusActive)
	for _, binding := range []AssetBinding{asset, alias} {
		found, err := FindAssetBindings(binding.UserID, binding.Kind, binding.ID)
		require.NoError(t, err)
		require.Empty(t, found)
	}
	for _, row := range protected {
		requireAssetRevocationFilterStatus(t, row.Id, ConfigurableResourceStateStatusActive)
	}
}
