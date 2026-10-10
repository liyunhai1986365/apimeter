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

func TestAssetBindingSameGroupWritersProceedWhileDeletionWaits(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(fmt.Sprintf("claim=%t", legacy), func(t *testing.T) {
			observer := setupAssetBindingConcurrencyTest(t)
			if DB.Dialector.Name() == "sqlite" {
				t.Skip("SQLite serializes all writers; requires MySQL or PostgreSQL row locks")
			}
			group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: "parent", CanonicalID: "parent"}
			require.NoError(t, SaveAssetBindings([]AssetBinding{group}))
			first := group
			first.Kind, first.ID, first.CanonicalID, first.GroupID = "asset", "first", "first", group.ID
			second := first
			second.ID, second.CanonicalID = "second", "second"
			write := SaveAssetBindingsContext
			if legacy {
				write = ClaimLegacyAssetBindingsContext
			}

			paused, release := make(chan struct{}), make(chan struct{})
			var captured atomic.Bool
			var once sync.Once
			var writers sync.WaitGroup
			require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("test:pause_child_registration", func(tx *gorm.DB) {
				state, ok := tx.Statement.Dest.(*ConfigurableResourceState)
				if !ok || state.ProfileID != assetBindingProfile || state.StateKey != assetBindingKey(first.ID) || !captured.CompareAndSwap(false, true) {
					return
				}
				// The first child already holds its parent lock and own mutexes.
				close(paused)
				select {
				case <-release:
				case <-time.After(10 * time.Second):
					tx.AddError(fmt.Errorf("timed out at child registration barrier"))
				}
			}))
			defer func() {
				once.Do(func() { close(release) })
				writers.Wait()
				DB.Callback().Create().Remove("test:pause_child_registration")
				DB.Callback().Query().Remove("test:group_delete_lock")
			}()
			created := make(chan error, 1)
			writers.Add(1)
			go func() {
				defer writers.Done()
				created <- write(context.Background(), []AssetBinding{first})
			}()
			select {
			case <-paused:
			case <-time.After(10 * time.Second):
				t.Fatal("first child did not reach the registration barrier")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			require.NoError(t, write(ctx, []AssetBinding{second}), "another child must commit while the first still holds the parent lock")

			deleting := make(chan struct{})
			var deleteStarted atomic.Bool
			require.NoError(t, DB.Callback().Query().Before("gorm:query").Register("test:group_delete_lock", func(tx *gorm.DB) {
				lock, ok := tx.Statement.Clauses["FOR"].Expression.(clause.Locking)
				if ok && lock.Strength == "UPDATE" && deleteStarted.CompareAndSwap(false, true) {
					close(deleting)
				}
			}))
			deleted := make(chan error, 1)
			writers.Add(1)
			go func() {
				defer writers.Done()
				deleted <- InvalidateAssetBindings(group, true)
			}()
			select {
			case <-deleting:
			case <-time.After(10 * time.Second):
				t.Fatal("group deletion did not attempt its exclusive lock")
			}
			select {
			case err := <-deleted:
				t.Fatalf("group deletion bypassed an uncommitted child: %v", err)
			case <-time.After(150 * time.Millisecond):
			}
			once.Do(func() { close(release) })
			require.NoError(t, <-created)
			require.NoError(t, <-deleted)
			for _, child := range []AssetBinding{first, second} {
				found, err := FindAssetBindings(child.UserID, child.Kind, child.ID)
				require.NoError(t, err)
				require.Empty(t, found, "group deletion must include a child committed after initial discovery")
			}
			late := first
			late.ID, late.CanonicalID = "late", "late"
			require.ErrorIs(t, write(context.Background(), []AssetBinding{late}), ErrAssetNotOwned)
			require.Zero(t, observer.deadlocks.Load())
		})
	}
}

