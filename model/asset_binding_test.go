package model

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestAssetBindingPersistenceAndIsolation(t *testing.T) {
	original := DB
	t.Cleanup(func() { DB = original })
	path := filepath.Join(t.TempDir(), "asset-state.db")
	open := testdb.AssetOpener(t, path)
	DB = open()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	asset := AssetBinding{ChannelID: 20, UserID: 1, Scope: "account-a", Kind: "asset", CanonicalID: "local", GroupID: "group"}
	for _, id := range []string{"local", "official"} {
		require.NoError(t, SaveAssetBinding(asset, id))
	}
	task := asset
	task.Kind = "task"
	require.NoError(t, SaveAssetBinding(task, "processing-task"))
	thief := asset
	thief.UserID = 2
	require.ErrorIs(t, SaveAssetBinding(thief, "local"), ErrAssetNotOwned)
	require.NoError(t, SaveAssetBinding(asset, "local"), "idempotent registration must preserve ownership")
	DB = open() // No in-memory ownership cache is required after restart.
	owned, err := FindAssetBindings(1, "asset", "official")
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.Equal(t, "local", owned[0].CanonicalID)
	foreign, err := FindAssetBindings(2, "asset", "official")
	require.NoError(t, err)
	require.Empty(t, foreign)
	otherScope, err := ListAssetBindings(20, 1, "rotated-account", "asset")
	require.NoError(t, err)
	require.Empty(t, otherScope)
	require.NoError(t, InvalidateAssetBindings(owned[0], false))
	for kind, id := range map[string]string{"asset": "official", "task": "processing-task"} {
		found, err := FindAssetBindings(1, kind, id)
		require.NoError(t, err)
		require.Empty(t, found, "deletion revokes both aliases and async handles")
	}
	require.ErrorIs(t, SaveAssetBinding(asset, "local"), ErrAssetNotOwned, "deleted resources cannot be adopted again")
	asset.CanonicalID = "child"
	require.NoError(t, SaveAssetBinding(asset, "child"))
	group := asset
	group.Kind, group.CanonicalID, group.GroupID = "group", "group", ""
	require.NoError(t, SaveAssetBinding(group, "group"))
	require.NoError(t, InvalidateAssetBindings(group, true))
	children, err := FindAssetBindings(1, "asset", "child")
	require.NoError(t, err)
	require.Empty(t, children)
}

func TestAssetBindingSessionExpiryAndStorage(t *testing.T) {
	original := DB
	t.Cleanup(func() { DB = original })
	DB = testdb.AssetOpener(t, filepath.Join(t.TempDir(), "asset-session.db"))()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	session := AssetBinding{ChannelID: 20, UserID: 1, Scope: "account", Kind: "session", ID: "sensitive-session", CanonicalID: "sensitive-session", ExpiresAt: common.GetTimestamp() - 1}
	require.NoError(t, SaveAssetBinding(session, "sensitive-session"))
	session.ExpiresAt = common.GetTimestamp() + 1800
	require.NoError(t, SaveAssetBinding(session, "sensitive-session"))
	found, err := FindAssetBindings(1, "session", "sensitive-session")
	require.NoError(t, err)
	require.Empty(t, found, "repeated registration must not extend session validity")
	require.NoError(t, SaveAssetBinding(session, "new-session"))
	found, err = FindAssetBindings(1, "session", "new-session")
	require.NoError(t, err)
	require.Len(t, found, 1)
	var rows []ConfigurableResourceState
	require.NoError(t, DB.Find(&rows).Error)
	encoded, err := common.Marshal(rows)
	require.NoError(t, err)
	require.False(t, strings.Contains(string(encoded), "sensitive-session"))
	require.NotContains(t, string(encoded), "new-session")
}

func TestAssetLibraryScopePersistence(t *testing.T) {
	original := DB
	t.Cleanup(func() { DB = original })
	open := testdb.AssetOpener(t, filepath.Join(t.TempDir(), "asset-library-scope.db"))
	DB = open()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	first, err := ResolveAssetLibraryScope(20, "endpoint-a", "old-credential-scope")
	require.NoError(t, err)
	require.Equal(t, "old-credential-scope", first)
	DB = open()
	rotated, err := ResolveAssetLibraryScope(20, "endpoint-a", "new-credential-scope")
	require.NoError(t, err)
	require.Equal(t, first, rotated, "credential rotation and restarts must preserve the library identity")
	otherEndpoint, err := ResolveAssetLibraryScope(20, "endpoint-b", "other-endpoint-scope")
	require.NoError(t, err)
	require.NotEqual(t, first, otherEndpoint)
	otherChannel, err := ResolveAssetLibraryScope(21, "endpoint-a", "other-channel-scope")
	require.NoError(t, err)
	require.NotEqual(t, first, otherChannel)
}

func TestAssetLibraryScopeConcurrentInitializationMySQL(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")) == "" {
		t.Skip("requires ASSET_TEST_MYSQL_DSN for real MySQL transaction contention")
	}
	original := DB
	t.Cleanup(func() { DB = original })
	DB = testdb.AssetOpener(t, "")()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	type result struct {
		scope string
		err   error
	}
	const requests = 16
	results := make(chan result, requests)
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		go func(i int) {
			<-start
			scope, err := ResolveAssetLibraryScope(20, "endpoint", []string{"old", "new"}[i%2])
			results <- result{scope, err}
		}(i)
	}
	close(start)
	winner := ""
	for i := 0; i < requests; i++ {
		got := <-results
		require.NoError(t, got.err)
		if winner == "" {
			winner = got.scope
		}
		require.Equal(t, winner, got.scope)
	}
	var rows int64
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Count(&rows).Error)
	require.EqualValues(t, 1, rows)
}

func TestAssetBindingConcurrentRegistrationMySQL(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")) == "" {
		t.Skip("requires ASSET_TEST_MYSQL_DSN for real MySQL transaction contention")
	}
	original := DB
	t.Cleanup(func() { DB = original })
	DB = testdb.AssetOpener(t, "")()
	require.Equal(t, "mysql", DB.Dialector.Name())
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	type result struct {
		owner int
		err   error
	}
	const requests = 16
	results := make(chan result, requests)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for i := 0; i < requests; i++ {
		workers.Add(1)
		go func(owner int) {
			defer workers.Done()
			<-start
			err := SaveAssetBinding(AssetBinding{ChannelID: 20, UserID: owner, Scope: "account", Kind: "asset"}, "contended-asset")
			results <- result{owner: owner, err: err}
		}(i%2 + 1)
	}
	close(start)
	workers.Wait()
	close(results)
	winner := 0
	for result := range results {
		if result.err == nil {
			if winner == 0 {
				winner = result.owner
			}
			require.Equal(t, winner, result.owner, "only one user can own a concurrently registered handle")
		} else {
			require.ErrorIs(t, result.err, ErrAssetNotOwned)
		}
	}
	require.NotZero(t, winner)
	var rows int64
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Count(&rows).Error)
	require.EqualValues(t, 1, rows, "the existing unique index must prevent duplicate owners")
	owned, err := FindAssetBindings(winner, "asset", "contended-asset")
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.NoError(t, InvalidateAssetBindings(owned[0], false))
	for _, owner := range []int{1, 2} {
		require.ErrorIs(t, SaveAssetBinding(AssetBinding{ChannelID: 20, UserID: owner, Scope: "account", Kind: "asset"}, "contended-asset"), ErrAssetNotOwned)
	}
}
