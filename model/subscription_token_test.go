package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedSubscriptionTokenUser(t *testing.T, id int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: "subscription-token-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    0,
	}).Error)
}

func seedSubscriptionTokenPlan(t *testing.T, id int, resetPeriod string) *SubscriptionPlan {
	t.Helper()
	plan := &SubscriptionPlan{
		Id:                 id,
		Title:              "Pro Subscription",
		PriceAmount:        19,
		Currency:           "USD",
		DurationUnit:       SubscriptionDurationMonth,
		DurationValue:      1,
		Enabled:            true,
		TotalAmount:        12000,
		QuotaResetPeriod:   resetPeriod,
		ModelLimitsEnabled: true,
		ModelLimits:        "gpt-4.1,claude-sonnet-4",
	}
	require.NoError(t, DB.Create(plan).Error)
	return plan
}

func getSubscriptionTokenBySubscriptionID(t *testing.T, subID int) Token {
	t.Helper()
	var token Token
	require.NoError(t, DB.Where("user_subscription_id = ?", subID).First(&token).Error)
	return token
}

func TestCompleteSubscriptionOrderCreatesDedicatedSubscriptionToken(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7001)
	plan := seedSubscriptionTokenPlan(t, 8001, SubscriptionResetMonthly)
	insertSubscriptionOrderForPaymentGuardTest(t, "sub-token-order", 7001, plan.Id, PaymentProviderStripe)

	require.NoError(t, CompleteSubscriptionOrder("sub-token-order", `{"provider":"stripe"}`, PaymentProviderStripe, "card"))

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 7001, plan.Id).First(&sub).Error)
	assert.NotZero(t, sub.TokenId)

	token := getSubscriptionTokenBySubscriptionID(t, sub.Id)
	assert.Equal(t, sub.TokenId, token.Id)
	assert.Equal(t, 7001, token.UserId)
	assert.NotEmpty(t, token.Key)
	assert.NotContains(t, token.Key, "sp-")
	assert.Equal(t, common.FormatSubscriptionKey(MaskTokenKey(token.Key)), token.GetMaskedKey())
	assert.Equal(t, "subscription", token.BillingSource)
	assert.Equal(t, plan.Id, token.SubscriptionPlanId)
	assert.Equal(t, sub.Id, token.UserSubscriptionId)
	assert.Equal(t, int(plan.TotalAmount), token.RemainQuota)
	assert.Equal(t, 0, token.UsedQuota)
	assert.Equal(t, sub.EndTime, token.ExpiredTime)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, plan.ModelLimits, token.ModelLimits)
}

func TestAdminBindSubscriptionCreatesDedicatedSubscriptionToken(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7002)
	plan := seedSubscriptionTokenPlan(t, 8002, SubscriptionResetWeekly)

	_, err := AdminBindSubscription(7002, plan.Id, "")
	require.NoError(t, err)

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 7002, plan.Id).First(&sub).Error)
	assert.NotZero(t, sub.TokenId)
	token := getSubscriptionTokenBySubscriptionID(t, sub.Id)
	assert.Equal(t, "subscription", token.BillingSource)
	assert.Equal(t, int(plan.TotalAmount), token.RemainQuota)
}

func TestSubscriptionDedicatedTokenUsesPlanTokenGroup(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7009)
	plan := seedSubscriptionTokenPlan(t, 8009, SubscriptionResetMonthly)
	plan.TokenGroup = "vip"
	require.NoError(t, DB.Save(plan).Error)

	_, err := AdminBindSubscription(7009, plan.Id, "")
	require.NoError(t, err)

	var sub UserSubscription
	require.NoError(t, DB.Where("user_id = ? AND plan_id = ?", 7009, plan.Id).First(&sub).Error)
	token := getSubscriptionTokenBySubscriptionID(t, sub.Id)
	assert.Equal(t, "vip", token.Group)
	assert.True(t, token.CrossGroupRetry)
}

