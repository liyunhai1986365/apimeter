package model

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTgxMaasAssetMappingPollingAndCancellation(t *testing.T) {
	observer := setupAssetBindingConcurrencyTest(t)
	save := func(ctx context.Context, provider string) error {
		return SaveTgxMaasAssetHandleContext(ctx, 91, 44, "project", "asset-original", provider)
	}
	require.NoError(t, save(context.Background(), "provider-id"))
	observer.inserts.Store(0)
	observer.updates.Store(0)
	for range 3 {
		require.NoError(t, save(context.Background(), "provider-id"))
	}
	require.Zero(t, observer.inserts.Load())
	require.Zero(t, observer.updates.Load())
	if DB.Dialector.Name() == "sqlite" {
		return
	}
	var mapping ConfigurableResourceState
	require.NoError(t, DB.Where("profile_id = ? AND resource_id = ?", "seedance-tgxmaas", "asset_handle").First(&mapping).Error)
	blocker := DB.Begin()
	require.NoError(t, blocker.Error)
	t.Cleanup(func() { _ = blocker.Rollback().Error })
	require.NoError(t, lockForUpdate(blocker).First(&mapping, mapping.Id).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	require.NoError(t, save(ctx, "provider-id"), "unchanged mapping must bypass an exclusive row lock")
	require.ErrorIs(t, save(ctx, "changed-provider-id"), context.DeadlineExceeded, "cancelled mapping writes must release the waiting connection")
	require.NoError(t, blocker.Rollback().Error)
	resolved, err := ResolveTgxMaasAssetHandle(91, 44, "project", "asset-original")
	require.NoError(t, err)
	require.Equal(t, "provider-id", resolved)
	require.NoError(t, save(context.Background(), "changed-provider-id"))
	resolved, err = ResolveTgxMaasAssetHandle(91, 44, "project", "asset-original")
	require.NoError(t, err)
	require.Equal(t, "changed-provider-id", resolved)
}
