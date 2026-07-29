package model

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedCreditQuotaUser(t *testing.T, id int, quota int, creditQuota int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{
		Id:          id,
		Username:    "credit-user",
		Password:    "password",
		AffCode:     "credit-user-aff",
		Quota:       quota,
		CreditQuota: creditQuota,
	}).Error)
}

func TestManageUserCreditQuotaGrantAddsSpendableBalanceAndDebt(t *testing.T) {
	truncateTables(t)
	seedCreditQuotaUser(t, 9101, 300, 100)

	user, record, err := ManageUserCreditQuota(
		9101,
		1,
		"root",
		CreditQuotaOperationGrant,
		250,
		"temporary credit",
	)

	require.NoError(t, err)
	assert.Equal(t, 550, user.Quota)
	assert.Equal(t, 350, user.CreditQuota)
	assert.Equal(t, 300, record.BalanceBefore)
	assert.Equal(t, 550, record.BalanceAfter)
	assert.Equal(t, 100, record.CreditBefore)
	assert.Equal(t, 350, record.CreditAfter)

	var persisted User
	require.NoError(t, DB.First(&persisted, 9101).Error)
	assert.Equal(t, 550, persisted.Quota)
	assert.Equal(t, 350, persisted.CreditQuota)
}

func TestManageUserCreditQuotaRepaymentOnlyReducesDebt(t *testing.T) {
	truncateTables(t)
	seedCreditQuotaUser(t, 9102, 80, 300)

	user, record, err := ManageUserCreditQuota(
		9102,
		1,
		"root",
		CreditQuotaOperationRepay,
		120,
		"bank transfer received",
	)

	require.NoError(t, err)
	assert.Equal(t, 80, user.Quota)
	assert.Equal(t, 180, user.CreditQuota)
	assert.Equal(t, 80, record.BalanceBefore)
	assert.Equal(t, 80, record.BalanceAfter)
	assert.Equal(t, 300, record.CreditBefore)
	assert.Equal(t, 180, record.CreditAfter)
}

func TestManageUserCreditQuotaRejectsOverRepaymentWithoutMutation(t *testing.T) {
	truncateTables(t)
	seedCreditQuotaUser(t, 9103, 60, 100)

	_, _, err := ManageUserCreditQuota(
		9103,
		1,
		"root",
		CreditQuotaOperationRepay,
		101,
		"",
	)

	require.ErrorIs(t, err, ErrCreditQuotaRepaymentExceedsDue)
	var user User
	require.NoError(t, DB.First(&user, 9103).Error)
	assert.Equal(t, 60, user.Quota)
	assert.Equal(t, 100, user.CreditQuota)
	var count int64
	require.NoError(t, DB.Model(&CreditQuotaRecord{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestManageUserCreditQuotaAllowsGrantAboveLegacyInt32Limit(t *testing.T) {
	if strconv.IntSize < 64 {
		t.Skip("large credit quota requires a 64-bit server")
	}
	truncateTables(t)
	seedCreditQuotaUser(t, 9104, 100, 0)
	largeGrant := int(int64(2_500_000_000))

	user, record, err := ManageUserCreditQuota(
		9104,
		1,
		"root",
		CreditQuotaOperationGrant,
		largeGrant,
		"",
	)

	require.NoError(t, err)
	assert.Equal(t, largeGrant+100, user.Quota)
	assert.Equal(t, largeGrant, user.CreditQuota)
	assert.Equal(t, largeGrant, record.Amount)

	var persisted User
	require.NoError(t, DB.First(&persisted, 9104).Error)
	assert.Equal(t, largeGrant+100, persisted.Quota)
	assert.Equal(t, largeGrant, persisted.CreditQuota)
}

func TestOrdinaryUserUpdateCannotModifyCreditQuota(t *testing.T) {
	truncateTables(t)
	seedCreditQuotaUser(t, 9105, 200, 150)

	update := &User{
		Id:          9105,
		Username:    "credit-user-renamed",
		CreditQuota: 999,
	}
	require.NoError(t, update.Update(false))

	var user User
	require.NoError(t, DB.First(&user, 9105).Error)
	assert.Equal(t, "credit-user-renamed", user.Username)
	assert.Equal(t, 200, user.Quota)
	assert.Equal(t, 150, user.CreditQuota)
}