func TestResetDueSubscriptionsRefreshesDedicatedSubscriptionTokenQuota(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7003)
	plan := seedSubscriptionTokenPlan(t, 8003, SubscriptionResetDaily)
	now := time.Now().Unix()
	sub := &UserSubscription{
		UserId:        7003,
		PlanId:        plan.Id,
		AmountTotal:   plan.TotalAmount,
		AmountUsed:    9000,
		StartTime:     now - 48*3600,
		EndTime:       now + 10*24*3600,
		Status:        "active",
		Source:        "admin",
		LastResetTime: now - 48*3600,
		NextResetTime: now - 24*3600,
	}
	require.NoError(t, DB.Create(sub).Error)

	token, err := EnsureSubscriptionTokenForSubscription(nil, sub, plan)
	require.NoError(t, err)
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 9000))

	resetCount, err := ResetDueSubscriptions(10)
	require.NoError(t, err)
	assert.Equal(t, 1, resetCount)

	var refreshedSub UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&refreshedSub).Error)
	refreshedToken := getSubscriptionTokenBySubscriptionID(t, sub.Id)
	assert.Equal(t, int64(0), refreshedSub.AmountUsed)
	assert.Equal(t, int(plan.TotalAmount), refreshedToken.RemainQuota)
	assert.Equal(t, 0, refreshedToken.UsedQuota)
	assert.Equal(t, common.TokenStatusEnabled, refreshedToken.Status)
}

func TestResetDueSubscriptionsRefreshesDedicatedTokenWithPeriodAmount(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7007)
	plan := seedSubscriptionTokenPlan(t, 8007, SubscriptionResetDaily)
	plan.TotalAmount = 12000
	plan.QuotaResetAmount = 5000
	require.NoError(t, DB.Save(plan).Error)
	now := time.Now().Unix()
	sub := &UserSubscription{
		UserId:        7007,
		PlanId:        plan.Id,
		AmountTotal:   plan.TotalAmount,
		AmountUsed:    3000,
		StartTime:     now - 48*3600,
		EndTime:       now + 10*24*3600,
		Status:        "active",
		Source:        "admin",
		LastResetTime: now - 48*3600,
		NextResetTime: now - 24*3600,
	}
	require.NoError(t, DB.Create(sub).Error)
	token, err := EnsureSubscriptionTokenForSubscription(nil, sub, plan)
	require.NoError(t, err)
	require.NoError(t, DecreaseTokenQuota(token.Id, token.Key, 3000))

	resetCount, err := ResetDueSubscriptions(10)
	require.NoError(t, err)
	assert.Equal(t, 1, resetCount)

	var refreshedSub UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&refreshedSub).Error)
	refreshedToken := getSubscriptionTokenBySubscriptionID(t, sub.Id)
	assert.Equal(t, int64(5000), refreshedSub.AmountTotal)
	assert.Equal(t, int64(0), refreshedSub.AmountUsed)
	assert.Equal(t, 5000, refreshedToken.RemainQuota)
	assert.Equal(t, 0, refreshedToken.UsedQuota)
}

func TestBuildSubscriptionSummariesCreatesMissingDedicatedToken(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7008)
	plan := seedSubscriptionTokenPlan(t, 8008, SubscriptionResetMonthly)
	now := time.Now().Unix()
	sub := UserSubscription{
		UserId:      7008,
		PlanId:      plan.Id,
		AmountTotal: plan.TotalAmount,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 3600,
		Status:      "active",
		Source:      "admin",
	}
	require.NoError(t, DB.Create(&sub).Error)
	assert.Zero(t, sub.TokenId)

	summaries, err := GetAllActiveUserSubscriptions(7008)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.NotNil(t, summaries[0].Token)
	assert.NotZero(t, summaries[0].Subscription.TokenId)
	assert.NotEmpty(t, summaries[0].Token.Key)
	assert.Contains(t, summaries[0].Token.Key, "****")

	var refreshed UserSubscription
	require.NoError(t, DB.Where("id = ?", sub.Id).First(&refreshed).Error)
	assert.Equal(t, summaries[0].Token.Id, refreshed.TokenId)
}

