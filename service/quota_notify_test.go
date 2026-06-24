package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestQuotaNotifyPlansDefaultTenAndOneDollarThresholds(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	plans := quotaNotifyPlans(6_000_000, 2_000_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 5_000_000, notifyType: "quota_exceed_usd_10", remainingQuota: 4_000_000},
	}, plans)

	plans = quotaNotifyPlans(1_000_000, 600_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 5_000_000, notifyType: "quota_exceed_usd_10", remainingQuota: 400_000},
		{threshold: 500_000, notifyType: "quota_exceed_usd_1", remainingQuota: 400_000},
	}, plans)
}

func TestSubscriptionQuotaNotifyPlansDefaultTenAndOneDollarThresholds(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	plans := quotaNotifyPlans(0, -400_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 5_000_000, notifyType: "quota_exceed_usd_10", remainingQuota: 400_000},
		{threshold: 500_000, notifyType: "quota_exceed_usd_1", remainingQuota: 400_000},
	}, plans)
}

func TestQuotaNotifyPlansUseCustomThresholdWhenConfigured(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	plans := quotaNotifyPlans(2_000_000, 600_000, 1_500_000)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 1_500_000, notifyType: "quota_exceed", remainingQuota: 1_400_000},
	}, plans)
}

func TestQuotaNotifyPlansDoNotTriggerWhenRemainingIsAtOrAboveThreshold(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	require.Empty(t, quotaNotifyPlans(6_000_000, 1_000_000, 0))
	require.Empty(t, quotaNotifyPlans(2_000_000, 500_000, 1_500_000))
}
