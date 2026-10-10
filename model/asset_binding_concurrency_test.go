package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/internal/testdb"
	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

type assetBindingWriteObserver struct {
	logger.Interface
	inserts   atomic.Int64
	selects   atomic.Int64
	updates   atomic.Int64
	deadlocks atomic.Int64
	rowLocks  atomic.Int64
}

func (l *assetBindingWriteObserver) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	if strings.HasPrefix(sql, "INSERT ") {
		l.inserts.Add(1)
	}
	if strings.HasPrefix(sql, "SELECT ") {
		l.selects.Add(1)
	}
	if strings.HasPrefix(sql, "UPDATE ") {
		l.updates.Add(1)
	}
	if strings.Contains(sql, "FOR UPDATE") || strings.Contains(sql, "FOR SHARE") || strings.Contains(sql, "LOCK IN SHARE MODE") {
		l.rowLocks.Add(1)
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1213 {
		l.deadlocks.Add(1)
	}
	l.Interface.Trace(ctx, begin, fc, err)
}

func setupAssetBindingConcurrencyTest(t *testing.T) *assetBindingWriteObserver {
	t.Helper()
	original := DB
	t.Cleanup(func() { DB = original })
	DB = testdb.AssetOpener(t, filepath.Join(t.TempDir(), "bindings.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	observer := &assetBindingWriteObserver{Interface: DB.Logger.LogMode(logger.Silent)}
	DB.Logger = observer
	return observer
}

func runConcurrentAssetWrites(t *testing.T, workers, rounds int, write func(int, int) error) {
	t.Helper()
	start := make(chan struct{})
	results := make(chan error, workers*rounds)
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for round := 0; round < rounds; round++ {
				results <- write(worker, round)
			}
		}(worker)
	}
	close(start)
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
}

func TestAssetBindingConcurrentDistinctPolling(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	const workers = 16
	bindings := make([]AssetBinding, workers)
	for i := range bindings {
		bindings[i] = AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset",
			ID: fmt.Sprintf("poll-asset-%d", i), Project: "project", GroupID: fmt.Sprintf("group-%d", i)}
		require.NoError(t, SaveAssetBindings([]AssetBinding{bindings[i]}))
	}
	observer.inserts.Store(0)
	observer.deadlocks.Store(0)
	runConcurrentAssetWrites(t, workers, 50, func(worker, round int) error {
		// GetAsset can return an ID that is also in the existing binding set.
		return SaveAssetBindings([]AssetBinding{bindings[worker], bindings[worker]})
	})
	require.Zero(t, observer.inserts.Load(), "polling existing assets must not attempt duplicate inserts")
	require.Zero(t, observer.deadlocks.Load(), "a retry must not hide the original polling deadlock")
	for _, binding := range bindings {
		found, err := FindAssetBindings(binding.UserID, binding.Kind, binding.ID)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, binding.GroupID, found[0].GroupID)
	}
}

func TestAssetBindingConcurrentDistinctCreationAndAliases(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	runConcurrentAssetWrites(t, 16, 8, func(worker, round int) error {
		asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset",
			ID: fmt.Sprintf("new-%d-%d", worker, round)}
		asset.CanonicalID = asset.ID
		if err := SaveAssetBindings([]AssetBinding{asset, asset}); err != nil {
			return err
		}
		alias := asset
		alias.ID += "-alias"
		return SaveAssetBindings([]AssetBinding{asset, alias})
	})
	require.Zero(t, observer.deadlocks.Load(), "first use and alias discovery must not recreate the gap-lock cycle")
	bindings, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Len(t, bindings, 16*8*2)
}

func TestAssetBindingConcurrentPollingAndRevocation(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "revoked", CanonicalID: "revoked"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	runConcurrentAssetWrites(t, 8, 20, func(worker, round int) error {
		if worker == 0 && round == 10 {
			return InvalidateAssetBindings(asset, false)
		}
		err := SaveAssetBindings([]AssetBinding{asset, asset})
		if errors.Is(err, ErrAssetNotOwned) {
			return nil
		}
		return err
	})
	found, err := FindAssetBindings(44, "asset", asset.ID)
	require.NoError(t, err)
	require.Empty(t, found, "a concurrent save must not revive revoked ownership")
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{asset}), ErrAssetNotOwned)
}

