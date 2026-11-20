package relay

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Interleave the timeout immediately before the atomic reservation handoff.
// No sleeps or scheduler timing are needed to exercise either CAS winner.
type imageReservationInterleave struct {
	relaycommon.BillingSettler
	beforeReserve func()
	commitErr     error
}

func (s *imageReservationInterleave) ReserveWithCommit(quota int, commit func(*gorm.DB, int) error) error {
	if s.beforeReserve != nil {
		s.beforeReserve()
	}
	return s.BillingSettler.ReserveWithCommit(quota, func(tx *gorm.DB, actual int) error {
		if s.commitErr != nil {
			return s.commitErr
		}
		return commit(tx, actual)
	})
}

func setupImageReservationRace(t *testing.T, subscription bool) (*gin.Context, *relaycommon.RelayInfo, *model.Task) {
	t.Helper()
	setupImageTaskTestDB(t)
	// Quota helpers also enqueue cache updates. Drain them before restoring
	// global DB/Redis state, including when an assertion fails mid-test.
	t.Cleanup(func() {
		require.Eventually(t, func() bool { return gopool.WorkerCount() == 0 }, time.Second, time.Millisecond)
	})
	model.InitColForTest()
	oldBatch := common.BatchUpdateEnabled
	common.BatchUpdateEnabled = false
	t.Cleanup(func() { common.BatchUpdateEnabled = oldBatch })
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}, &model.SubscriptionPlan{}, &model.UserSubscription{}, &model.SubscriptionPreConsumeRecord{}))
	require.NoError(t, model.DB.Create(&model.User{Id: 721, Username: "reservation-race", Quota: 100000, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.Token{Id: 722, UserId: 721, Key: "reservation-race-token", Status: common.TokenStatusEnabled, RemainQuota: 100000}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 723, Type: constant.ChannelTypeGemini, Name: "race", Key: "dummy"}).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	info := &relaycommon.RelayInfo{
		RequestId: "image-reservation-race", UserId: 721, TokenId: 722, TokenKey: "reservation-race-token", ForcePreConsume: true,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelId: 723, ChannelType: constant.ChannelTypeGemini},
		PriceData:   types.PriceData{UsePrice: true, ModelPrice: .02, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}, OtherRatios: map[string]float64{"n": 1}},
	}
	if subscription {
		require.NoError(t, model.DB.Create(&model.SubscriptionPlan{Id: 724, Title: "image plan", TotalAmount: 100000, Enabled: true}).Error)
		require.NoError(t, model.DB.Create(&model.UserSubscription{Id: 725, UserId: 721, PlanId: 724, AmountTotal: 100000, Status: "active", StartTime: time.Now().Unix() - 60, EndTime: time.Now().Unix() + 3600}).Error)
		info.BillingSource = service.BillingSourceSubscription
		info.TokenUserSubscriptionId = 725
	}
	require.Nil(t, service.PreConsumeBilling(c, 10000, info))
	task, apiErr := insertLocalImageAsyncTask(info, []byte(`{"n":1}`))
	require.Nil(t, apiErr)
	// startLocalImageAsyncTask gives the worker a separate pricing map.
	info.PriceData.OtherRatios = cloneImageAsyncRatios(info.PriceData.OtherRatios)
	task.Status = model.TaskStatusInProgress
	won, err := task.UpdateWithStatus(model.TaskStatusSubmitted)
	require.NoError(t, err)
	require.True(t, won)
	c.Set(localImageAsyncWorkerKey, true)
	c.Set(localImageAsyncTaskKey, task)
	return c, info, task
}

