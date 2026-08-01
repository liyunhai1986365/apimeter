package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func insertStripeAutoRechargeTestUser(t *testing.T, id int, quota int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:       id,
		Username: "stripe_auto_user_" + common.GetRandomString(6),
		AffCode:  "stripe_auto_aff_" + common.GetRandomString(6),
		Quota:    quota,
		Status:   common.UserStatusEnabled,
	}).Error)
}

func insertStripeAutoRechargeTestConfig(t *testing.T, userId int, thresholdQuota int64) *StripeAutoRecharge {
	t.Helper()
	config := &StripeAutoRecharge{
		UserId:                userId,
		StripeCustomerId:      "cus_test",
		StripePaymentMethodId: "pm_test",
		CardBrand:             "visa",
		CardLast4:             "4242",
		Enabled:               true,
		ThresholdQuota:        thresholdQuota,
		TopUpAmount:           20,
		State:                 StripeAutoRechargeStateIdle,
	}
	require.NoError(t, DB.Create(config).Error)
	return config
}

func TestListDueStripeAutoRechargesUsesAuthoritativeWalletBalance(t *testing.T) {
	truncateTables(t)
	quotaPerUnit := int(common.QuotaPerUnit)
	insertStripeAutoRechargeTestUser(t, 8101, 5*quotaPerUnit)
	insertStripeAutoRechargeTestUser(t, 8102, 20*quotaPerUnit)
	insertStripeAutoRechargeTestConfig(t, 8101, int64(10*quotaPerUnit))
	insertStripeAutoRechargeTestConfig(t, 8102, int64(10*quotaPerUnit))

	due, err := ListDueStripeAutoRecharges(10)
	require.NoError(t, err)
	require.Len(t, due, 1)
	require.Equal(t, 8101, due[0].UserId)
}

func TestClaimStripeAutoRechargeIsAtomic(t *testing.T) {
	truncateTables(t)
	insertStripeAutoRechargeTestUser(t, 8201, 0)
	config := insertStripeAutoRechargeTestConfig(t, 8201, int64(common.QuotaPerUnit))

	claimed, err := ClaimStripeAutoRecharge(config.Id)
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = ClaimStripeAutoRecharge(config.Id)
	require.NoError(t, err)
	require.False(t, claimed)
}

func TestLatePaymentIntentResponseCannotReopenCompletedRecharge(t *testing.T) {
	truncateTables(t)
	insertStripeAutoRechargeTestUser(t, 8301, 0)
	config := insertStripeAutoRechargeTestConfig(t, 8301, int64(common.QuotaPerUnit))
	require.NoError(t, DB.Model(config).Update("state", StripeAutoRechargeStateProcessing).Error)
	require.NoError(t, SetStripeAutoRechargePending(config.Id, "auto_ref_race", "", 2000))
	require.NoError(t, CompleteStripeAutoRecharge("auto_ref_race"))

	// Simulates paymentintent.New returning after its succeeded webhook was
	// already handled. The late response must not restore pending state.
	require.NoError(t, SetStripeAutoRechargePending(config.Id, "auto_ref_race", "pi_late", 2000))
	stored, err := GetStripeAutoRechargeByUserId(8301)
	require.NoError(t, err)
	require.Equal(t, StripeAutoRechargeStateIdle, stored.State)
	require.Empty(t, stored.PendingTradeNo)
	require.Empty(t, stored.PendingPaymentIntentId)
}

func TestUpdateStripeAutoRechargeRejectsPendingCharge(t *testing.T) {
	truncateTables(t)
	insertStripeAutoRechargeTestUser(t, 8401, 0)
	config := insertStripeAutoRechargeTestConfig(t, 8401, int64(common.QuotaPerUnit))
	require.NoError(t, DB.Model(config).Updates(map[string]interface{}{
		"state":            StripeAutoRechargeStatePending,
		"pending_trade_no": "auto_ref_pending",
	}).Error)

	err := UpdateStripeAutoRechargeSettings(8401, false, int64(common.QuotaPerUnit), 20, "test")
	require.ErrorIs(t, err, ErrStripeAutoRechargeBusy)
}

func TestUpsertStripeAutoRechargeDoesNotReplaceCardDuringCharge(t *testing.T) {
	truncateTables(t)
	insertStripeAutoRechargeTestUser(t, 8501, 0)
	config := insertStripeAutoRechargeTestConfig(t, 8501, int64(common.QuotaPerUnit))
	require.NoError(t, DB.Model(config).Updates(map[string]interface{}{
		"state":            StripeAutoRechargeStatePending,
		"pending_trade_no": "auto_ref_card_replace",
	}).Error)

	err := UpsertStripeAutoRechargePaymentMethod(&StripeAutoRecharge{
		UserId:                8501,
		StripeCustomerId:      "cus_test",
		StripePaymentMethodId: "pm_replacement",
		CardBrand:             "mastercard",
		CardLast4:             "4444",
	})
	require.ErrorIs(t, err, ErrStripeAutoRechargeBusy)
	stored, err := GetStripeAutoRechargeByUserId(8501)
	require.NoError(t, err)
	require.Equal(t, "pm_test", stored.StripePaymentMethodId)
	require.Equal(t, "auto_ref_card_replace", stored.PendingTradeNo)
}