func TestAssetBindingConcurrentFirstUseClaims(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	const rounds = 8
	runConcurrentAssetWrites(t, 16, rounds, func(worker, round int) error {
		binding := AssetBinding{ChannelID: worker + 1, UserID: worker + 1, Scope: "shared-account",
			Kind: "asset", ID: fmt.Sprintf("shared-%d", round)}
		binding.CanonicalID = binding.ID
		if worker%2 == 0 {
			binding.Kind = "task"
		}
		alias := binding
		alias.ID = fmt.Sprintf("alias-%d-%d", round, worker)
		bindings := []AssetBinding{alias, binding}
		if worker%2 == 0 {
			bindings[0], bindings[1] = bindings[1], bindings[0]
		}
		err := ClaimLegacyAssetBindings(bindings)
		if errors.Is(err, ErrAssetNotOwned) {
			return nil
		}
		return err
	})
	require.Zero(t, observer.deadlocks.Load(), "racing mutex initialization must not deadlock with an ownership insert")
	for round := 0; round < rounds; round++ {
		var states []ConfigurableResourceState
		require.NoError(t, assetHandleStates(DB, "asset", fmt.Sprintf("shared-%d", round)).Find(&states).Error)
		require.Len(t, states, 1, "exactly one claimant may own each shared asset/task")
	}
	var rows int64
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("profile_id = ?", assetBindingProfile).Count(&rows).Error)
	require.EqualValues(t, rounds*2, rows, "rejected claims must not leave partial aliases")
}

func TestAssetBindingMutexInitializationRaceMySQL(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires MySQL duplicate-key handling during mutex initialization")
	}
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "original", CanonicalID: "original"}
	alias := asset
	alias.ID = "new-alias"
	paused := make(chan struct{})
	release := make(chan struct{})
	var captured atomic.Bool
	var unblock sync.Once
	var writers sync.WaitGroup
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("test:pause_mutex_init", func(tx *gorm.DB) {
		states, ok := tx.Statement.Dest.(*[]ConfigurableResourceState)
		if !ok || len(*states) == 0 || (*states)[0].ProfileID != assetBindingLockProfile || !captured.CompareAndSwap(false, true) {
			return
		}
		close(paused)
		select {
		case <-release:
		case <-time.After(10 * time.Second):
			tx.AddError(fmt.Errorf("timed out waiting for concurrent mutex initialization"))
		}
	}))
	defer func() {
		unblock.Do(func() { close(release) })
		writers.Wait()
		DB.Callback().Create().Remove("test:pause_mutex_init")
	}()
	done := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		done <- SaveAssetBindings([]AssetBinding{asset, alias})
	}()
	select {
	case <-paused:
	case <-time.After(10 * time.Second):
		t.Fatal("initialization did not reach the insert barrier")
	}
	// Win one of the stale batch's keys. The losing batch must roll back,
	// rediscover that row and still initialize/register its remaining alias.
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset}))
	unblock.Do(func() { close(release) })
	require.NoError(t, <-done)
	require.Zero(t, observer.deadlocks.Load())
	for _, binding := range []AssetBinding{asset, alias} {
		found, err := FindAssetBindings(binding.UserID, binding.Kind, binding.ID)
		require.NoError(t, err)
		require.Equal(t, []AssetBinding{binding}, found)
	}
}

