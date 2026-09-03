package service

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestBuildBalanceForecastUsesConservativeRecentRate(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	forecast := buildBalanceForecast(1001, 4_800_000, model.WalletUsageAggregate{
		UserId:      1001,
		Used24Hours: 2_400_000,
		Used7Days:   8_400_000,
		FirstUsedAt: now.Add(-7 * 24 * time.Hour).Unix(),
	}, now)

	require.Equal(t, model.BalanceForecastStatusReady, forecast.Status)
	require.Equal(t, int64(100_000), forecast.HourlyConsumption)
	require.Equal(t, int64(1_200_000), forecast.DailyConsumption)
	require.InDelta(t, 48, forecast.EstimatedHours, 0.001)
	require.Equal(t, now.Add(48*time.Hour).Unix(), forecast.EstimatedExhaustedAt)
}

func TestBuildBalanceForecastAdaptsSampleWindowForNewUsage(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	forecast := buildBalanceForecast(1002, 600_000, model.WalletUsageAggregate{
		UserId:      1002,
		Used24Hours: 100_000,
		Used7Days:   100_000,
		FirstUsedAt: now.Add(-30 * time.Minute).Unix(),
	}, now)

	require.Equal(t, int64(100_000), forecast.HourlyConsumption)
	require.Equal(t, int64(100_000), forecast.DailyConsumption)
	require.InDelta(t, 6, forecast.EstimatedHours, 0.001)
}

func TestBuildBalanceForecastUsesExactElapsedSampleWindow(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	forecast := buildBalanceForecast(1005, 600_000, model.WalletUsageAggregate{
		UserId:      1005,
		Used24Hours: 150_000,
		Used7Days:   150_000,
		FirstUsedAt: now.Add(-90 * time.Minute).Unix(),
	}, now)

	require.Equal(t, int64(100_000), forecast.HourlyConsumption)
	require.InDelta(t, 6, forecast.EstimatedHours, 0.001)
}

func TestBuildBalanceForecastHandlesMissingUsageAndDepletedBalance(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

	missing := buildBalanceForecast(1003, 500_000, model.WalletUsageAggregate{}, now)
	require.Equal(t, model.BalanceForecastStatusInsufficientData, missing.Status)
	require.Zero(t, missing.EstimatedHours)

	depleted := buildBalanceForecast(1004, 0, model.WalletUsageAggregate{
		Used24Hours: 100_000,
		Used7Days:   100_000,
		FirstUsedAt: now.Add(-time.Hour).Unix(),
	}, now)
	require.Equal(t, model.BalanceForecastStatusDepleted, depleted.Status)
	require.Zero(t, depleted.EstimatedHours)
}

func TestBalanceForecastNotificationLevels(t *testing.T) {
	require.Empty(t, balanceForecastNotificationLevel(0))
	require.Equal(t, balanceForecastLevelCritical, balanceForecastNotificationLevel(24))
	require.Equal(t, balanceForecastLevelWarning, balanceForecastNotificationLevel(72))
	require.Equal(t, balanceForecastLevelNotice, balanceForecastNotificationLevel(7*24))
	require.Empty(t, balanceForecastNotificationLevel(7*24+0.01))
}

func TestBalanceForecastEmailUsesAgentSiteTopUpLink(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	_, content := balanceForecastEmail(balanceForecastLevelWarning, &model.BalanceForecast{
		Balance:           300_000,
		HourlyConsumption: 10_000,
		DailyConsumption:  100_000,
		EstimatedHours:    48,
	}, "zh", "https://agent.example.com")

	require.Contains(t, content, "href='https://agent.example.com/console/topup'")
	require.NotContains(t, content, "https://modelsell.com/console/topup")
}
