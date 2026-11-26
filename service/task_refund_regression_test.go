package service

import (
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTimeoutRefundRetriesOnNextPoll(t *testing.T) {
	truncate(t)
	timeout, limit := constant.TaskTimeoutMinutes, constant.TaskQueryLimit
	constant.TaskTimeoutMinutes, constant.TaskQueryLimit = 5, 10
	t.Cleanup(func() { constant.TaskTimeoutMinutes, constant.TaskQueryLimit = timeout, limit })
	seedUser(t, 1, 70000)
	seedToken(t, 2, 1, "timeout-refund", 70000)
	seedChannel(t, 3)
	task := makeTask(1, 3, 30000, 2, BillingSourceWallet, 0)
	task.SubmitTime = time.Now().Add(-time.Hour).Unix()
	task.Progress = "30%"
	// The local image job has no upstream polling endpoint; only the timeout
	// sweeper can complete it after the simulated accounting failure.
	task.PrivateData.AsyncImage = true
	require.NoError(t, model.DB.Create(task).Error)
	const hook = "test:timeout_refund_token_write"
	blocked, attempted := true, false
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		if blocked && tx.Statement.Table == "tokens" {
			attempted = true
			tx.AddError(errors.New("temporary token update failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
	check := func(status model.TaskStatus, quota, balance, logs int) {
		t.Helper()
		var saved model.Task
		require.NoError(t, model.DB.First(&saved, task.ID).Error)
		require.Equal(t, status, saved.Status)
		require.Equal(t, quota, saved.Quota)
		require.Equal(t, balance, getUserQuota(t, 1))
		require.Equal(t, balance, getTokenRemainQuota(t, 2))
		require.EqualValues(t, logs, countLogs(t))
	}
	RunTaskPollingOnce(t.Context(), nil)
	require.True(t, attempted)
	check(model.TaskStatusInProgress, 30000, 70000, 0)
	blocked = false
	RunTaskPollingOnce(t.Context(), nil)
	check(model.TaskStatusFailure, 0, 100000, 1)
	RunTaskPollingOnce(t.Context(), nil)
	check(model.TaskStatusFailure, 0, 100000, 1)
}

func TestFailedMediaPollRetriesRefund(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"video", `{"code":"success","data":{"status":"FAILURE","fail_reason":"upstream failed"}}`},
		{"suno", `{"code":"success","data":[{"task_id":"remote","status":"FAILURE","fail_reason":"upstream failed"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 1, 70000)
			seedToken(t, 2, 1, "media-refund", 70000)
			seedChannel(t, 3)
			seedSubscription(t, 1, 1, 100000, 30000)
			task := makeTask(1, 3, 30000, 2, BillingSourceSubscription, 1)
			task.PrivateData.UpstreamTaskID = "remote"
			require.NoError(t, model.DB.Create(task).Error)
			adaptor := fakeSunoPollingAdaptor{response: tc.body}
			oldAdaptor, oldCache := GetTaskAdaptorFunc, common.MemoryCacheEnabled
			common.MemoryCacheEnabled = false
			GetTaskAdaptorFunc = func(constant.TaskPlatform) TaskPollingAdaptor { return adaptor }
			t.Cleanup(func() { GetTaskAdaptorFunc, common.MemoryCacheEnabled = oldAdaptor, oldCache })
			lookup := func() *model.Task {
				var fresh model.Task
				require.NoError(t, model.DB.First(&fresh, task.ID).Error)
				return &fresh
			}
			poll := func(row *model.Task) error {
				tasks := map[string]*model.Task{"remote": row}
				if tc.name == "suno" {
					return updateSunoTasks(t.Context(), 3, []string{"remote"}, tasks)
				}
				ch, err := model.GetChannelById(3, true)
				require.NoError(t, err)
				return updateVideoSingleTask(t.Context(), adaptor, ch, "remote", tasks)
			}
			const hook = "test:media_refund_token_write"
			blocked, attempted := true, false
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
				if blocked && tx.Statement.Table == "tokens" {
					attempted = true
					tx.AddError(errors.New("token write unavailable"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
			stale := *task
			_ = poll(lookup())
			require.True(t, attempted)
			require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), lookup().Status)
			require.Equal(t, 30000, lookup().Quota)
			require.EqualValues(t, 30000, getSubscriptionUsed(t, 1))
			require.Equal(t, 70000, getTokenRemainQuota(t, 2))
			require.Zero(t, countLogs(t))
			blocked = false
			require.NoError(t, poll(lookup()))
			require.NoError(t, poll(&stale))
			require.Equal(t, model.TaskStatus(model.TaskStatusFailure), lookup().Status)
			require.Zero(t, lookup().Quota)
			require.Zero(t, getSubscriptionUsed(t, 1))
			require.Equal(t, 100000, getTokenRemainQuota(t, 2))
			require.Equal(t, 70000, getUserQuota(t, 1))
			require.EqualValues(t, 1, countLogs(t))
		})
	}
}

func TestFailedVideoRefundAfterSubscriptionDeletion(t *testing.T) {
	truncate(t)
	const user, channel, token, sub, reserved = 1, 2, 3, 4, 30000
	seedUser(t, user, 5000)
	seedToken(t, token, user, "deleted-sub-video", 100000-reserved)
	seedChannel(t, channel)
	seedSubscription(t, sub, user, 100000, reserved)
	seedChargedAccounting(t, user, channel, token, reserved, 1)
	task := makeTask(user, channel, reserved, token, BillingSourceSubscription, sub)
	task.PrivateData.UpstreamTaskID = "remote"
	require.NoError(t, model.DB.Create(task).Error)
	stale := *task
	_, err := model.AdminDeleteUserSubscription(sub)
	require.NoError(t, err)
	seedSubscription(t, sub+1, user, 100000, 1200)

	ch, err := model.GetChannelById(channel, true)
	require.NoError(t, err)
	adaptor := fakeSunoPollingAdaptor{response: `{"code":"success","data":{"status":"FAILURE","fail_reason":"upstream failed"}}`}
	for _, row := range []*model.Task{task, &stale} {
		require.NoError(t, updateVideoSingleTask(t.Context(), adaptor, ch, "remote", map[string]*model.Task{"remote": row}))
	}

	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.EqualValues(t, model.TaskStatusFailure, saved.Status)
	require.Equal(t, "upstream failed", saved.FailReason)
	require.Zero(t, saved.Quota)
	require.Equal(t, 100000, getTokenRemainQuota(t, token))
	require.Equal(t, 5000, getUserQuota(t, user))
	require.ErrorIs(t, model.DB.First(&model.UserSubscription{}, sub).Error, gorm.ErrRecordNotFound)
	require.EqualValues(t, 1200, getSubscriptionUsed(t, sub+1))
	require.EqualValues(t, 1, countLogs(t))
	log := getLastLog(t)
	require.Equal(t, model.LogTypeRefund, log.Type)
	require.Equal(t, reserved, log.Quota)
}

func TestTaskRefundCommitsBothAccountsOnce(t *testing.T) {
	for _, source := range []string{BillingSourceWallet, BillingSourceSubscription} {
		t.Run(source, func(t *testing.T) {
			truncate(t)
			const reserved = 30000
			subID := 0
			if source == BillingSourceSubscription {
				subID = 1
				seedSubscription(t, subID, 1, 100000, reserved)
			}
			seedUser(t, 1, 70000)
			seedToken(t, 2, 1, "refund-retry", 70000)
			seedChannel(t, 3)
			seedChargedAccounting(t, 1, 3, 2, reserved, 1)
			task := makeTask(1, 3, reserved, 2, source, subID)
			task.Status = model.TaskStatusFailure
			require.NoError(t, model.DB.Create(task).Error)
			stale := *task
			block := true
			const hook = "test:task_refund_token_write"
			require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
				if block && tx.Statement.Table == "tokens" {
					tx.AddError(errors.New("token write unavailable"))
				}
			}))
			t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
			require.False(t, RefundTaskQuota(t.Context(), task, "upstream failed"))
			require.Equal(t, reserved, getTaskQuota(t, task.ID))
			require.Equal(t, 70000, getUserQuota(t, 1))
			require.Equal(t, 70000, getTokenRemainQuota(t, 2))
			require.Zero(t, countLogs(t))
			if subID != 0 {
				require.EqualValues(t, reserved, getSubscriptionUsed(t, subID))
			}
			block = false
			require.True(t, RefundTaskQuota(t.Context(), task, "retry"))
			RefundTaskQuota(t.Context(), &stale, "stale retry")
			require.Zero(t, getTaskQuota(t, task.ID))
			require.Equal(t, 100000, getTokenRemainQuota(t, 2))
			if subID != 0 {
				require.Zero(t, getSubscriptionUsed(t, subID))
				require.Equal(t, 70000, getUserQuota(t, 1))
			} else {
				require.Equal(t, 100000, getUserQuota(t, 1))
			}
			require.EqualValues(t, 1, countLogs(t))
			used, requests := getUserUsageAccounting(t, 1)
			require.Zero(t, used)
			require.Equal(t, 1, requests)
		})
	}
}

func TestImagePollingChannelFailureDoesNotOverwriteDelivery(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 80000)
	seedToken(t, 2, 1, "channel-lookup", 80000)
	const channel = 933
	newTask := func(id string) *model.Task {
		task := makeTask(1, channel, 10000, 2, BillingSourceWallet, 0)
		task.TaskID, task.PrivateData.UpstreamTaskID = id, id
		task.PrivateData.AsyncImage, task.PrivateData.BillingContext.PerCallBilling = true, true
		require.NoError(t, model.DB.Create(task).Error)
		return task
	}
	done, live := newTask("done"), newTask("live")
	stale := *done
	require.NoError(t, ApplyFetchedImageTask(t.Context(), done, []byte(`{"state":"succeeded","data":{"images":[{"url":"https://example.invalid/image"}]}}`)))
	priorLogs := countLogs(t)
	lookup := func() error {
		return updateImageTasks(t.Context(), channel, []string{"done", "live"}, map[string]*model.Task{"done": &stale, "live": live})
	}
	// Simulate the delay between creating a DB channel and refreshing this node's cache.
	seedChannel(t, channel)
	previousCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	t.Cleanup(func() { common.MemoryCacheEnabled = previousCache })
	require.Error(t, lookup())
	require.EqualValues(t, model.TaskStatusInProgress, live.Status)
	require.NoError(t, model.DB.Delete(&model.Channel{}, channel).Error)
	require.Error(t, lookup())
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, done.ID).Error)
	require.EqualValues(t, model.TaskStatusSuccess, saved.Status)
	require.True(t, saved.ImageQuotaSettled())
	require.Equal(t, 10000, saved.Quota)
	require.EqualValues(t, model.TaskStatusFailure, live.Status)
	require.Zero(t, getTaskQuota(t, live.ID))
	require.Equal(t, 90000, getUserQuota(t, 1))
	require.Equal(t, 90000, getTokenRemainQuota(t, 2))
	require.Equal(t, priorLogs+1, countLogs(t))
}

func TestFetchedImageRefundCanRetryAfterTokenFailure(t *testing.T) {
	truncate(t)
	seedUser(t, 1, 70000)
	seedToken(t, 2, 1, "failed-image", 70000)
	seedChannel(t, 3)
	seedChargedAccounting(t, 1, 3, 2, 50000, 2)
	task := makeTask(1, 3, 30000, 2, BillingSourceWallet, 0)
	task.PrivateData.AsyncImage = true
	require.NoError(t, model.DB.Create(task).Error)
	stale := *task
	response := []byte(`{"state":"failed","error":{"message":"upstream failed"}}`)
	const hook = "test:image_refund_token_write"
	block := true
	require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register(hook, func(tx *gorm.DB) {
		if block && tx.Statement.Table == "tokens" {
			tx.AddError(errors.New("temporary token failure"))
		}
	}))
	t.Cleanup(func() { require.NoError(t, model.DB.Callback().Update().Remove(hook)) })
	require.Error(t, ApplyFetchedImageTask(t.Context(), task, response))
	var pending model.Task
	require.NoError(t, model.DB.First(&pending, task.ID).Error)
	require.EqualValues(t, model.TaskStatusInProgress, pending.Status)
	require.Equal(t, 30000, pending.Quota)
	require.Equal(t, 70000, getUserQuota(t, 1))
	require.Zero(t, countLogs(t))
	block = false
	require.NoError(t, ApplyFetchedImageTask(t.Context(), &pending, response))
	require.NoError(t, ApplyFetchedImageTask(t.Context(), &stale, response))
	require.EqualValues(t, model.TaskStatusFailure, pending.Status)
	require.Zero(t, getTaskQuota(t, task.ID))
	require.Equal(t, 100000, getUserQuota(t, 1))
	require.Equal(t, 100000, getTokenRemainQuota(t, 2))
	require.EqualValues(t, 1, countLogs(t))
	used, requests := getUserUsageAccounting(t, 1)
	require.Equal(t, 50000, used)
	require.Equal(t, 2, requests)
	require.EqualValues(t, 50000, getChannelUsedQuota(t, 3))
}

func TestImageResponsePricingWithAndWithoutUsage(t *testing.T) {
	const sizePrice = `pixels(response("data.images.0.size")) > 2360000 ? tier("large", 600000.0) : tier("small", 300000.0)`
	for _, tc := range []struct {
		name, expr, usage string
		want, logs        int
	}{
		{"reported", sizePrice, `,"usage":{"input_tokens":1,"total_tokens":1}`, 300000, 1},
		{"omitted", sizePrice, "", 300000, 1},
		{"token-dependent", `tier("tokens", p * 2 + img_o * 10)`, "", 150000, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			truncate(t)
			seedUser(t, 1, 850000)
			seedToken(t, 2, 1, "image-size", 850000)
			seedChannel(t, 3)
			task := makeTask(1, 3, 150000, 2, BillingSourceWallet, 0)
			task.PrivateData.AsyncImage = true
			task.PrivateData.UpstreamTaskID = "remote-image"
			task.PrivateData.BillingContext.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
				BillingMode: "tiered_expr", GroupRatio: 1, QuotaPerUnit: 500000,
				ExprVersion: 1, EstimatedQuotaAfterGroup: 150000, ExprString: tc.expr,
			}
			require.NoError(t, model.DB.Create(task).Error)
			body := []byte(`{"state":"succeeded","data":{"images":[{"url":"https://example.invalid/image","size":"4096x4096"}]}` + tc.usage + "}")
			require.NoError(t, ApplyFetchedImageTask(t.Context(), task, body))
			require.NoError(t, ApplyFetchedImageTask(t.Context(), task, body))
			require.Equal(t, tc.want, getTaskQuota(t, task.ID))
			require.Equal(t, 1000000-tc.want, getUserQuota(t, 1))
			require.Equal(t, 1000000-tc.want, getTokenRemainQuota(t, 2))
			require.EqualValues(t, tc.logs, countLogs(t))
			if tc.logs != 0 {
				require.Equal(t, tc.want, getLastLog(t).Quota)
			}
		})
	}
}
