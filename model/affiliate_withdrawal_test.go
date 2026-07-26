package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffiliateWithdrawalReservesAndRestoresRewardBalance(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	common.QuotaPerUnit = 100

	seedAffiliateRewardUser(t, 8401, "affiliate-withdrawal", 50, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 8401).Updates(map[string]interface{}{
		"aff_quota": 1000, "aff_history": 1000,
	}).Error)

	withdrawal, err := SubmitAffiliateWithdrawal(8401, 400, "bank account")
	require.NoError(t, err)
	assert.Equal(t, AffiliateWithdrawalStatusPending, withdrawal.Status)
	quota, affQuota, affHistory := getAffiliateRewardUserBalances(t, 8401)
	assert.Equal(t, 50, quota)
	assert.Equal(t, 600, affQuota)
	assert.Equal(t, 1000, affHistory)

	user, err := GetUserById(8401, true)
	require.NoError(t, err)
	require.Error(t, user.TransferAffQuotaToQuota(700))

	require.NoError(t, CancelAffiliateWithdrawal(8401, withdrawal.Id))
	_, affQuota, affHistory = getAffiliateRewardUserBalances(t, 8401)
	assert.Equal(t, 1000, affQuota)
	assert.Equal(t, 1000, affHistory)
	require.ErrorIs(t, CancelAffiliateWithdrawal(8401, withdrawal.Id), ErrAffiliateWithdrawalStatus)
}

func TestAffiliateWithdrawalApprovalRejectionAndPayment(t *testing.T) {
	truncateTables(t)
	originalQuotaPerUnit := common.QuotaPerUnit
	t.Cleanup(func() { common.QuotaPerUnit = originalQuotaPerUnit })
	common.QuotaPerUnit = 100

	seedAffiliateRewardUser(t, 8411, "affiliate-withdrawal-review", 0, 0)
	require.NoError(t, DB.Model(&User{}).Where("id = ?", 8411).Updates(map[string]interface{}{
		"aff_quota": 1000, "aff_history": 1000,
	}).Error)

	rejected, err := SubmitAffiliateWithdrawal(8411, 300, "account one")
	require.NoError(t, err)
	require.NoError(t, CompleteAffiliateWithdrawal(rejected.Id, AffiliateWithdrawalStatusApproved, "approved"))
	require.NoError(t, CompleteAffiliateWithdrawal(rejected.Id, AffiliateWithdrawalStatusRejected, "rejected"))
	_, affQuota, _ := getAffiliateRewardUserBalances(t, 8411)
	assert.Equal(t, 1000, affQuota)

	paid, err := SubmitAffiliateWithdrawal(8411, 500, "account two")
	require.NoError(t, err)
	require.NoError(t, CompleteAffiliateWithdrawal(paid.Id, AffiliateWithdrawalStatusApproved, "approved"))
	require.NoError(t, CompleteAffiliateWithdrawal(paid.Id, AffiliateWithdrawalStatusPaid, "paid"))
	_, affQuota, _ = getAffiliateRewardUserBalances(t, 8411)
	assert.Equal(t, 500, affQuota)
	require.ErrorIs(t, CompleteAffiliateWithdrawal(paid.Id, AffiliateWithdrawalStatusRejected, "late"), ErrAffiliateWithdrawalStatus)

	withdrawals, total, err := ListAffiliateWithdrawals(8411, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, withdrawals, 2)
	assert.Equal(t, AffiliateWithdrawalStatusPaid, withdrawals[0].Status)
	assert.Equal(t, AffiliateWithdrawalStatusRejected, withdrawals[1].Status)
}
