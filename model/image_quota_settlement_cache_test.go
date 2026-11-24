package model

import (
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageQuotaRefundBecomesSpendableOnlyAfterCommit(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback"} {
		t.Run(outcome, func(t *testing.T) {
			truncateTables(t)
			resetBatchUpdateTestState(t)
			server := useUserCacheMiniRedis(t)
			common.SyncFrequency = 600
			common.BatchUpdateEnabled = true
			user := createReserveTestUser(t, 100)
			token := createReserveTestToken(t, 100)
			ok, err := TryReserveUserQuota(user.Id, 30)
			require.NoError(t, err)
			require.True(t, ok)
			ok, err = TryReserveTokenQuota(token.Id, token.Key, 30, false)
			require.NoError(t, err)
			require.True(t, ok)
			walletKey, tokenKey := getUserCacheKey(user.Id), getTokenCacheKey(token.Key)
			task := Task{TaskID: common.GetRandomString(16), Status: TaskStatusInProgress, Quota: 30}
			require.NoError(t, task.Insert())
			// Force recovery to resolve each signed marker from SQL.
			outage := &imageQuotaCleanupOutage{}
			outage.down.Store(true)
			common.RDB.AddHook(outage)
			if outcome == "rollback" {
				failNextImageUpdate(t, "tokens")
			}
			const callback = "test:observe_uncommitted_image_refund"
			observed := false
			require.NoError(t, DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != "users" || observed {
					return
				}
				observed = true
				require.Equal(t, "70", server.HGet(walletKey, "Quota"))
				require.Equal(t, "70", server.HGet(tokenKey, "RemainQuota"))
			}))
			t.Cleanup(func() { require.NoError(t, DB.Callback().Update().Remove(callback)) })
			err = SettleImageTaskQuota(ImageTaskQuotaReservation{UserID: user.Id, TokenID: token.Id, TokenKey: token.Key}, -20, func(tx *gorm.DB) error {
				task.Quota, task.Status = 10, TaskStatusSuccess
				won, err := task.UpdateWithStatusTx(tx, TaskStatusInProgress)
				require.True(t, won)
				return err
			})
			want := 90
			if outcome == "rollback" {
				require.ErrorIs(t, err, ErrImageTaskTokenReservation)
				want = 70
			} else {
				require.NoError(t, err)
			}
			require.True(t, observed)
			require.Equal(t, "70", server.HGet(walletKey, "Quota"))
			require.Equal(t, "70", server.HGet(tokenKey, "RemainQuota"))
			outage.down.Store(false)
			server.FastForward(imageQuotaCacheLease + time.Second)
			for range 2 { // Duplicate readers must not credit a refund twice.
				recoverImageQuota(walletKey, "wallet")
				recoverImageQuota(tokenKey, "token")
				require.Equal(t, strconv.Itoa(want), server.HGet(walletKey, "Quota"))
				require.Equal(t, strconv.Itoa(want), server.HGet(tokenKey, "RemainQuota"))
				require.Equal(t, strconv.Itoa(100-want), server.HGet(tokenKey, "UsedQuota"))
			}
			// The regular queued debit and the synchronous image refund coexist.
			batchUpdate()
			require.Equal(t, want, getUserQuotaFromDB(t, user.Id))
			require.Equal(t, want, getTokenFromDB(t, token.Id).RemainQuota)
		})
	}
}
