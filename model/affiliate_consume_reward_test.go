package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessAffiliateConsumeRewardsCreditsNetDailyUsageOnce(t *testing.T) {
	truncateTables(t)
	originalRatio := common.AffiliateConsumeRewardRatio
	t.Cleanup(func() {
		common.AffiliateConsumeRewardRatio = originalRatio
		require.NoError(t, setting.UpdateAffiliateRoleConfigsByJSONString(setting.DefaultAffiliateRoleConfigsJSON))
	})
	common.AffiliateConsumeRewardRatio = 0.1
	require.NoError(t, setting.UpdateAffiliateRoleConfigsByJSONString(setting.DefaultAffiliateRoleConfigsJSON))

	seedAffiliateRewardUser(t, 8301, "consume-inviter", 1000, 0)
	seedAffiliateRewardUser(t, 8302, "consume-invitee", 0, 8301)
	seedAffiliateRewardUser(t, 8303, "consume-unrelated", 0, 0)

	periodStart := int64(1_800_000_000)
	periodEnd := periodStart + 24*60*60
	for _, log := range []Log{
		{UserId: 8302, Type: LogTypeConsume, Quota: 10_000, CreatedAt: periodStart + 10},
		{UserId: 8302, Type: LogTypeRefund, Quota: 2_000, CreatedAt: periodStart + 20},
		{UserId: 8302, Type: LogTypeConsume, Quota: 50_000, CreatedAt: periodEnd},
		{UserId: 8303, Type: LogTypeConsume, Quota: 90_000, CreatedAt: periodStart + 30},
	} {
		require.NoError(t, LOG_DB.Create(&log).Error)
	}

	processed, err := ProcessAffiliateConsumeRewards(periodStart, periodEnd, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	quota, affQuota, affHistory := getAffiliateRewardUserBalances(t, 8301)
	assert.Equal(t, 1000, quota)
	assert.Equal(t, 800, affQuota)
	assert.Equal(t, 800, affHistory)

	var reward AffiliateConsumeReward
	require.NoError(t, DB.Where("period_start = ? AND invitee_id = ?", periodStart, 8302).First(&reward).Error)
	assert.Equal(t, int64(8_000), reward.ConsumeQuota)
	assert.Equal(t, int64(800), reward.RewardQuota)
	assert.Equal(t, 0.1, reward.RewardRatio)
	processed, err = ProcessAffiliateConsumeRewards(periodStart, periodEnd, 100)
	require.NoError(t, err)
	assert.Zero(t, processed)
	quota, affQuota, affHistory = getAffiliateRewardUserBalances(t, 8301)
	assert.Equal(t, 1000, quota)
	assert.Equal(t, 800, affQuota)
	assert.Equal(t, 800, affHistory)

	var count int64
	require.NoError(t, DB.Model(&AffiliateConsumeReward{}).
		Where("period_start = ? AND invitee_id = ?", periodStart, 8302).
		Count(&count).Error)
	assert.Equal(t, int64(1), count)
}

func TestProcessAffiliateConsumeRewardsUsesInviterRolePolicy(t *testing.T) {
	truncateTables(t)
	originalRatio := common.AffiliateConsumeRewardRatio
	t.Cleanup(func() {
		common.AffiliateConsumeRewardRatio = originalRatio
		require.NoError(t, setting.UpdateAffiliateRoleConfigsByJSONString(setting.DefaultAffiliateRoleConfigsJSON))
	})
	common.AffiliateConsumeRewardRatio = 0.05
	require.NoError(t, setting.UpdateAffiliateRoleConfigsByJSONString(`[
		{"id":"consume-partner","name":"Consume Partner","consume_reward_ratio":20}
	]`))

	seedAffiliateRewardUser(t, 8311, "consume-role-inviter", 0, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 8311).Update("affiliate_role", "consume-partner").Error)
	seedAffiliateRewardUser(t, 8312, "consume-role-invitee", 0, 8311)

	periodStart := int64(1_800_100_000)
	periodEnd := periodStart + 24*60*60
	require.NoError(t, LOG_DB.Create(&Log{
		UserId: 8312, Type: LogTypeConsume, Quota: 10_000, CreatedAt: periodStart + 10,
	}).Error)

	processed, err := ProcessAffiliateConsumeRewards(periodStart, periodEnd, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	quota, affQuota, affHistory := getAffiliateRewardUserBalances(t, 8311)
	assert.Zero(t, quota)
	assert.Equal(t, 2000, affQuota)
	assert.Equal(t, 2000, affHistory)

	var reward AffiliateConsumeReward
	require.NoError(t, DB.Where("period_start = ? AND invitee_id = ?", periodStart, 8312).First(&reward).Error)
	assert.Equal(t, int64(2_000), reward.RewardQuota)
	assert.Equal(t, "consume-partner", reward.AffiliateRole)
	assert.Equal(t, "Consume Partner", reward.AffiliateRoleName)
}
