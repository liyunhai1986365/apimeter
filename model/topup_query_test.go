package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func insertTopUpForQueryTest(t *testing.T, tradeNo string, userID int, status string, createTime int64) {
	t.Helper()
	topUp := &TopUp{
		UserId:          userID,
		Amount:          2,
		Money:           9.99,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodStripe,
		PaymentProvider: PaymentProviderStripe,
		Status:          status,
		CreateTime:      createTime,
	}
	require.NoError(t, topUp.Insert())
}

func insertUserForTopUpQueryTest(t *testing.T, id int, username string) {
	t.Helper()
	user := &User{
		Id:       id,
		Username: username,
		Status:   common.UserStatusEnabled,
		AffCode:  username + "_aff",
	}
	require.NoError(t, DB.Create(user).Error)
}

func TestGetUserTopUpsFiltersByStatus(t *testing.T) {
	truncateTables(t)

	now := common.GetTimestamp()
	insertUserForTopUpQueryTest(t, 501, "wallet_query_user")
	insertTopUpForQueryTest(t, "query-paid-order", 501, common.TopUpStatusSuccess, now)
	insertTopUpForQueryTest(t, "query-pending-order", 501, common.TopUpStatusPending, now)
	insertTopUpForQueryTest(t, "query-expired-order", 501, common.TopUpStatusExpired, now)

	topups, total, err := GetUserTopUps(501, TopUpQuery{
		Status: common.TopUpStatusSuccess,
	}, &common.PageInfo{
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, topups, 1)
	assert.Equal(t, "query-paid-order", topups[0].TradeNo)
}

func TestGetAllTopUpsFiltersByDateRangeAndIncludesUsername(t *testing.T) {
	truncateTables(t)

	insertUserForTopUpQueryTest(t, 601, "alice_wallet")
	insertUserForTopUpQueryTest(t, 602, "bob_wallet")
	insertTopUpForQueryTest(t, "range-before-order", 601, common.TopUpStatusSuccess, 1700000000)
	insertTopUpForQueryTest(t, "range-inside-order", 602, common.TopUpStatusSuccess, 1700100000)
	insertTopUpForQueryTest(t, "range-after-order", 601, common.TopUpStatusSuccess, 1700200000)

	topups, total, err := GetAllTopUps(TopUpQuery{
		StartTime: 1700050000,
		EndTime:   1700150000,
	}, &common.PageInfo{
		Page:     1,
		PageSize: 10,
	})

	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, topups, 1)
	assert.Equal(t, "range-inside-order", topups[0].TradeNo)
	assert.Equal(t, "bob_wallet", topups[0].Username)
}