func TestAssetBindingRevocationDuringAliasHydrationMySQL57(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() != "mysql" {
		t.Skip("requires ASSET_TEST_MYSQL_DSN for InnoDB row-lock ordering")
	}
	var version string
	require.NoError(t, DB.Raw("SELECT VERSION()").Scan(&version).Error)
	if !strings.HasPrefix(version, "5.7.") {
		t.Skip("uses MySQL 5.7 information_schema lock diagnostics")
	}
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "original", CanonicalID: "original"}
	alias := asset
	alias.ID = "alias"
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset, alias}))
	// Unchanged polling is read-only; filling missing metadata still needs
	// the write transaction whose lock order must agree with deletion.
	asset.Project, alias.Project = "project", "project"
	var aliasRow ConfigurableResourceState
	require.NoError(t, assetHandleStates(DB, alias.Kind, alias.ID).First(&aliasRow).Error)

	locked := make(chan int64, 1)
	release := make(chan struct{})
	var unblock sync.Once
	var writers sync.WaitGroup
	var captured atomic.Bool
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:pause_alias_lock", func(tx *gorm.DB) {
		row, ok := tx.Statement.Dest.(*ConfigurableResourceState)
		_, locking := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
		if tx.Error != nil || !ok || !locking || row.Id != aliasRow.Id || !captured.CompareAndSwap(false, true) {
			return
		}
		var connectionID int64
		if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT CONNECTION_ID()").Scan(&connectionID).Error; err != nil {
			tx.AddError(err)
		}
		locked <- connectionID
		select {
		case <-release:
		case <-time.After(15 * time.Second):
			tx.AddError(fmt.Errorf("timed out waiting for concurrent revocation"))
		}
	}))
	defer func() {
		unblock.Do(func() { close(release) })
		writers.Wait()
		DB.Callback().Query().Remove("test:pause_alias_lock")
	}()
	polled := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		polled <- SaveAssetBindings([]AssetBinding{alias, asset})
	}()
	var connectionID int64
	select {
	case connectionID = <-locked:
	case <-time.After(15 * time.Second):
		t.Fatal("metadata hydration did not acquire the alias row lock")
	}
	revoked := make(chan error, 1)
	writers.Add(1)
	go func() {
		defer writers.Done()
		revoked <- InvalidateAssetBindings(asset, false)
	}()
	// The old bulk UPDATE first locks the lower-PK original, then waits for
	// the alias. Polling already holds the alias and will next lock the original.
	// A fixed revocation path must instead wait before locking ownership rows.
	var waitErr error
	deadline := time.Now().Add(10 * time.Second)
	var waits int64
	for time.Now().Before(deadline) {
		waitErr = DB.Raw(`SELECT COUNT(*) FROM information_schema.innodb_lock_waits w
			JOIN information_schema.innodb_trx b ON b.trx_id = w.blocking_trx_id
			WHERE b.trx_mysql_thread_id = ?`, connectionID).Scan(&waits).Error
		if waitErr != nil || waits > 0 {
			break
		}
		// MySQL 5.7 caches these diagnostic tables for 100 ms after a read.
		// Polling faster can keep returning the pre-wait snapshot.
		time.Sleep(200 * time.Millisecond)
	}
	unblock.Do(func() { close(release) })
	pollErr, revokeErr := <-polled, <-revoked
	require.NoError(t, waitErr)
	require.Positive(t, waits, "deletion must overlap the paused polling transaction")
	if !errors.Is(pollErr, ErrAssetNotOwned) {
		require.NoError(t, pollErr)
	}
	require.NoError(t, revokeErr)
	require.Zero(t, observer.deadlocks.Load(), "deleting aliases must not acquire row locks in the opposite order to polling")
	for _, id := range []string{asset.ID, alias.ID} {
		found, err := FindAssetBindings(44, "asset", id)
		require.NoError(t, err)
		require.Empty(t, found)
	}
}

func TestAssetBindingDeadlockRetriesEntireTransaction(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "first", CanonicalID: "first"}
	alias := asset
	alias.ID = "second"
	var attempts int
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("test:asset_deadlock", func(tx *gorm.DB) {
		state, ok := tx.Statement.Dest.(*ConfigurableResourceState)
		if !ok || state.ProfileID != assetBindingProfile || state.StateKey != assetBindingKey(alias.ID) {
			return
		}
		attempts++
		if attempts == 1 {
			tx.AddError(&mysql.MySQLError{Number: 1213, Message: "injected deadlock"})
		}
	}))
	defer DB.Callback().Create().Remove("test:asset_deadlock")
	// The first alias has already been inserted when the second insert fails.
	// Retrying only that statement would lose the first alias after rollback.
	require.NoError(t, SaveAssetBindings([]AssetBinding{asset, alias}))
	require.Equal(t, 2, attempts)
	for _, id := range []string{asset.ID, alias.ID} {
		found, err := FindAssetBindings(44, "asset", id)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, asset.CanonicalID, found[0].CanonicalID)
	}
}

