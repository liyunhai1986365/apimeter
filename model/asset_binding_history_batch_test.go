package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAssetHistoryBatchKeepsLegacyFamiliesAndScope(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	var batch []*assetHistoryRead
	var expected []ConfigurableResourceState
	for i := range 12 {
		binding := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: fmt.Sprint("alias-", i), CanonicalID: fmt.Sprint("family-", i), GroupID: "parent"}
		status := ConfigurableResourceStateStatusActive
		if i%2 == 0 {
			status = ConfigurableResourceStateStatusInvalid
		}
		row := insertAssetRevocationFilterFixture(t, binding, status)
		require.NoError(t, DB.Model(&row).Updates(map[string]any{"user_id": i % 3, "token_id": i % 2}).Error)
		expected = append(expected, row)
		other := binding
		other.Scope = "other-account"
		insertAssetRevocationFilterFixture(t, other, ConfigurableResourceStateStatusInvalid)
		batch = append(batch, &assetHistoryRead{db: DB.WithContext(context.Background()), ids: []string{binding.CanonicalID}, done: make(chan assetHistoryResult, 1)})
	}
	executeAssetHistoryReads(assetHistoryScope{DB.Statement.ConnPool, 91, "account", "asset"}, batch)
	for i, r := range batch {
		result := <-r.done
		require.NoError(t, result.err)
		require.Len(t, result.states, 1)
		require.Equal(t, expected[i].Id, result.states[0].Id)
		require.Equal(t, expected[i].Status, result.states[0].Status)
	}
}

func TestAssetScopeWalkCrossesPagesAndHistoricalTuples(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	rows := make([]ConfigurableResourceState, 1100)
	for i := range rows {
		rows[i] = ConfigurableResourceState{ChannelID: 91, ProfileID: assetBindingProfile, ResourceID: "asset", PreRequestID: "account", UserID: i / 550, TokenID: 3, StateKey: assetBindingKey(fmt.Sprint(i)), StateValue: "44", Metadata: "{}"}
	}
	require.NoError(t, DB.CreateInBatches(rows, 50).Error)
	query := DB.WithContext(context.Background()).Model(&ConfigurableResourceState{}).Select("id", "metadata").Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ?", 91, assetBindingProfile, "asset", "account")
	seen := map[int]bool{}
	require.NoError(t, walkScopedAssetStateRows(query, func(page []ConfigurableResourceState) error {
		for _, row := range page {
			require.False(t, seen[row.Id])
			seen[row.Id] = true
		}
		return nil
	}))
	require.Len(t, seen, 1100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pages := 0
	err := walkScopedAssetStateRows(query.WithContext(ctx), func([]ConfigurableResourceState) error { pages++; cancel(); return nil })
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, pages)
}
