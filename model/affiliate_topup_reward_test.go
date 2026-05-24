package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedAffiliateRewardUser(t *testing.T, id int, username string, quota int, inviterId int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:        id,
		Username:  username,
		Status:    common.UserStatusEnabled,
		Quota:     quota,
		AffCode:   username,
		InviterId: inviterId,
	}).Error)
}

func getAffiliateRewardUserQuota(t *testing.T, userId int) int {
	t.Helper()
	var user User
	require.NoError(t, DB.Select("quota").Where("id = ?", userId).First(&user).Error)
	return user.Quota
}

func countAffiliateRewardRecords(t *testing.T, tradeNo string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, DB.Model(&AffiliateTopUpReward{}).Where("trade_no = ?", tradeNo).Count(&count).Error)
	return count
}

func TestCreateAffiliateTopUpRewardSchedulesRewardForInvitedUserPayment(t *testing.T) {
	truncateTables(t)
	originalRatio := common.AffiliateTopUpRewardRatio
	t.Cleanup(func() {
		common.AffiliateTopUpRewardRatio = originalRatio
	})
	common.AffiliateTopUpRewardRatio = 0.1

	seedAffiliateRewardUser(t, 8101, "inviter", 1000, 0)
	seedAffiliateRewardUser(t, 8102, "invitee", 0, 8101)
	topUp := &TopUp{
		UserId:          8102,
		Amount:          20,
		Money:           20,
		TradeNo:         "affiliate-reward-order",
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      time.Now().Unix(),
		CompleteTime:    time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	created, err := CreateAffiliateTopUpReward(topUp, 100000)
	require.NoError(t, err)
	assert.True(t, created)
	assert.Equal(t, int64(1), countAffiliateRewardRecords(t, topUp.TradeNo))

	var reward AffiliateTopUpReward
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&reward).Error)
	assert.Equal(t, 8101, reward.InviterId)
	assert.Equal(t, 8102, reward.InviteeId)
	assert.Equal(t, 100000, reward.TopUpQuota)
	assert.Equal(t, 10000, reward.RewardQuota)
	assert.Equal(t, AffiliateTopUpRewardStatusPending, reward.Status)
	assert.Equal(t, topUp.CompleteTime+24*60*60, reward.AvailableAt)
	assert.Equal(t, 1000, getAffiliateRewardUserQuota(t, 8101))
}

func TestCreateAffiliateTopUpRewardIsIdempotentPerTradeNo(t *testing.T) {
	truncateTables(t)
	originalRatio := common.AffiliateTopUpRewardRatio
	t.Cleanup(func() {
		common.AffiliateTopUpRewardRatio = originalRatio
	})
	common.AffiliateTopUpRewardRatio = 0.2

	seedAffiliateRewardUser(t, 8111, "inviter-idempotent", 0, 0)
	seedAffiliateRewardUser(t, 8112, "invitee-idempotent", 0, 8111)
	topUp := &TopUp{
		UserId:          8112,
		Amount:          30,
		Money:           30,
		TradeNo:         "affiliate-reward-idempotent",
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusSuccess,
		CompleteTime:    time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	created, err := CreateAffiliateTopUpReward(topUp, 200000)
	require.NoError(t, err)
	assert.True(t, created)
	created, err = CreateAffiliateTopUpReward(topUp, 200000)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, int64(1), countAffiliateRewardRecords(t, topUp.TradeNo))
}

