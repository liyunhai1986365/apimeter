package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func reserveImageTaskTestDelta(task *Task, accounts ImageTaskQuotaReservation, delta int) error {
	staged := *task
	staged.Quota += delta
	err := ReserveImageTaskQuota(accounts, delta, func(tx *gorm.DB) error {
		won, err := staged.UpdateWithStatusTx(tx, TaskStatusInProgress)
		if err == nil && !won {
			return errors.New("task CAS lost")
		}
		return err
	})
	if err == nil {
		*task = staged
	}
	return err
}

func TestImageTaskQuotaReservationBatchAndCache(t *testing.T) {
	for _, cached := range []bool{false, true} {
		for _, batch := range []bool{false, true} {
			for _, subscription := range []bool{false, true} {
				t.Run(fmt.Sprintf("cache=%v/batch=%v/subscription=%v", cached, batch, subscription), func(t *testing.T) {
					truncateTables(t)
					resetBatchUpdateTestState(t)
					if cached {
						useUserCacheMiniRedis(t)
					}
					common.BatchUpdateEnabled = batch
					user := createReserveTestUser(t, 100)
					token := createReserveTestToken(t, 100)
					accounts := ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}
					if subscription {
						sub := UserSubscription{UserId: user.Id, AmountTotal: 100, AmountUsed: 20, Status: "active"}
						require.NoError(t, DB.Create(&sub).Error)
						accounts.SubscriptionID = sub.Id
					} else {
						won, err := TryReserveUserQuota(user.Id, 20)
						require.NoError(t, err)
						require.True(t, won)
					}
					won, err := TryReserveTokenQuota(token.Id, token.Key, 20, false)
					require.NoError(t, err)
					require.True(t, won)
					task := Task{TaskID: common.GetRandomString(16), UserId: user.Id, Status: TaskStatusInProgress, Quota: 20}
					require.NoError(t, task.Insert())
					userBefore := getUserQuotaFromDB(t, user.Id)
					tokenBefore := getTokenFromDB(t, token.Id).RemainQuota
					require.NoError(t, reserveImageTaskTestDelta(&task, accounts, 30))
					require.Equal(t, 50, task.Quota)
					require.Equal(t, tokenBefore-30, getTokenFromDB(t, token.Id).RemainQuota, "extra delta must persist before any batch flush")
					if !subscription {
						require.Equal(t, userBefore-30, getUserQuotaFromDB(t, user.Id))
					}
					batchUpdate()
					require.Equal(t, 50, getTokenFromDB(t, token.Id).RemainQuota, "batch flush must not apply the extra reservation twice")
					require.Equal(t, 50, getTokenFromDB(t, token.Id).UsedQuota)
					if subscription {
						var sub UserSubscription
						require.NoError(t, DB.First(&sub, accounts.SubscriptionID).Error)
						require.EqualValues(t, 50, sub.AmountUsed)
						require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
					} else {
						require.Equal(t, 50, getUserQuotaFromDB(t, user.Id))
						if cached {
							userCache, err := GetUserCache(user.Id)
							require.NoError(t, err)
							require.Equal(t, 50, userCache.Quota)
						}
					}
					if cached {
						tokenCache, err := cacheGetTokenByKey(token.Key)
						require.NoError(t, err)
						require.Equal(t, 50, tokenCache.RemainQuota)
						require.Equal(t, 50, tokenCache.UsedQuota)
					}
				})
			}
		}
	}
}

func TestImageTaskQuotaReservationCachedBalanceWinsOverBatchDatabase(t *testing.T) {
	truncateTables(t)
	resetBatchUpdateTestState(t)
	useUserCacheMiniRedis(t)
	common.BatchUpdateEnabled = true
	user := createReserveTestUser(t, 100)
	token := createReserveTestToken(t, 100)
	won, err := TryReserveTokenQuota(token.Id, token.Key, 80, false)
	require.NoError(t, err)
	require.True(t, won)
	require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
	task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 80}
	require.NoError(t, task.Insert())
	version := task.ImageBase64Version
	err = reserveImageTaskTestDelta(&task, ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}, 30)
	require.ErrorIs(t, err, ErrImageTaskTokenReservation)
	require.NoError(t, DB.First(&task, task.ID).Error)
	require.Equal(t, 80, task.Quota)
	require.Equal(t, version, task.ImageBase64Version)
	require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
	batchUpdate()
	require.Equal(t, 20, getTokenFromDB(t, token.Id).RemainQuota)
}

