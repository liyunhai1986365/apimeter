package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDeletedWalletImageSettlement(t *testing.T) {
	require.NoError(t, model.DB.AutoMigrate(&model.UserSession{}))
	for _, tc := range []struct {
		name      string
		delivered int
		hard      bool
		retry     bool
	}{
		{"partial_local_retry", 1, false, true},
		{"equal_upstream", 3, false, false},
		{"overage_upstream", 4, false, false},
		{"wallet_removed", 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			const user, token, channel, reserved = 950, 951, 952, 30000
			seedUser(t, user, 70000)
			seedToken(t, token, user, "deleted-wallet-image", 70000)
			seedChannel(t, channel)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", token).Update("used_quota", reserved).Error)
			task := makeTask(user, channel, reserved, token, BillingSourceWallet, 0)
			task.PrivateData.AsyncImage = true
			task.PrivateData.BillingContext.PerCallBilling = true
			task.PrivateData.BillingContext.OtherRatios = map[string]float64{"n": 3}
			if !tc.retry {
				task.PrivateData.UpstreamTaskID = "provider-task"
			}
			require.NoError(t, model.DB.Create(task).Error)
			require.NoError(t, model.DeleteUserById(user))
			if tc.hard {
				// Even if the wallet has disappeared, any surviving token is still owed its reservation.
				require.NoError(t, model.DB.Unscoped().Delete(&model.User{}, user).Error)
			}
			const image = "cHJpdmF0ZQ=="
			result := dto.ImageTaskResponse{State: "succeeded"}
			for range tc.delivered {
				result.Data.Images = append(result.Data.Images, dto.ImageTaskImage{B64Json: image})
			}
			var err error
			task.Status, task.PrivateData.ResultURL = model.TaskStatusSuccess, "private-url"
			task.Data, err = common.Marshal(result)
			require.NoError(t, err)
			if tc.retry {
				const hook = "test:wallet_image_token_refund_error"
				writeErr := errors.New("token temporarily unavailable")
				down := true
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
					if down && tx.Statement.Table == "tokens" {
						tx.AddError(writeErr)
					}
				}))
				t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
				won, err := CompleteImageTask(t.Context(), task, model.TaskStatusInProgress, result, nil)
				require.False(t, won)
				require.ErrorIs(t, err, writeErr)
				var pending model.Task
				require.NoError(t, model.DB.First(&pending, task.ID).Error)
				require.NotNil(t, pending.PrivateData.PendingImageSettlement)
				require.Empty(t, pending.Data)
				require.Equal(t, reserved, pending.Quota)
				var archived model.User
				require.NoError(t, model.DB.Unscoped().First(&archived, user).Error)
				require.Equal(t, 70000, archived.Quota)
				require.Equal(t, 70000, getTokenRemainQuota(t, token))

				down = false
				oldLimit := constant.TaskQueryLimit
				constant.TaskQueryLimit = 1
				t.Cleanup(func() { constant.TaskQueryLimit = oldLimit })
				require.NoError(t, model.DB.Create(&model.User{Id: user + 10, Username: "next-image-user", AffCode: "next-image-user", Quota: 70000}).Error)
				next := makeTask(user+10, channel, reserved, 0, BillingSourceWallet, 0)
				next.TaskID = "next-live-wallet-image"
				next.PrivateData.AsyncImage = true
				next.PrivateData.PendingImageSettlement = &model.ImageSettlement{Quota: 10000, Data: []byte(`{"state":"succeeded"}`)}
				require.NoError(t, model.DB.Create(next).Error)
				RunTaskPollingOnce(t.Context(), nil)
				won, err = RetryImageSettlement(t.Context(), &pending)
				require.NoError(t, err)
				require.False(t, won, "a stale second worker must not refund again")
				RunTaskPollingOnce(t.Context(), nil)
				var completed model.Task
				require.NoError(t, model.DB.First(&completed, next.ID).Error)
				require.EqualValues(t, model.TaskStatusSuccess, completed.Status)
				require.Equal(t, 90000, getUserQuota(t, user+10))
			} else {
				won, err := CompleteImageTask(t.Context(), task, model.TaskStatusInProgress, result, nil)
				require.NoError(t, err)
				require.True(t, won)
			}
			var final model.Task
			require.NoError(t, model.DB.First(&final, task.ID).Error)
			require.EqualValues(t, model.TaskStatusFailure, final.Status)
			require.Zero(t, final.Quota)
			require.Nil(t, final.PrivateData.PendingImageSettlement)
			require.Empty(t, final.PrivateData.ResultURL)
			require.NotContains(t, string(final.Data), image)
			var body dto.ImageTaskResponse
			require.NoError(t, common.Unmarshal(final.Data, &body))
			require.Empty(t, body.Data.Images)
			require.Equal(t, "wallet_deleted", body.Error["code"])
			var archived model.User
			err = model.DB.Unscoped().First(&archived, user).Error
			if tc.hard {
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			} else {
				require.NoError(t, err)
				require.True(t, archived.DeletedAt.Valid)
				require.Equal(t, 100000, archived.Quota)
			}
			var refunded model.Token
			require.NoError(t, model.DB.First(&refunded, token).Error)
			require.Equal(t, 100000, refunded.RemainQuota)
			require.Zero(t, refunded.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ?", user).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, model.LogTypeRefund, logs[0].Type)
			require.Equal(t, reserved, logs[0].Quota)
		})
	}
}
