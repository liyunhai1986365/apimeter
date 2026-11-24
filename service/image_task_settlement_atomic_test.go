package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestScheduledImageSettlementAfterSubscriptionLimit(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		t.Run(fmt.Sprintf("concurrent_refund_%t", parallel), func(t *testing.T) {
			truncate(t)
			oldLimit := constant.TaskQueryLimit
			constant.TaskQueryLimit = 1
			t.Cleanup(func() { constant.TaskQueryLimit = oldLimit })
			const userID, tokenID, channelID, subID = 971, 972, 973, 974
			const reserved = 30000
			seedUser(t, userID, 70000)
			seedToken(t, tokenID, userID, "image-cap-refund", 40000)
			seedChannel(t, channelID)
			seedSubscription(t, subID, userID, 100000, 100000)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", 60000).Error)
			const callback = "test:image_settlement_refund_write_unavailable"
			var down atomic.Bool
			down.Store(true)
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
				if down.Load() && tx.Statement.Table == "tokens" {
					tx.AddError(errors.New("token update temporarily unavailable"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(callback)) })
			newPending := func(source string, id, delivered int) *model.Task {
				task := makeTask(userID, channelID, reserved, tokenID, source, id)
				task.TaskID = "pending-" + source
				task.PrivateData.AsyncImage = true
				task.PrivateData.BillingContext.PerCallBilling = true
				task.PrivateData.BillingContext.OtherRatios = map[string]float64{"n": 3}
				require.NoError(t, model.DB.Create(task).Error)
				res := dto.ImageTaskResponse{ID: task.TaskID, State: "succeeded"}
				for range delivered {
					res.Data.Images = append(res.Data.Images, dto.ImageTaskImage{B64Json: "cHJpdmF0ZQ=="})
				}
				var err error
				task.Data, err = common.Marshal(res)
				require.NoError(t, err)
				task.Status = model.TaskStatusSuccess
				won, err := CompleteImageTask(t.Context(), task, model.TaskStatusInProgress, res, nil)
				require.ErrorIs(t, err, model.ErrImageTaskTokenReservation)
				require.False(t, won)
				return task
			}
			load := func(id int64) model.Task {
				var task model.Task
				require.NoError(t, model.DB.First(&task, id).Error)
				return task
			}
			first := newPending(BillingSourceSubscription, subID, 4)
			next := newPending(BillingSourceWallet, 0, 1)
			RunTaskPollingOnce(t.Context(), nil)
			require.EqualValues(t, 100000, getSubscriptionUsed(t, subID))
			require.Equal(t, 40000, getTokenRemainQuota(t, tokenID))
			require.Equal(t, 70000, getUserQuota(t, userID))
			require.NotNil(t, load(first.ID).PrivateData.PendingImageSettlement)
			down.Store(false)
			if parallel {
				const workers = 5
				type attempt struct {
					won bool
					err error
				}
				start, done := make(chan struct{}), make(chan attempt, workers)
				for range workers {
					snapshot := load(first.ID)
					go func() {
						<-start
						won, err := RetryImageSettlement(t.Context(), &snapshot)
						done <- attempt{won, err}
					}()
				}
				close(start)
				var winners int
				for range workers {
					res := <-done
					require.NoError(t, res.err)
					if res.won {
						winners++
					}
				}
				require.Equal(t, 1, winners)
			} else {
				RunTaskPollingOnce(t.Context(), nil)
			}
			failed := load(first.ID)
			require.EqualValues(t, model.TaskStatusFailure, failed.Status)
			require.Zero(t, failed.Quota)
			require.Contains(t, failed.FailReason, "subscription quota")
			require.Nil(t, failed.PrivateData.PendingImageSettlement)
			require.Empty(t, failed.PrivateData.ResultURL)
			require.NotContains(t, string(failed.Data), "cHJpdmF0ZQ==")
			var public dto.ImageTaskResponse
			require.NoError(t, common.Unmarshal(failed.Data, &public))
			require.Empty(t, public.Data.Images)
			require.Equal(t, "failed", public.State)
			require.Equal(t, "insufficient_subscription_quota", public.Error["code"])
			require.EqualValues(t, 70000, getSubscriptionUsed(t, subID))
			require.Equal(t, 70000, getTokenRemainQuota(t, tokenID))
			require.Equal(t, 70000, getUserQuota(t, userID))
			var refunds []model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeRefund).Find(&refunds).Error)
			require.Len(t, refunds, 1)
			require.Equal(t, reserved, refunds[0].Quota)
			require.NotNil(t, load(next.ID).PrivateData.PendingImageSettlement)
			RunTaskPollingOnce(t.Context(), nil)
			RunTaskPollingOnce(t.Context(), nil)
			completed := load(next.ID)
			require.EqualValues(t, model.TaskStatusSuccess, completed.Status)
			require.Equal(t, 10000, completed.Quota)
			require.Nil(t, completed.PrivateData.PendingImageSettlement)
			require.Equal(t, 90000, getTokenRemainQuota(t, tokenID))
			require.Equal(t, 90000, getUserQuota(t, userID))
			var token model.Token
			require.NoError(t, model.DB.First(&token, tokenID).Error)
			require.Equal(t, 10000, token.UsedQuota)
			var consumes []model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeConsume).Find(&consumes).Error)
			require.Len(t, consumes, 1)
			require.Equal(t, 10000, consumes[0].Quota)
		})
	}
}