func TestImageTaskQuotaReservationRollsBackAllWritesAndCache(t *testing.T) {
	for _, failure := range []string{"task", "wallet", "subscription", "token"} {
		t.Run(failure, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			useUserCacheMiniRedis(t)
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			require.NoError(t, populateUserCache(user))
			_, err := GetTokenByKey(token.Key, true)
			require.NoError(t, err)
			accounts := ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}
			table := map[string]string{"task": "tasks", "wallet": "users", "subscription": "user_subscriptions", "token": "tokens"}[failure]
			if failure == "subscription" {
				sub := UserSubscription{UserId: user.Id, AmountTotal: 100, AmountUsed: 10}
				require.NoError(t, DB.Create(&sub).Error)
				accounts.SubscriptionID = sub.Id
			}
			task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
			require.NoError(t, task.Insert())
			version := task.ImageBase64Version
			failNextImageUpdate(t, table)
			require.Error(t, reserveImageTaskTestDelta(&task, accounts, 20))
			require.Equal(t, 10, task.Quota)
			require.Equal(t, version, task.ImageBase64Version)
			require.NoError(t, DB.First(&task, task.ID).Error)
			require.Equal(t, 10, task.Quota)
			require.Equal(t, version, task.ImageBase64Version)
			require.Equal(t, 100, getUserQuotaFromDB(t, user.Id))
			require.Equal(t, 100, getTokenFromDB(t, token.Id).RemainQuota)
			tokenCache, err := cacheGetTokenByKey(token.Key)
			require.NoError(t, err)
			require.Equal(t, 100, tokenCache.RemainQuota)
			require.Zero(t, tokenCache.UsedQuota)
			userCache, err := GetUserCache(user.Id)
			require.NoError(t, err)
			require.Equal(t, 100, userCache.Quota)
			if accounts.SubscriptionID > 0 {
				var sub UserSubscription
				require.NoError(t, DB.First(&sub, accounts.SubscriptionID).Error)
				require.EqualValues(t, 10, sub.AmountUsed)
			}
		})
	}
}

func TestImageTaskQuotaReservationAccountSemantics(t *testing.T) {
	for _, mode := range []string{"limited_token", "subscription_limit", "unlimited_token", "playground"} {
		t.Run(mode, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			user := createReserveTestUser(t, 10)
			token := createReserveTestToken(t, 10)
			accounts := ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key, TokenUnlimited: mode == "unlimited_token", SkipToken: mode == "playground"}
			if mode == "subscription_limit" {
				sub := UserSubscription{UserId: user.Id, AmountTotal: 100, AmountUsed: 90}
				require.NoError(t, DB.Create(&sub).Error)
				accounts.SubscriptionID = sub.Id
			}
			task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 10}
			require.NoError(t, task.Insert())
			err := reserveImageTaskTestDelta(&task, accounts, 20)
			if mode == "limited_token" || mode == "subscription_limit" {
				if mode == "limited_token" {
					require.ErrorIs(t, err, ErrImageTaskTokenReservation)
				} else {
					require.ErrorIs(t, err, ErrImageTaskSubscriptionReservation)
				}
				require.Equal(t, 10, getUserQuotaFromDB(t, user.Id))
				require.Equal(t, 10, getTokenFromDB(t, token.Id).RemainQuota)
				require.NoError(t, DB.First(&task, task.ID).Error)
				require.Equal(t, 10, task.Quota)
			} else {
				require.NoError(t, err)
				require.Equal(t, -10, getUserQuotaFromDB(t, user.Id), "additional wallet usage retains the existing debt semantics")
				if mode == "playground" {
					require.Equal(t, 10, getTokenFromDB(t, token.Id).RemainQuota)
				} else {
					require.Equal(t, -10, getTokenFromDB(t, token.Id).RemainQuota)
					require.Equal(t, 20, getTokenFromDB(t, token.Id).UsedQuota)
				}
			}
		})
	}
}
