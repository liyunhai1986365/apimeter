package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListWithdrawalManagementCombinesUserAndAgentRequests(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&User{
		Id:          9101,
		Username:    "withdrawal-user",
		DisplayName: "Withdrawal User",
		Password:    "test",
	}).Error)
	require.NoError(t, DB.Create(&Agent{
		Id:                 9201,
		OwnerUserId:        9101,
		Name:               "Withdrawal Agent",
		Slug:               "withdrawal-agent",
		SettlementCurrency: "RMB",
	}).Error)
	require.NoError(t, DB.Create(&AffiliateWithdrawal{
		Id:          9301,
		UserId:      9101,
		AmountQuota: 500,
		Status:      AffiliateWithdrawalStatusPending,
		AccountInfo: "Alipay: user@example.com",
		CreatedAt:   100,
	}).Error)
	require.NoError(t, DB.Create(&AgentWithdrawal{
		Id:          9401,
		AgentId:     9201,
		AmountQuota: 700,
		AmountMoney: 7,
		Status:      AgentWithdrawalStatusApproved,
		AccountInfo: "USDT (TRC20): TXyz",
		CreatedAt:   200,
	}).Error)

	items, total, err := ListWithdrawalManagement("", "", 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, items, 2)
	assert.Equal(t, WithdrawalSourceAgent, items[0].Source)
	assert.Equal(t, "Withdrawal Agent", items[0].ApplicantName)
	assert.Equal(t, "RMB", items[0].Currency)
	assert.Equal(t, WithdrawalSourceUser, items[1].Source)
	assert.Equal(t, "Withdrawal User", items[1].ApplicantName)

	pending, total, err := ListWithdrawalManagement(WithdrawalSourceUser, AffiliateWithdrawalStatusPending, 0, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, pending, 1)
	assert.Equal(t, 9301, pending[0].Id)
}

func TestListWithdrawalManagementRejectsUnknownSource(t *testing.T) {
	_, _, err := ListWithdrawalManagement("unknown", "", 0, 20)
	require.ErrorIs(t, err, ErrInvalidWithdrawalSource)
}
