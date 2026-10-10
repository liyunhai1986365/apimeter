package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAssetTransactionRestoresMySQLConnection(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires MySQL session configuration")
	}
	pool, err := DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	var before, after int
	require.NoError(t, DB.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&before).Error)
	for _, fail := range []bool{false, true} {
		failure := errors.New("rollback asset transaction")
		err := assetWriteTransaction(DB, func(tx *gorm.DB) error {
			var timeout int
			require.NoError(t, tx.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&timeout).Error)
			require.Equal(t, min(before, assetMySQLLockWaitSeconds), timeout)
			if fail {
				return failure
			}
			return nil
		})
		if fail {
			require.ErrorIs(t, err, failure)
		} else {
			require.NoError(t, err)
		}
		require.NoError(t, DB.Raw("SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&after).Error)
		require.Equal(t, before, after, "subsequent non-asset work must see the original session setting")
	}
}

func TestAssetDeletionSlotWaitHasDeadline(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	for range cap(assetDeletionSlots) {
		assetDeletionSlots <- struct{}{}
	}
	defer func() {
		for range cap(assetDeletionSlots) {
			<-assetDeletionSlots
		}
	}()
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "queued"}
	pool, err := DB.DB()
	require.NoError(t, err)
	before := pool.Stats()
	queries := observer.selects.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = revokeAssetIdentity(ctx, asset)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, queries, observer.selects.Load(), "waiting for a slot must not issue SQL")
	require.Equal(t, before.InUse, pool.Stats().InUse)
	require.Equal(t, before.WaitCount, pool.Stats().WaitCount)
}

func TestAssetListPoolWaitHasDeadline(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	pool, err := DB.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	conn, err := pool.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	before := pool.Stats()
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	bindings, err := ListAssetBindingsContext(ctx, 91, 44, "account", "asset")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, bindings, "a canceled list must not return partial ownership")
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, before.InUse, pool.Stats().InUse)
	require.Equal(t, before.WaitCount+1, pool.Stats().WaitCount)
}

func TestAssetIdentityLockTimeoutReleasesPoolMySQL(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires actual MySQL row locks")
	}
	DB = DB.Session(&gorm.Session{PrepareStmt: true}) // Match production's prepared-statement pool.
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "held", CanonicalID: "held"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	var identity ConfigurableResourceState
	require.NoError(t, assetIdentityStates(DB, assetOwnLifecycleKeys(asset)).First(&identity).Error)
	tx := DB.Begin()
	require.NoError(t, tx.Error)
	defer tx.Rollback()
	require.NoError(t, lockForUpdate(tx).First(&identity, identity.Id).Error)
	pool, err := DB.DB()
	require.NoError(t, err)
	baseline := pool.Stats().InUse

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	start := time.Now()
	err = InvalidateAssetBindingsContext(ctx, asset, false)
	cancel()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(start), time.Second)
	require.Eventually(t, func() bool { return pool.Stats().InUse == baseline }, time.Second, 10*time.Millisecond)

	alias := asset
	alias.ID = "pending-alias"
	// A caller without a deadline must stop at the shorter server-side wait.
	// Closing a socket alone does not cancel MySQL 5.7's running lock wait.
	start = time.Now()
	err = SaveAssetBindings([]AssetBinding{alias})
	// MySQL 5.7 checks elapsed lock time periodically, so the Go deadline can
	// win the race with its 1205 response. Both must leave no server waiter.
	if !errors.Is(err, context.DeadlineExceeded) {
		var lockTimeout *mysql.MySQLError
		require.ErrorAs(t, err, &lockTimeout)
		require.EqualValues(t, 1205, lockTimeout.Number)
	}
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, time.Second)
	require.Less(t, elapsed, assetTransactionTimeout+2*time.Second)
	require.Eventually(t, func() bool { return pool.Stats().InUse == baseline }, time.Second, 10*time.Millisecond)
	t.Logf("held identity: registration returned in %s; pool InUse=%d (held test transaction only)", elapsed, pool.Stats().InUse)
	// information_schema transaction snapshots may lag briefly. The original
	// blocking transaction stays open, so waiters cannot disappear by unlock.
	require.Eventually(t, func() bool {
		var serverWaiters int64
		result := DB.Raw("SELECT COUNT(*) FROM information_schema.innodb_trx AS t JOIN information_schema.processlist AS p ON p.ID = t.trx_mysql_thread_id WHERE p.DB = DATABASE() AND t.trx_state = 'LOCK WAIT'").Scan(&serverWaiters)
		return result.Error == nil && serverWaiters == 0
	}, 2*time.Second, 50*time.Millisecond, "server lock waiters must expire even while the blocking transaction remains open")
	var count int64
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("profile_id = ? AND state_key = ?", assetBindingProfile, assetBindingKey(alias.ID)).Count(&count).Error)
	require.Zero(t, count, "a timed-out transaction must not leave a registered alias")
	found, err := FindAssetBindings(44, "asset", asset.ID)
	require.NoError(t, err)
	require.Len(t, found, 1, "a timed-out deletion must not revoke the original")
	require.NoError(t, tx.Rollback().Error)
	require.NoError(t, SaveAssetBindings([]AssetBinding{alias}))
	require.NoError(t, InvalidateAssetBindings(asset, false))
	found, err = FindAssetBindings(44, "asset", alias.ID)
	require.NoError(t, err)
	require.Empty(t, found, "a committed deletion must also revoke the alias")
}