func TestPreConsumeUserSubscriptionCanRequireBoundSubscription(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7004)
	plan := seedSubscriptionTokenPlan(t, 8004, SubscriptionResetNever)
	now := time.Now().Unix()
	first := &UserSubscription{
		UserId:      7004,
		PlanId:      plan.Id,
		AmountTotal: 1000,
		AmountUsed:  900,
		StartTime:   now - 3600,
		EndTime:     now + 3600,
		Status:      "active",
	}
	second := &UserSubscription{
		UserId:      7004,
		PlanId:      plan.Id,
		AmountTotal: 10000,
		AmountUsed:  0,
		StartTime:   now - 3600,
		EndTime:     now + 3600,
		Status:      "active",
	}
	require.NoError(t, DB.Create(first).Error)
	require.NoError(t, DB.Create(second).Error)

	_, err := PreConsumeUserSubscriptionForSubscription("bound-sub-insufficient", 7004, first.Id, "gpt-4.1", 500)
	require.Error(t, err)

	res, err := PreConsumeUserSubscriptionForSubscription("bound-sub-ok", 7004, second.Id, "gpt-4.1", 500)
	require.NoError(t, err)
	assert.Equal(t, second.Id, res.UserSubscriptionId)
}

func TestSyncSubscriptionTokensForPlanUpdatesExistingDedicatedKeys(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7005)
	plan := seedSubscriptionTokenPlan(t, 8005, SubscriptionResetMonthly)
	var sub *UserSubscription
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		sub, err = CreateUserSubscriptionFromPlanTx(tx, 7005, plan, "admin")
		return err
	})
	require.NoError(t, err)
	require.NotNil(t, sub)
	require.NoError(t, PostConsumeUserSubscriptionDelta(sub.Id, 3000))
	require.NoError(t, DecreaseTokenQuota(sub.TokenId, getSubscriptionTokenBySubscriptionID(t, sub.Id).Key, 3000))

	plan.Title = "Updated Pro"
	plan.ModelLimitsEnabled = true
	plan.ModelLimits = "gpt-4.1-mini"
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return SyncSubscriptionTokensForPlanTx(tx, plan)
	}))

	token := getSubscriptionTokenBySubscriptionID(t, sub.Id)
	assert.Equal(t, "Updated Pro Key", token.Name)
	assert.True(t, token.ModelLimitsEnabled)
	assert.Equal(t, "gpt-4.1-mini", token.ModelLimits)
	assert.Equal(t, int(plan.TotalAmount)-3000, token.RemainQuota)
	assert.Equal(t, 3000, token.UsedQuota)
}

func TestUserTokenQueriesExcludeSubscriptionDedicatedKeys(t *testing.T) {
	truncateTables(t)

	seedSubscriptionTokenUser(t, 7006)
	require.NoError(t, DB.Create(&Token{
		UserId:      7006,
		Name:        "normal-key",
		Key:         "normal-key",
		Status:      common.TokenStatusEnabled,
		RemainQuota: 1000,
	}).Error)
	require.NoError(t, DB.Create(&Token{
		UserId:             7006,
		Name:               "subscription-key",
		Key:                "subscription-key",
		Status:             common.TokenStatusEnabled,
		RemainQuota:        1000,
		BillingSource:      "subscription",
		UserSubscriptionId: 9006,
	}).Error)

	count, err := CountUserTokens(7006, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	tokens, err := GetAllUserTokens(7006, 0, 10, 0, nil)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	assert.Equal(t, "normal-key", tokens[0].Name)

	tokens, total, err := SearchUserTokens(7006, "%key", "", 0, 10, 0, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, tokens, 1)
	assert.Equal(t, "normal-key", tokens[0].Name)
}