func TestAssetBindingConcurrentChildrenInitializeParentOnce(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	// No group binding is pre-created: historical children can be the first
	// resources to initialize a parent's lifecycle. This must not upgrade
	// concurrent shared locks to exclusive locks and create a deadlock cycle.
	runConcurrentAssetWrites(t, 16, 4, func(worker, round int) error {
		id := fmt.Sprintf("child-%d-%d", worker, round)
		return SaveAssetBindings([]AssetBinding{{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: id, CanonicalID: id, GroupID: "unregistered-parent"}})
	})
	require.Zero(t, observer.deadlocks.Load())
	bindings, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Len(t, bindings, 64)
}

func TestAssetBindingConcurrentStoredGroupLockExpansion(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	if DB.Dialector.Name() == "sqlite" {
		t.Skip("requires concurrent shared row locks")
	}
	groups := make([]AssetBinding, 2)
	aliases := make([]AssetBinding, 2)
	for i := range groups {
		id := fmt.Sprintf("group-%d", i)
		groups[i] = AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "group", ID: id, CanonicalID: id}
		aliases[i] = groups[i]
		aliases[i].ID += "-alias"
		require.NoError(t, SaveAssetBindings([]AssetBinding{groups[i], aliases[i]}))
	}

	// Both writers initially need shared locks on both parents. Resolving
	// their different stored group aliases reveals an exclusive dependency.
	// They must release the shared locks and retry in PK order, not upgrade
	// in place while the other transaction holds the same shared parents.
	arrived, release := make(chan struct{}, 2), make(chan struct{})
	var captured [2]atomic.Bool
	var once sync.Once
	var writers sync.WaitGroup
	require.NoError(t, DB.Callback().Query().After("gorm:query").Register("test:pause_group_resolution", func(tx *gorm.DB) {
		states, ok := tx.Statement.Dest.(*[]ConfigurableResourceState)
		if tx.Error != nil || !ok {
			return
		}
		for _, state := range *states {
			for i, alias := range aliases {
				if state.ProfileID != assetBindingProfile || state.ResourceID != "group" || state.StateKey != assetBindingKey(alias.ID) || !captured[i].CompareAndSwap(false, true) {
					continue
				}
				arrived <- struct{}{}
				select {
				case <-release:
				case <-time.After(10 * time.Second):
					tx.AddError(fmt.Errorf("timed out at group dependency barrier"))
				}
			}
		}
	}))
	defer func() {
		once.Do(func() { close(release) })
		writers.Wait()
		DB.Callback().Query().Remove("test:pause_group_resolution")
	}()
	done := make(chan error, 2)
	for i, alias := range aliases {
		alias.CanonicalID, alias.Project = alias.ID, "hydrated"
		bindings := []AssetBinding{alias}
		for j, group := range groups {
			id := fmt.Sprintf("child-%d-%d", i, j)
			bindings = append(bindings, AssetBinding{ChannelID: 91, UserID: 44, Scope: "account", Kind: "asset", ID: id, CanonicalID: id, GroupID: group.ID})
		}
		writers.Add(1)
		go func() {
			defer writers.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			done <- SaveAssetBindingsContext(ctx, bindings)
		}()
	}
	for range 2 {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			t.Fatal("both transactions must hold shared parents before dependency expansion")
		}
	}
	once.Do(func() { close(release) })
	for range 2 {
		require.NoError(t, <-done)
	}
	require.Zero(t, observer.deadlocks.Load(), "dependency expansion must not rely on deadlock recovery")
	for _, alias := range aliases {
		found, err := FindAssetBindings(alias.UserID, alias.Kind, alias.ID)
		require.NoError(t, err)
		require.Len(t, found, 1)
		require.Equal(t, alias.CanonicalID, found[0].CanonicalID)
		require.Equal(t, "hydrated", found[0].Project)
	}
	children, err := ListAssetBindings(91, 44, "account", "asset")
	require.NoError(t, err)
	require.Len(t, children, 4)
}