func TestImagenLocalAsyncReservationTimeoutRace(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		for _, order := range []string{"reservation_first", "timeout_before_reserve", "timeout_refund_after_handoff_rejected", "save_error"} {
			t.Run(fmt.Sprintf("subscription=%v/%s", subscription, order), func(t *testing.T) {
				c, info, task := setupImageReservationRace(t, subscription)
				var timeoutSnapshot model.Task
				require.NoError(t, model.DB.First(&timeoutSnapshot, task.ID).Error)
				initialVersion := task.ImageBase64Version
				timeout := func(refund bool) {
					timeoutSnapshot.Status = model.TaskStatusFailure
					timeoutSnapshot.FailReason = "timeout"
					won, err := timeoutSnapshot.UpdateWithStatus(model.TaskStatusInProgress)
					require.NoError(t, err)
					require.True(t, won)
					if refund {
						require.True(t, service.RefundTaskQuota(context.Background(), &timeoutSnapshot, "timeout"))
					}
				}
				switch order {
				case "timeout_before_reserve":
					timeout(true)
				case "timeout_refund_after_handoff_rejected":
					info.Billing = &imageReservationInterleave{BillingSettler: info.Billing, beforeReserve: func() { timeout(false) }}
				case "save_error":
					info.Billing = &imageReservationInterleave{BillingSettler: info.Billing, commitErr: errors.New("task database unavailable")}
				}
				apiErr := prepareOutboundImageCount(c, info, &dto.ImageRequest{}, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":3}}`))
				if order == "reservation_first" {
					require.Nil(t, apiErr)
					require.Equal(t, 30000, task.Quota)
					require.Greater(t, task.ImageBase64Version, initialVersion)
					// The scanner's old snapshot must lose, even with unchanged status.
					timeoutSnapshot.Status = model.TaskStatusFailure
					won, err := timeoutSnapshot.UpdateWithStatus(model.TaskStatusInProgress)
					require.NoError(t, err)
					require.False(t, won)
					require.Equal(t, initialVersion, timeoutSnapshot.ImageBase64Version)
					require.NoError(t, model.DB.First(&timeoutSnapshot, task.ID).Error)
					require.Equal(t, 30000, timeoutSnapshot.Quota)
					timeout(true)
				} else {
					require.NotNil(t, apiErr)
					require.True(t, kittypes.IsSkipRetryError(apiErr))
					require.Equal(t, 10000, info.Billing.GetPreConsumedQuota(), "failed handoff leaves the initial reservation unchanged")
					require.Equal(t, 10000, info.FinalPreConsumedQuota)
					require.Equal(t, 10000, task.Quota, "failed save must not mutate the worker snapshot")
					require.Equal(t, float64(1), task.PrivateData.BillingContext.OtherRatios["n"])
					if subscription {
						require.EqualValues(t, 10000, info.SubscriptionPreConsumed)
					}
					if order == "timeout_refund_after_handoff_rejected" {
						require.True(t, service.RefundTaskQuota(context.Background(), &timeoutSnapshot, "timeout"))
					}
				}
				var reloaded model.Task
				require.NoError(t, model.DB.First(&reloaded, task.ID).Error)
				finishLocalImageAsyncTask(c, info, &reloaded, nil, kittypes.NewError(errors.New("request stopped"), kittypes.ErrorCodeBadResponse))
				require.Eventually(t, func() bool {
					var user model.User
					var token model.Token
					if model.DB.First(&user, 721).Error != nil || model.DB.First(&token, 722).Error != nil || user.Quota != 100000 || token.RemainQuota != 100000 {
						return false
					}
					if subscription {
						var sub model.UserSubscription
						return model.DB.First(&sub, 725).Error == nil && sub.AmountUsed == 0
					}
					return true
				}, time.Second, time.Millisecond)
			})
		}
	}
}

func TestImagenReservationSameValueUpdateWithChangedRows(t *testing.T) {
	c, info, task := setupImageReservationRace(t, false)
	now := time.Now().Truncate(time.Second)
	model.DB.NowFunc = func() time.Time { return now }
	// Normalize every timestamp before simulating MySQL's changed-rows result.
	require.NoError(t, task.Update())
	version := task.ImageBase64Version
	cols, err := model.DB.Migrator().ColumnTypes(&model.Task{})
	require.NoError(t, err)
	var conditions []string
	for _, column := range cols {
		conditions = append(conditions, fmt.Sprintf(`NEW."%s" IS OLD."%s"`, column.Name(), column.Name()))
	}
	// SQLite normally returns matched rows. Ignore updates only if every
	// column is identical, matching default MySQL affected-row semantics.
	require.NoError(t, model.DB.Exec(`CREATE TRIGGER mysql_noop_rows BEFORE UPDATE ON tasks WHEN `+strings.Join(conditions, " AND ")+` BEGIN SELECT RAISE(IGNORE); END;`).Error)
	apiErr := prepareOutboundImageCount(c, info, &dto.ImageRequest{}, &gemini.Adaptor{}, []byte(`{"instances":[{}],"parameters":{"sampleCount":1}}`))
	require.Nil(t, apiErr)
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Equal(t, 10000, saved.Quota)
	require.Equal(t, version+1, saved.ImageBase64Version)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), saved.Status)
}
