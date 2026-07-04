package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestQuotaNotifyPlansDefaultTenFiveAndInsufficientThresholds(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	plans := quotaNotifyPlans(6_000_000, 2_000_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 5_000_000, notifyType: "quota_exceed_usd_10", remainingQuota: 4_000_000},
	}, plans)

	plans = quotaNotifyPlans(4_000_000, 500_000, 0)

	require.Empty(t, plans)

	plans = quotaNotifyPlans(3_000_000, 600_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 2_500_000, notifyType: "quota_exceed_usd_5", remainingQuota: 2_400_000},
	}, plans)

	plans = quotaNotifyPlans(100_000, 100_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 0, notifyType: "quota_exceed_insufficient", remainingQuota: 0},
	}, plans)

	plans = quotaNotifyPlans(6_000_000, 6_000_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 0, notifyType: "quota_exceed_insufficient", remainingQuota: 0},
	}, plans)
}

func TestSubscriptionQuotaNotifyPlansDefaultTenFiveAndInsufficientThresholds(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	plans := quotaNotifyPlans(3_000_000, 600_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 2_500_000, notifyType: "quota_exceed_usd_5", remainingQuota: 2_400_000},
	}, plans)

	plans = quotaNotifyPlans(100_000, 100_000, 0)

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 0, notifyType: "quota_exceed_insufficient", remainingQuota: 0},
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

	require.Empty(t, quotaNotifyPlans(1_400_000, 100_000, 1_500_000))

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 0, notifyType: "quota_exceed_insufficient", remainingQuota: 0},
	}, quotaNotifyPlans(2_000_000, 2_000_000, 1_500_000))
}

func TestQuotaNotifyPlansDoNotTriggerWhenRemainingIsAtOrAboveThreshold(t *testing.T) {
	originalQuotaPerUnit := common.QuotaPerUnit
	common.QuotaPerUnit = 500_000
	t.Cleanup(func() {
		common.QuotaPerUnit = originalQuotaPerUnit
	})

	require.Empty(t, quotaNotifyPlans(6_000_000, 1_000_000, 0))
	require.Empty(t, quotaNotifyPlans(3_000_000, 500_000, 0))
	require.Empty(t, quotaNotifyPlans(100_000, 99_999, 0))
	require.Empty(t, quotaNotifyPlans(2_000_000, 500_000, 1_500_000))
}

func TestFilterQuotaNotifyPlansSkipsAlreadySentThresholds(t *testing.T) {
	plans := []quotaNotifyPlan{
		{threshold: 5_000_000, notifyType: "quota_exceed_usd_10", remainingQuota: 2_400_000},
		{threshold: 2_500_000, notifyType: "quota_exceed_usd_5", remainingQuota: 2_400_000},
		{threshold: 0, notifyType: "quota_exceed_insufficient", remainingQuota: 0},
	}

	state := map[string]int{
		"quota_exceed_usd_10":       5_000_000,
		"quota_exceed_insufficient": 0,
	}

	require.Equal(t, []quotaNotifyPlan{
		{threshold: 2_500_000, notifyType: "quota_exceed_usd_5", remainingQuota: 2_400_000},
	}, filterQuotaNotifyPlansByState(plans, state))
}

func TestNormalizeQuotaNotifyStateClearsRecoveredThresholds(t *testing.T) {
	state := map[string]int{
		"quota_exceed_usd_10":       5_000_000,
		"quota_exceed_usd_5":        2_500_000,
		"quota_exceed_insufficient": 0,
	}

	normalizeQuotaNotifyState(state, 3_000_000)

	require.Equal(t, map[string]int{
		"quota_exceed_usd_10": 5_000_000,
	}, state)

	normalizeQuotaNotifyState(state, 5_000_000)

	require.Empty(t, state)
}

func TestFilterQuotaNotifyPlansDoesNotReuseCustomThresholdStateAfterThresholdChanges(t *testing.T) {
	plans := []quotaNotifyPlan{
		{threshold: 1_000_000, notifyType: "quota_exceed", remainingQuota: 900_000},
	}
	state := map[string]int{
		"quota_exceed": 1_500_000,
	}

	require.Equal(t, plans, filterQuotaNotifyPlansByState(plans, state))
}

func TestReserveUserQuotaNotifyStateOnlyAllowsThresholdOnce(t *testing.T) {
	truncate(t)

	user := model.User{
		Username: "quota-notify-reserve",
		Password: "password",
		Email:    "quota-notify-reserve@example.com",
		Status:   common.UserStatusEnabled,
		Quota:    2_400_000,
	}
	user.SetSetting(dto.UserSetting{NotifyType: dto.NotifyTypeEmail})
	require.NoError(t, model.DB.Create(&user).Error)

	plan := quotaNotifyPlan{
		threshold:      2_500_000,
		notifyType:     "quota_exceed_usd_5",
		remainingQuota: 2_400_000,
	}

	reserved, err := reserveUserQuotaNotifyState(user.Id, plan, plan.remainingQuota)
	require.NoError(t, err)
	require.True(t, reserved)

	reserved, err = reserveUserQuotaNotifyState(user.Id, plan, plan.remainingQuota)
	require.NoError(t, err)
	require.False(t, reserved)

	updated, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.Equal(t, map[string]int{"quota_exceed_usd_5": 2_500_000}, updated.GetSetting().QuotaNotifyState)
}
