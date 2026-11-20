package relay

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

func TestImagenReservationWriteOutageKeepsOriginalRefundOwner(t *testing.T) {
	for _, failure := range []string{"all_writes", "wallet", "subscription", "token"} {
		t.Run(failure, func(t *testing.T) {
			c, info, task := setupImageReservationRace(t, failure == "subscription")
			version := task.ImageBase64Version
			tables := map[string][]string{
				"all_writes": {"tasks", "users", "tokens"},
				"wallet":     {"users"}, "subscription": {"user_subscriptions"}, "token": {"tokens"},
			}[failure]
			for _, table := range tables {
				require.NoError(t, model.DB.Exec("CREATE TRIGGER image_outage_"+table+" BEFORE UPDATE ON "+table+" BEGIN SELECT RAISE(ABORT, 'temporary database write outage'); END").Error)
			}
			apiErr := prepareOutboundImageCount(c, info, &dto.ImageRequest{}, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":3}}`))
			require.NotNil(t, apiErr)
			if failure == "token" || failure == "subscription" {
				require.Equal(t, http.StatusForbidden, apiErr.StatusCode, "preserve existing quota error classification")
			}
			require.Equal(t, 10000, info.Billing.GetPreConsumedQuota())
			require.Equal(t, 10000, info.FinalPreConsumedQuota)
			require.Equal(t, 10000, task.Quota)
			require.Equal(t, version, task.ImageBase64Version)
			require.Equal(t, float64(1), task.PrivateData.BillingContext.OtherRatios["n"])
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.Equal(t, 10000, saved.Quota)
			require.Equal(t, version, saved.ImageBase64Version, "CAS must roll back if a later account write fails")
			var token model.Token
			require.NoError(t, model.DB.First(&token, 722).Error)
			require.Equal(t, 90000, token.RemainQuota)
			var user model.User
			require.NoError(t, model.DB.First(&user, 721).Error)
			if failure == "subscription" {
				require.Equal(t, 100000, user.Quota)
				var sub model.UserSubscription
				require.NoError(t, model.DB.First(&sub, 725).Error)
				require.EqualValues(t, 10000, sub.AmountUsed)
			} else {
				require.Equal(t, 90000, user.Quota)
			}
			if failure == "all_writes" {
				// Reproduce the original outage: even the worker's failure CAS
				// fails, so only a later timeout can recover the published quota.
				finishLocalImageAsyncTask(c, info, &saved, nil, apiErr)
			}
			for _, table := range tables {
				require.NoError(t, model.DB.Exec("DROP TRIGGER image_outage_"+table).Error)
			}
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), saved.Status)
			saved.Status = model.TaskStatusFailure
			saved.FailReason = "timeout after database recovery"
			won, err := saved.UpdateWithStatus(model.TaskStatusInProgress)
			require.NoError(t, err)
			require.True(t, won)
			require.True(t, service.RefundTaskQuota(context.Background(), &saved, saved.FailReason))
			require.Zero(t, saved.Quota)
			require.True(t, service.RefundTaskQuota(context.Background(), &saved, "duplicate timeout"))
			require.NoError(t, model.DB.First(&user, 721).Error)
			require.NoError(t, model.DB.First(&token, 722).Error)
			require.Equal(t, 100000, user.Quota)
			require.Equal(t, 100000, token.RemainQuota, "no unpublished token debit may survive the outage")
			if failure == "subscription" {
				var sub model.UserSubscription
				require.NoError(t, model.DB.First(&sub, 725).Error)
				require.Zero(t, sub.AmountUsed)
			}
		})
	}
}

func TestImagenTimeoutDoesNotNeedUnpublishedReservationRefund(t *testing.T) {
	for _, failedRefund := range []string{"wallet", "subscription", "token"} {
		t.Run(failedRefund, func(t *testing.T) {
			c, info, task := setupImageReservationRace(t, failedRefund == "subscription")
			trigger := map[string]string{
				"wallet":       `BEFORE UPDATE OF quota ON users WHEN NEW.quota - OLD.quota = 20000`,
				"subscription": `BEFORE UPDATE OF amount_used ON user_subscriptions WHEN OLD.amount_used - NEW.amount_used = 20000`,
				"token":        `BEFORE UPDATE OF remain_quota ON tokens WHEN NEW.remain_quota - OLD.remain_quota = 20000`,
			}[failedRefund]
			require.NoError(t, model.DB.Exec("CREATE TRIGGER fail_unpublished_refund "+trigger+" BEGIN SELECT RAISE(ABORT, 'unpublished refund unavailable'); END").Error)
			info.Billing = &imageReservationInterleave{BillingSettler: info.Billing, beforeReserve: func() {
				var timeout model.Task
				require.NoError(t, model.DB.First(&timeout, task.ID).Error)
				timeout.Status = model.TaskStatusFailure
				won, err := timeout.UpdateWithStatus(model.TaskStatusInProgress)
				require.NoError(t, err)
				require.True(t, won)
				require.True(t, service.RefundTaskQuota(context.Background(), &timeout, "timeout"))
			}}
			require.NotNil(t, prepareOutboundImageCount(c, info, &dto.ImageRequest{}, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":3}}`)))
			require.NoError(t, model.DB.Exec("DROP TRIGGER fail_unpublished_refund").Error)
			var user model.User
			var token model.Token
			require.NoError(t, model.DB.First(&user, 721).Error)
			require.NoError(t, model.DB.First(&token, 722).Error)
			require.Equal(t, 100000, user.Quota)
			require.Equal(t, 100000, token.RemainQuota)
			if failedRefund == "subscription" {
				var sub model.UserSubscription
				require.NoError(t, model.DB.First(&sub, 725).Error)
				require.Zero(t, sub.AmountUsed)
			}
		})
	}
}