func TestAssetBindingRevocationRollbackAndRetry(t *testing.T) {
	for _, retryable := range []bool{false, true} {
		t.Run(fmt.Sprintf("retryable=%t", retryable), func(t *testing.T) {
			setupAssetBindingConcurrencyTest(t)
			asset := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: "original", CanonicalID: "original"}
			bindings := []AssetBinding{asset}
			for i := 0; i < assetBindingBatchSize; i++ {
				alias := asset
				alias.Kind, alias.ID = "task", fmt.Sprintf("task-alias-%d", i)
				bindings = append(bindings, alias)
			}
			require.NoError(t, SaveAssetBindings(bindings))
			var injected error = errors.New("injected storage failure")
			if retryable {
				injected = &mysql.MySQLError{Number: 1213, Message: "injected deadlock"}
			}
			updates := 0
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register("test:revoke_failure", func(tx *gorm.DB) {
				values, ok := tx.Statement.Dest.(map[string]any)
				if !ok || values["status"] != ConfigurableResourceStateStatusInvalid {
					return
				}
				updates++
				if updates == 1 {
					tx.AddError(injected)
				}
			}))
			defer DB.Callback().Update().Remove("test:revoke_failure")
			err := InvalidateAssetBindings(asset, false)
			if retryable {
				require.NoError(t, err)
				require.Equal(t, 2, updates, "retry must redo the identity update after rollback")
			} else {
				require.ErrorIs(t, err, injected)
				require.Equal(t, 1, updates, "unrelated storage failures must not be retried")
			}
			var stored []ConfigurableResourceState
			require.NoError(t, DB.Where("profile_id = ?", assetBindingProfile).Find(&stored).Error)
			require.Len(t, stored, len(bindings))
			for _, state := range stored {
				require.Equal(t, ConfigurableResourceStateStatusActive, state.Status, "identity revocation must not rewrite any aliases")
			}
			for _, binding := range []AssetBinding{bindings[0], bindings[len(bindings)-1]} {
				found, err := FindAssetBindings(binding.UserID, binding.Kind, binding.ID)
				require.NoError(t, err)
				if retryable {
					require.Empty(t, found)
				} else {
					require.Len(t, found, 1, "failed revocation must roll back for the whole family")
				}
			}
			fresh := asset
			fresh.ID = "alias-after-revocation-attempt"
			err = SaveAssetBindings([]AssetBinding{fresh})
			if retryable {
				require.ErrorIs(t, err, ErrAssetNotOwned, "a committed identity tombstone must reject new aliases")
			} else {
				require.NoError(t, err, "a rolled-back deletion must not leave a committed identity tombstone")
			}
		})
	}
}

func TestAssetBindingRetryLimitsAndCancellation(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	deadlock := &mysql.MySQLError{Number: 1213}
	attempts := 0
	err := retryAssetBindingWrite(DB, func() error { attempts++; return deadlock })
	require.ErrorIs(t, err, deadlock)
	require.Equal(t, 4, attempts)
	attempts = 0
	err = retryAssetBindingWrite(DB, func() error { attempts++; return ErrAssetNotOwned })
	require.ErrorIs(t, err, ErrAssetNotOwned)
	require.Equal(t, 1, attempts, "ownership failures must not be retried")
	duplicate := &mysql.MySQLError{Number: 1062}
	attempts = 0
	err = retryAssetBindingWrite(DB, func() error { attempts++; return duplicate })
	require.ErrorIs(t, err, duplicate)
	require.Equal(t, 1, attempts, "only mutex initialization may classify a duplicate key as retryable")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	attempts = 0
	err = retryAssetBindingWrite(DB.WithContext(ctx), func() error {
		attempts++
		cancel()
		return deadlock
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, attempts)
}