func TestCreateAffiliateTopUpRewardRespectsPerInviteeRewardLimit(t *testing.T) {
	truncateTables(t)
	originalRatio := common.AffiliateTopUpRewardRatio
	originalLimit := common.AffiliateTopUpRewardLimit
	t.Cleanup(func() {
		common.AffiliateTopUpRewardRatio = originalRatio
		common.AffiliateTopUpRewardLimit = originalLimit
	})
	common.AffiliateTopUpRewardRatio = 0.1
	common.AffiliateTopUpRewardLimit = 2

	seedAffiliateRewardUser(t, 8141, "inviter-limit", 0, 0)
	seedAffiliateRewardUser(t, 8142, "invitee-limit", 0, 8141)

	for i := 1; i <= 3; i++ {
		topUp := &TopUp{
			UserId:          8142,
			Amount:          int64(10 * i),
			Money:           float64(10 * i),
			TradeNo:         fmt.Sprintf("affiliate-reward-limit-%d", i),
			PaymentMethod:   PaymentMethodWaffo,
			PaymentProvider: PaymentProviderWaffo,
			Status:          common.TopUpStatusSuccess,
			CompleteTime:    time.Now().Unix(),
		}
		require.NoError(t, topUp.Insert())

		created, err := CreateAffiliateTopUpReward(topUp, 100000)
		require.NoError(t, err)
		assert.Equal(t, i <= 2, created)
	}

	var count int64
	require.NoError(t, DB.Model(&AffiliateTopUpReward{}).
		Where("inviter_id = ? AND invitee_id = ?", 8141, 8142).
		Count(&count).Error)
	assert.Equal(t, int64(2), count)
	assert.Equal(t, int64(0), countAffiliateRewardRecords(t, "affiliate-reward-limit-3"))
}

func TestProcessDueAffiliateTopUpRewardsCreditsInviterAfterDelay(t *testing.T) {
	truncateTables(t)
	seedAffiliateRewardUser(t, 8121, "inviter-due", 5000, 0)
	seedAffiliateRewardUser(t, 8122, "invitee-due", 0, 8121)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&AffiliateTopUpReward{
		TradeNo:         "affiliate-reward-due",
		InviterId:       8121,
		InviteeId:       8122,
		TopUpId:         1,
		TopUpQuota:      50000,
		RewardQuota:     7500,
		RewardRatio:     0.15,
		PaymentProvider: PaymentProviderCreem,
		Status:          AffiliateTopUpRewardStatusPending,
		AvailableAt:     now - 1,
		CreatedAt:       now - 24*60*60,
	}).Error)

	processed, err := ProcessDueAffiliateTopUpRewards(10)
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Equal(t, 12500, getAffiliateRewardUserQuota(t, 8121))

	var reward AffiliateTopUpReward
	require.NoError(t, DB.Where("trade_no = ?", "affiliate-reward-due").First(&reward).Error)
	assert.Equal(t, AffiliateTopUpRewardStatusCompleted, reward.Status)
	assert.NotZero(t, reward.RewardedAt)

	processed, err = ProcessDueAffiliateTopUpRewards(10)
	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	assert.Equal(t, 12500, getAffiliateRewardUserQuota(t, 8121))
}

func TestRechargeWaffoCreatesDelayedAffiliateTopUpReward(t *testing.T) {
	truncateTables(t)
	originalRatio := common.AffiliateTopUpRewardRatio
	t.Cleanup(func() {
		common.AffiliateTopUpRewardRatio = originalRatio
	})
	common.AffiliateTopUpRewardRatio = 0.1

	seedAffiliateRewardUser(t, 8131, "inviter-waffo", 0, 0)
	seedAffiliateRewardUser(t, 8132, "invitee-waffo", 0, 8131)
	topUp := &TopUp{
		UserId:          8132,
		Amount:          2,
		Money:           2,
		TradeNo:         "affiliate-reward-waffo",
		PaymentMethod:   PaymentMethodWaffo,
		PaymentProvider: PaymentProviderWaffo,
		Status:          common.TopUpStatusPending,
		CreateTime:      time.Now().Unix(),
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, RechargeWaffo(topUp.TradeNo, "127.0.0.1"))

	var reward AffiliateTopUpReward
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&reward).Error)
	assert.Equal(t, 8131, reward.InviterId)
	assert.Equal(t, 8132, reward.InviteeId)
	assert.Equal(t, int(2*common.QuotaPerUnit), reward.TopUpQuota)
	assert.Equal(t, int(2*common.QuotaPerUnit*0.1), reward.RewardQuota)
	assert.Equal(t, AffiliateTopUpRewardStatusPending, reward.Status)
	assert.Equal(t, 0, getAffiliateRewardUserQuota(t, 8131))
}