func TestImageSettlementAtomicRetry(t *testing.T) {
	for _, source := range []string{BillingSourceWallet, BillingSourceSubscription} {
		for _, delivered := range []int{1, 4} {
			t.Run(fmt.Sprintf("%s/delivered_%d", source, delivered), func(t *testing.T) {
				truncate(t)
				const initial, reserved = 100000, 30000
				userBalance := initial - reserved
				subID := 0
				if source == BillingSourceSubscription {
					userBalance, subID = initial, 944
					seedSubscription(t, subID, 941, initial, reserved)
				}
				seedUser(t, 941, userBalance)
				seedToken(t, 942, 941, "image-settlement-token", initial-reserved)
				seedChannel(t, 943)
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 942).Update("used_quota", reserved).Error)
				task := makeTask(941, 943, reserved, 942, source, subID)
				task.Status, task.SubmitTime = model.TaskStatusInProgress, time.Now().Add(-48*time.Hour).Unix()
				task.PrivateData.AsyncImage = true
				task.PrivateData.BillingContext.PerCallBilling = true
				task.PrivateData.BillingContext.OtherRatios = map[string]float64{"n": 3}
				require.NoError(t, model.DB.Create(task).Error)
				load := func() model.Task {
					var fresh model.Task
					require.NoError(t, model.DB.First(&fresh, task.ID).Error)
					return fresh
				}
				logs := func() int64 {
					var n int64
					require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("user_id = ? AND type = ?", 941, model.LogTypeConsume).Count(&n).Error)
					return n
				}
				verify := func(cost int) {
					t.Helper()
					wantWallet := initial - cost
					if subID != 0 {
						wantWallet = initial
						require.EqualValues(t, cost, getSubscriptionUsed(t, subID))
					}
					require.Equal(t, wantWallet, getUserQuota(t, 941))
					var token model.Token
					require.NoError(t, model.DB.First(&token, 942).Error)
					require.Equal(t, initial-cost, token.RemainQuota)
					require.Equal(t, cost, token.UsedQuota)
				}

				staleTimeout := load()
				const hook = "test:reject_once_image_settlement_token_update"
				var failed atomic.Bool
				require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
					if tx.Statement.Table == "tokens" && failed.CompareAndSwap(false, true) {
						tx.AddError(errors.New("injected token update failure"))
					}
				}))
				t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
				result := dto.ImageTaskResponse{}
				for i := 0; i < delivered; i++ {
					result.Data.Images = append(result.Data.Images, dto.ImageTaskImage{URL: fmt.Sprintf("https://image.invalid/%d", i)})
				}
				body, err := common.Marshal(result)
				require.NoError(t, err)
				task.Data, task.Status, task.PrivateData.ResultURL = body, model.TaskStatusSuccess, result.Data.Images[0].URL
				won, err := CompleteImageTask(context.Background(), task, model.TaskStatusInProgress, result, body)
				require.Error(t, err)
				require.False(t, won)
				verify(reserved)
				pending := load()
				require.EqualValues(t, model.TaskStatusInProgress, pending.Status)
				require.Equal(t, reserved, pending.Quota)
				require.NotNil(t, pending.PrivateData.PendingImageSettlement)
				require.Empty(t, pending.Data)
				require.Empty(t, pending.PrivateData.ResultURL)
				require.Zero(t, logs())
				require.Empty(t, model.GetTimedOutUnfinishedTasks(time.Now().Unix(), 100))
				staleTimeout.Status = model.TaskStatusFailure
				won, err = staleTimeout.UpdateWithStatus(model.TaskStatusInProgress)
				require.NoError(t, err)
				require.False(t, won)

				const workers = 5
				type attempt struct {
					won bool
					err error
				}
				start, done := make(chan struct{}), make(chan attempt, workers)
				for i := 0; i < workers; i++ {
					snapshot := load()
					go func() {
						<-start
						won, err := RetryImageSettlement(context.Background(), &snapshot)
						done <- attempt{won, err}
					}()
				}
				close(start)
				var winners int
				for i := 0; i < workers; i++ {
					out := <-done
					require.NoError(t, out.err)
					if out.won {
						winners++
					}
				}
				require.Equal(t, 1, winners)
				completed := load()
				require.Nil(t, completed.PrivateData.PendingImageSettlement)
				require.True(t, completed.ImageQuotaSettled())
				require.EqualValues(t, model.TaskStatusSuccess, completed.Status)
				require.Equal(t, 10000*delivered, completed.Quota)
				require.JSONEq(t, string(body), string(completed.Data))
				require.Equal(t, result.Data.Images[0].URL, completed.PrivateData.ResultURL)
				verify(completed.Quota)
				require.EqualValues(t, 1, logs())
			})
		}
	}
}

