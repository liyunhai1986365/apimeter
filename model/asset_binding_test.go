package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestAssetLegacyClaimRollsBackConflictingAliases(t *testing.T) {
	original := DB
	t.Cleanup(func() { DB = original })
	DB = testdb.AssetOpener(t, filepath.Join(t.TempDir(), "legacy-asset.db"))()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	owner := AssetBinding{ChannelID: 20, UserID: 1, Scope: "account", Kind: "asset", CanonicalID: "z-canonical"}
	require.NoError(t, SaveAssetBinding(owner, "z-canonical"))
	for _, conflict := range []string{"other-user", "revoked", "old-scope"} {
		claim := owner
		switch conflict {
		case "other-user":
			claim.UserID = 2
		case "revoked":
			require.NoError(t, InvalidateAssetBindings(owner, false))
		case "old-scope":
			claim.Scope = "new-account"
		}
		alias := claim
		alias.ID = "a-" + conflict
		claim.ID = "z-canonical"
		require.ErrorIs(t, ClaimLegacyAssetBindings([]AssetBinding{alias, claim}), ErrAssetNotOwned)
		exists, err := HasAssetBinding("asset", alias.ID)
		require.NoError(t, err)
		require.False(t, exists, "a conflict must roll back the earlier alias insert")
	}
}

func TestAssetLegacyConcurrentClaimsMySQL(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")) == "" {
		t.Skip("requires ASSET_TEST_MYSQL_DSN for real MySQL transaction contention")
	}
	original := DB
	originalSQLite, originalMySQL, originalPostgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
	t.Cleanup(func() {
		DB = original
		common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = originalSQLite, originalMySQL, originalPostgres
	})
	DB = testdb.AssetOpener(t, "")()
	common.SetMainDatabaseType(common.DatabaseTypeMySQL)
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	type result struct {
		owner int
		alias string
		err   error
	}
	const requests = 16
	results := make(chan result, requests)
	start := make(chan struct{})
	for i := 0; i < requests; i++ {
		go func(i int) {
			<-start
			binding := AssetBinding{ChannelID: 20, UserID: i%2 + 1, Scope: "account", Kind: "asset", ID: "z-canonical", CanonicalID: "z-canonical"}
			alias := binding
			alias.ID = fmt.Sprintf("a-alias-%d", i)
			results <- result{binding.UserID, alias.ID, ClaimLegacyAssetBindings([]AssetBinding{alias, binding})}
		}(i)
	}
	close(start)
	completed := make([]result, 0, requests)
	for i := 0; i < requests; i++ {
		completed = append(completed, <-results)
	}
	winner := 0
	for _, got := range completed {
		if got.err == nil {
			if winner == 0 {
				winner = got.owner
			}
			require.Equal(t, winner, got.owner)
		} else {
			require.ErrorIs(t, got.err, ErrAssetNotOwned)
			exists, err := HasAssetBinding("asset", got.alias)
			require.NoError(t, err)
			require.False(t, exists)
		}
	}
	require.NotZero(t, winner)
	bindings, err := FindAssetBindings(winner, "asset", "z-canonical")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
}

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
	require.NoError(t, DB.Model(&ConfigurableResourceState{}).Where("profile_id = ?", assetBindingProfile).Count(&rows).Error)
	require.EqualValues(t, 1, rows, "the existing unique index must prevent duplicate owners")
	owned, err := FindAssetBindings(winner, "asset", "contended-asset")
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.NoError(t, InvalidateAssetBindings(owned[0], false))
	for _, owner := range []int{1, 2} {
		require.ErrorIs(t, SaveAssetBinding(AssetBinding{ChannelID: 20, UserID: owner, Scope: "account", Kind: "asset"}, "contended-asset"), ErrAssetNotOwned)
	}
}

func TestAssetBindingMetadataHydrationIsAtomic(t *testing.T) {
	original := DB
	t.Cleanup(func() { DB = original })
	DB = testdb.AssetOpener(t, filepath.Join(t.TempDir(), "asset-metadata.db"))()
	require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
	binding := AssetBinding{ChannelID: 20, UserID: 1, Scope: "account", Kind: "asset", ID: "alias", CanonicalID: "canonical"}
	require.NoError(t, SaveAssetBindings([]AssetBinding{binding}))
	binding.Project, binding.GroupID = "project", "group"
	require.NoError(t, SaveAssetBindings([]AssetBinding{binding}))
	found, err := FindAssetBindings(1, "asset", "alias")
	require.NoError(t, err)
	require.Equal(t, []AssetBinding{binding}, found)
	for _, field := range []string{"project", "group"} {
		conflict := binding
		switch field {
		case "project":
			conflict.Project = "other"
		case "group":
			conflict.GroupID = "other"
		}
		newAlias := binding
		newAlias.ID = "new-" + field
		require.ErrorIs(t, SaveAssetBindings([]AssetBinding{newAlias, conflict}), ErrAssetNotOwned)
		exists, err := HasAssetBinding("asset", newAlias.ID)
		require.NoError(t, err)
		require.False(t, exists, "no partial alias registration after metadata conflict")
	}
	blank := binding
	blank.Project, blank.GroupID, blank.CanonicalID = "", "", "later-asset-id"
	require.NoError(t, SaveAssetBindings([]AssetBinding{blank}))
	found, err = FindAssetBindings(1, "asset", "alias")
	require.NoError(t, err)
	require.Equal(t, []AssetBinding{binding}, found)
}

func TestAssetLegacyConcurrentClaimsAcrossChannelsMySQL(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")) == "" {
		t.Skip("requires ASSET_TEST_MYSQL_DSN for cross-channel transaction contention")
	}
	for _, mixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("concurrent_normal_registration_%v", mixed), func(t *testing.T) {
			original := DB
			sqlite, mysql, postgres := common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL
			t.Cleanup(func() {
				DB = original
				common.UsingSQLite, common.UsingMySQL, common.UsingPostgreSQL = sqlite, mysql, postgres
			})
			DB = testdb.AssetOpener(t, "")()
			common.SetMainDatabaseType(common.DatabaseTypeMySQL)
			require.NoError(t, DB.AutoMigrate(&ConfigurableResourceState{}))
			type result struct {
				owner int
				alias string
				err   error
			}
			const requests = 16
			results := make(chan result, requests)
			start := make(chan struct{})
			for i := 0; i < requests; i++ {
				go func(i int) {
					<-start
					binding := AssetBinding{ChannelID: 20 + i, UserID: i + 1, Scope: "same-account", Kind: "asset", ID: "shared", CanonicalID: "shared"}
					// Asset and task namespaces must contend too, even when their
					// alias sets arrive in a different order.
					if i%2 == 0 {
						binding.Kind = "task"
					}
					alias := binding
					alias.ID = fmt.Sprintf("alias-%d", i)
					bindings := []AssetBinding{alias, binding}
					if i%2 == 0 {
						bindings[0], bindings[1] = bindings[1], bindings[0]
					}
					var err error
					if mixed && i%2 == 0 {
						err = SaveAssetBindings(bindings)
					} else {
						err = ClaimLegacyAssetBindings(bindings)
					}
					results <- result{i + 1, alias.ID, err}
				}(i)
			}
			close(start)
			successes := 0
			for i := 0; i < requests; i++ {
				got := <-results
				if got.err == nil {
					successes++
				} else {
					require.ErrorIs(t, got.err, ErrAssetNotOwned)
					exists, err := HasAssetBinding("asset", got.alias)
					require.NoError(t, err)
					require.False(t, exists)
				}
			}
			require.Equal(t, 1, successes, "only one channel/user may own the shared asset/task")
		})
	}
}
