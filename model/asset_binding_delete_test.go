package model

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func TestAssetBindingQueuedDeletionsDoNotBorrowConnections(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() == "sqlite" {
		t.Skip("requires concurrent row-lock transactions")
	}
	count := cap(assetDeletionSlots) * 3
	assets := make([]AssetBinding, count)
	for i := range assets {
		id := fmt.Sprintf("delete-%d", i)
		assets[i] = AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: id, CanonicalID: id}
	}
	require.NoError(t, SaveAssetBindings(assets))
	pool, err := DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(cap(assetDeletionSlots) + 1)
	waitsBefore := pool.Stats().WaitCount
	arrived, release := make(chan struct{}, cap(assetDeletionSlots)), make(chan struct{})
	var captured atomic.Int64
	var once sync.Once
	var writers sync.WaitGroup
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:hold_delete_transaction", func(tx *gorm.DB) {
		lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if tx.Error != nil || !ok || lock.Strength != "UPDATE" || captured.Add(1) > int64(cap(assetDeletionSlots)) {
			return
		}
		arrived <- struct{}{}
		select {
		case <-release:
		case <-time.After(10 * time.Second):
			tx.AddError(fmt.Errorf("timed out holding deletion transaction"))
		}
	}))
	defer func() {
		once.Do(func() { close(release) })
		writers.Wait()
		DB.Callback().Query().Remove("test:hold_delete_transaction")
	}()
	done := make(chan error, count)
	for _, asset := range assets {
		writers.Add(1)
		go func() {
			defer writers.Done()
			done <- InvalidateAssetBindings(asset, false)
		}()
	}
	for range cap(assetDeletionSlots) {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("deletion transactions did not reach the barrier")
		}
	}
	// Fill every deletion slot with a paused transaction. Additional deletes
	// must wait outside the pool, leaving its last connection usable by reads.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, pool.PingContext(ctx))
	found, err := FindAssetBindingsContext(ctx, 44, "asset", assets[0].ID)
	require.NoError(t, err)
	require.Len(t, found, 1)
	require.Equal(t, waitsBefore, pool.Stats().WaitCount, "queued deletions must not contend for the remaining pool connection")
	once.Do(func() { close(release) })
	for range count {
		require.NoError(t, <-done)
	}
	remaining, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Empty(t, remaining)
}