func TestImageSettlementDeletedSubscription(t *testing.T) {
	for _, delivered := range []int{1, 3, 4} {
		t.Run(fmt.Sprintf("delivered_%d", delivered), func(t *testing.T) {
			truncate(t)
			const userID, tokenID, channelID, subID, reserved = 981, 982, 983, 984, 30000
			seedUser(t, userID, 123456)
			seedToken(t, tokenID, userID, "deleted-image-subscription", 70000)
			seedChannel(t, channelID)
			seedSubscription(t, subID, userID, 100000, reserved)
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", tokenID).Update("used_quota", reserved).Error)
			task := makeTask(userID, channelID, reserved, tokenID, BillingSourceSubscription, subID)
			task.PrivateData.AsyncImage, task.PrivateData.UpstreamTaskID = true, "provider-id"
			task.PrivateData.BillingContext.PerCallBilling = true
			task.PrivateData.BillingContext.OtherRatios = map[string]float64{"n": 3}
			require.NoError(t, model.DB.Create(task).Error)
			result := dto.ImageTaskResponse{State: "succeeded"}
			for range delivered {
				result.Data.Images = append(result.Data.Images, dto.ImageTaskImage{B64Json: "cHJpdmF0ZQ=="})
			}
			var err error
			task.Data, err = common.Marshal(result)
			require.NoError(t, err)
			task.Status = model.TaskStatusSuccess
			const readHook, writeHook = "test:image_subscription_read_failed", "test:image_token_refund_failed"
			var readDown, writeDown atomic.Bool
			readDown.Store(true)
			readErr := errors.New("subscription read temporarily unavailable")
			require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(readHook, func(tx *gorm.DB) {
				if readDown.Load() && tx.Statement.Table == "user_subscriptions" {
					tx.AddError(readErr)
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(readHook)) })
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(writeHook, func(tx *gorm.DB) {
				if writeDown.Load() && tx.Statement.Table == "tokens" {
					tx.AddError(errors.New("token refund unavailable"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(writeHook)) })
			load := func() model.Task {
				var row model.Task
				require.NoError(t, model.DB.First(&row, task.ID).Error)
				return row
			}
			verifyPending := func() {
				row := load()
				require.NotNil(t, row.PrivateData.PendingImageSettlement)
				require.EqualValues(t, model.TaskStatusInProgress, row.Status)
				require.Equal(t, reserved, row.Quota)
				require.Equal(t, 70000, getTokenRemainQuota(t, tokenID))
				require.Equal(t, 123456, getUserQuota(t, userID))
			}
			won, err := CompleteImageTask(t.Context(), task, model.TaskStatusInProgress, result, nil)
			require.ErrorIs(t, err, readErr)
			require.NotErrorIs(t, err, model.ErrImageTaskSubscriptionDeleted)
			require.False(t, won)
			verifyPending()
			readDown.Store(false)
			_, err = model.AdminDeleteUserSubscription(subID)
			require.NoError(t, err)
			writeDown.Store(true)
			won, err = RetryImageSettlement(t.Context(), task)
			require.ErrorIs(t, err, model.ErrImageTaskTokenReservation)
			require.False(t, won)
			verifyPending()
			writeDown.Store(false)
			const workers = 5
			type attempt struct {
				won bool
				err error
			}
			start, done := make(chan struct{}), make(chan attempt, workers)
			for range workers {
				snapshot := load()
				go func() { <-start; won, err := RetryImageSettlement(t.Context(), &snapshot); done <- attempt{won, err} }()
			}
			close(start)
			wins := 0
			for range workers {
				out := <-done
				require.NoError(t, out.err)
				if out.won {
					wins++
				}
			}
			require.Equal(t, 1, wins)
			row := load()
			require.EqualValues(t, model.TaskStatusFailure, row.Status)
			require.Zero(t, row.Quota)
			require.Nil(t, row.PrivateData.PendingImageSettlement)
			require.Empty(t, row.PrivateData.ResultURL)
			require.NotContains(t, string(row.Data), "cHJpdmF0ZQ==")
			var public dto.ImageTaskResponse
			require.NoError(t, common.Unmarshal(row.Data, &public))
			require.Equal(t, "subscription_deleted", public.Error["code"])
			require.Empty(t, public.Data.Images)
			var subscriptions int64
			require.NoError(t, model.DB.Unscoped().Model(&model.UserSubscription{}).Where("id = ?", subID).Count(&subscriptions).Error)
			require.Zero(t, subscriptions, "a refunded task must not recreate the deleted subscription")
			require.Equal(t, 123456, getUserQuota(t, userID))
			var token model.Token
			require.NoError(t, model.DB.First(&token, tokenID).Error)
			require.Equal(t, 100000, token.RemainQuota)
			require.Zero(t, token.UsedQuota)
			var logs []model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ?", userID).Find(&logs).Error)
			require.Len(t, logs, 1)
			require.Equal(t, model.LogTypeRefund, logs[0].Type)
			require.Equal(t, reserved, logs[0].Quota)
		})
	}
}

func TestImageSettlementWaitsForSubscriptionDeletion(t *testing.T) {
	if common.UsingSQLite {
		t.Skip("requires independent connections and row locks")
	}
	for _, commit := range []bool{false, true} {
		t.Run(fmt.Sprintf("delete_commits_%t", commit), func(t *testing.T) {
			truncate(t)
			const userID, tokenID, channelID, subID = 991, 992, 993, 994
			seedUser(t, userID, 123456)
			seedToken(t, tokenID, userID, "image-delete-race", 70000)
			seedChannel(t, channelID)
			seedSubscription(t, subID, userID, 100000, 30000)
			task := makeTask(userID, channelID, 30000, tokenID, BillingSourceSubscription, subID)
			task.PrivateData.AsyncImage = true
			task.PrivateData.PendingImageSettlement = &model.ImageSettlement{Quota: 10000, Data: []byte(`{"state":"succeeded"}`)}
			require.NoError(t, model.DB.Create(task).Error)
			deletion := model.DB.Begin()
			require.NoError(t, deletion.Error)
			defer deletion.Rollback()
			require.NoError(t, deletion.Delete(&model.UserSubscription{}, subID).Error)
			checkStarted := make(chan struct{}, 1)
			const hook = "test:image_waiting_for_subscription_delete"
			require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == "user_subscriptions" {
					select {
					case checkStarted <- struct{}{}:
					default:
					}
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Query().Remove(hook)) })
			type attempt struct {
				won bool
				err error
			}
			done := make(chan attempt, 1)
			go func() { won, err := RetryImageSettlement(t.Context(), task); done <- attempt{won, err} }()
			select {
			case <-checkStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("settlement did not try to read the subscription")
			}
			if commit {
				require.NoError(t, deletion.Commit().Error)
			} else {
				require.NoError(t, deletion.Rollback().Error)
			}
			result := <-done
			require.NoError(t, result.err)
			require.True(t, result.won)
			if commit {
				require.EqualValues(t, model.TaskStatusFailure, task.Status)
				require.Equal(t, 100000, getTokenRemainQuota(t, tokenID))
			} else {
				require.EqualValues(t, model.TaskStatusSuccess, task.Status)
				require.Equal(t, 90000, getTokenRemainQuota(t, tokenID))
				require.EqualValues(t, 10000, getSubscriptionUsed(t, subID))
			}
			require.Equal(t, 123456, getUserQuota(t, userID))
		})
	}
}
