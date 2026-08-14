package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetWalletUsageAggregateOnlyCountsRecentWalletUsage(t *testing.T) {
	truncateTables(t)
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	rows := []BillingUsageItem{
		{UserId: 1001, LogId: 1, BillingSource: BillingSourceWallet, SettlementAmount: 100, BilledAt: now.Add(-time.Hour).Unix()},
		{UserId: 1001, LogId: 2, BillingSource: BillingSourceWallet, SettlementAmount: 400, BilledAt: now.Add(-25 * time.Hour).Unix()},
		{UserId: 1001, LogId: 3, BillingSource: BillingSourceSubscription, SettlementAmount: 200, BilledAt: now.Add(-time.Hour).Unix()},
		{UserId: 1001, LogId: 4, BillingSource: BillingSourceUserProvider, SettlementAmount: 300, BilledAt: now.Add(-time.Hour).Unix()},
		{UserId: 1001, LogId: 5, BillingSource: BillingSourceWallet, SettlementAmount: 900, BilledAt: now.Add(-8 * 24 * time.Hour).Unix()},
		{UserId: 1002, LogId: 6, BillingSource: BillingSourceWallet, SettlementAmount: 700, BilledAt: now.Add(-time.Hour).Unix()},
	}
	require.NoError(t, LOG_DB.Create(&rows).Error)

	aggregate, err := GetWalletUsageAggregate(1001, now)

	require.NoError(t, err)
	require.Equal(t, int64(100), aggregate.Used24Hours)
	require.Equal(t, int64(500), aggregate.Used7Days)
	require.Equal(t, now.Add(-25*time.Hour).Unix(), aggregate.FirstUsedAt)
	require.Equal(t, now.Add(-time.Hour).Unix(), aggregate.LastUsedAt)
}

func TestUpsertBalanceForecastPreservesNotificationState(t *testing.T) {
	truncateTables(t)
	existing := &BalanceForecast{
		UserId:              2001,
		Balance:             100,
		CalculatedAt:        1,
		Status:              BalanceForecastStatusReady,
		LastNotifiedLevel:   "warning",
		LastNotifiedAt:      2,
		LastNotifiedBalance: 100,
	}
	require.NoError(t, DB.Create(existing).Error)

	updated, err := UpsertBalanceForecast(&BalanceForecast{
		UserId:           2001,
		Balance:          80,
		CalculatedAt:     3,
		Status:           BalanceForecastStatusReady,
		EstimatedHours:   12,
		DailyConsumption: 160,
	})

	require.NoError(t, err)
	require.Equal(t, int64(80), updated.Balance)
	require.Equal(t, "warning", updated.LastNotifiedLevel)
	require.Equal(t, int64(2), updated.LastNotifiedAt)
	require.Equal(t, int64(100), updated.LastNotifiedBalance)
}
