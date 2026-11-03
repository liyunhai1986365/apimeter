package model

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestYHSessionAtomicCompletion(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	session := AssetBinding{ChannelID: 91, UserID: 44, Scope: "yh-account", Kind: "session", ID: "secret-token", SessionName: "short123", SessionCreatedAt: common.GetTimestamp(), ExpiresAt: common.GetTimestamp() + 1800}
	require.NoError(t, ReserveYHSessionName(context.Background(), session))
	require.Error(t, ReserveYHSessionName(context.Background(), session))
	require.NoError(t, SaveAssetBindings([]AssetBinding{session}))
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "yh-account", Kind: "group", ID: "128", CanonicalID: "128", GroupType: "LivenessFace"}
	session.SessionGroupID = group.ID
	runConcurrentAssetWrites(t, 8, 2, func(int, int) error { return SaveAssetBindings([]AssetBinding{session, group}) })
	conflict := session
	conflict.SessionGroupID = "129"
	other := group
	other.ID, other.CanonicalID = "129", "129"
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{conflict, other}), ErrAssetNotOwned)
	bindings, err := FindScopedAssetBindingsContext(context.Background(), 91, 44, "yh-account", "group", "129")
	require.NoError(t, err)
	require.Empty(t, bindings, "failed completion must not register another group")
	require.NoError(t, InvalidateAssetBindingsContext(context.Background(), group, true))
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{session, group}), ErrAssetRevoked)
}

func TestYHExpiredSessionCannotRegisterGroup(t *testing.T) {
	setupAssetBindingConcurrencyTest(t)
	session := AssetBinding{ChannelID: 91, UserID: 44, Scope: "yh-account", Kind: "session", ID: "expired-token", SessionName: "expired1", ExpiresAt: common.GetTimestamp() - 1}
	require.NoError(t, SaveAssetBindings([]AssetBinding{session}))
	group := AssetBinding{ChannelID: 91, UserID: 44, Scope: "yh-account", Kind: "group", ID: "130", CanonicalID: "130"}
	session.SessionGroupID = group.ID
	require.ErrorIs(t, SaveAssetBindings([]AssetBinding{session, group}), ErrAssetNotOwned)
	bindings, err := FindScopedAssetBindingsContext(context.Background(), 91, 44, "yh-account", "group", "130")
	require.NoError(t, err)
	require.Empty(t, bindings)
}
