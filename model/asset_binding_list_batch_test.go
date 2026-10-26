package model

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetListBatchOneWalkKeepsOwnersAndHistoricalTuples(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	var rows []ConfigurableResourceState
	for i := range 1100 {
		binding := AssetBinding{ChannelID: 91, UserID: 1 + i%2, Scope: "account", Kind: "asset", ID: fmt.Sprint("asset-", i)}
		metadata, err := common.Marshal(binding)
		require.NoError(t, err)
		rows = append(rows, ConfigurableResourceState{ChannelID: 91, ProfileID: assetBindingProfile, ResourceID: "asset", PreRequestID: "account", UserID: i / 550, TokenID: 3,
			StateKey: assetBindingKey(binding.ID), StateValue: fmt.Sprint(binding.UserID), Status: ConfigurableResourceStateStatusActive, Metadata: string(metadata)})
	}
	require.NoError(t, DB.CreateInBatches(rows, 50).Error)
	other := AssetBinding{ChannelID: 91, UserID: 1, Scope: "other-account", Kind: "asset", ID: "outside"}
	insertAssetRevocationFilterFixture(t, other, ConfigurableResourceStateStatusActive)
	other.Scope, other.ID = "account", "inactive"
	insertAssetRevocationFilterFixture(t, other, ConfigurableResourceStateStatusInvalid)
	other.ID, other.ExpiresAt = "expired", common.GetTimestamp()-1
	insertAssetRevocationFilterFixture(t, other, ConfigurableResourceStateStatusActive)
	var batch []*assetListRead
	for i := range 40 {
		batch = append(batch, &assetListRead{db: DB.WithContext(context.Background()), user: 1 + i%2, done: make(chan assetListReadResult, 1)})
	}
	observer.selects.Store(0)
	executeAssetListReads(assetHistoryScope{DB.Statement.ConnPool, 91, "account", "asset"}, batch)
	// Scope discovery + 1 zero-tuple page + 2 pages for each historical tuple.
	require.EqualValues(t, 6, observer.selects.Load(), "forty readers must share one account walk")
	for _, request := range batch {
		result := <-request.done
		require.NoError(t, result.err)
		require.Len(t, result.bindings, 550)
		for _, binding := range result.bindings {
			require.Equal(t, request.user, binding.UserID)
			require.Equal(t, "account", binding.Scope)
		}
	}
}

func TestAssetListBatchIsolatesCorruptionAndChecksLiveRevocation(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 1, Scope: "account", Kind: "asset", ID: "valid"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	bad := asset
	bad.UserID, bad.ID = 2, "corrupt"
	row := insertAssetRevocationFilterFixture(t, bad, ConfigurableResourceStateStatusActive)
	require.NoError(t, DB.Model(&row).Update("metadata", "invalid-json").Error)
	var batch []*assetListRead
	for user := 1; user <= 3; user++ {
		batch = append(batch, &assetListRead{db: DB.WithContext(context.Background()), user: user, done: make(chan assetListReadResult, 1)})
	}
	executeAssetListReads(assetHistoryScope{DB.Statement.ConnPool, 91, "account", "asset"}, batch)
	good, corrupt, empty := <-batch[0].done, <-batch[1].done, <-batch[2].done
	require.NoError(t, good.err)
	require.Len(t, good.bindings, 1)
	require.Error(t, corrupt.err)
	require.NoError(t, empty.err)
	require.Empty(t, empty.bindings)
	require.NoError(t, InvalidateAssetBindings(asset, false))
	bindings, err := ListAssetBindings(91, 1, "account", "asset")
	require.NoError(t, err)
	require.Empty(t, bindings, "completed scans must not cache authorization")
	bad.UserID = 1
	metadata, err := common.Marshal(bad)
	require.NoError(t, err)
	require.NoError(t, DB.Model(&row).Update("metadata", string(metadata)).Error)
	_, err = ListAssetBindings(91, 2, "account", "asset")
	require.ErrorContains(t, err, "inconsistent stored asset ownership")
}

func TestAssetListBatchLateReaderGetsFreshScanAndCancellation(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 1, Scope: "account", Kind: "asset", ID: "first"}
	insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	started, release := make(chan struct{}), make(chan struct{})
	var once, releaseOnce sync.Once
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("hold-list-page", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "state_key >") {
			once.Do(func() { close(started); <-release })
		}
	}))
	defer DB.Callback().Query().Remove("hold-list-page")
	defer releaseOnce.Do(func() { close(release) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := make(chan error, 1)
	go func() { _, err := readAssetListBindings(DB.WithContext(ctx), 91, 1, "account", "asset"); first <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("account scan did not start")
	}
	asset.ID = "added-after-snapshot"
	insertAssetRevocationFilterFixture(t, asset, ConfigurableResourceStateStatusActive)
	second := make(chan assetListReadResult, 1)
	go func() {
		bindings, err := readAssetListBindings(DB.WithContext(context.Background()), 91, 1, "account", "asset")
		second <- assetListReadResult{bindings, err}
	}()
	key := assetHistoryScope{DB.Statement.ConnPool, 91, "account", "asset"}
	require.Eventually(t, func() bool {
		assetListReads.Lock()
		defer assetListReads.Unlock()
		return len(assetListReads.queues[key]) == 1
	}, 3*time.Second, time.Millisecond)
	cancel()
	require.ErrorIs(t, <-first, context.Canceled)
	releaseOnce.Do(func() { close(release) })
	select {
	case result := <-second:
		require.NoError(t, result.err, "canceling the first batch must not cancel queued readers")
		require.Len(t, result.bindings, 2, "a late reader must see the write preceding its request")
	case <-time.After(3 * time.Second):
		t.Fatal("queued scan did not finish")
	}
}
