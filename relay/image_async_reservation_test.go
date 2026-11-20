package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type imageReservationTransport func(*http.Request) (*http.Response, error)

func (f imageReservationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestImagenLocalAsyncReservationAndSettlement(t *testing.T) {
	for _, tc := range []struct {
		name                         string
		requested, outbound, valid   int
		balance                      int
		upstreamFailure, reserveFail bool
	}{
		{name: "increased_partial", requested: 1, outbound: 3, valid: 1, balance: 100000},
		{name: "increased_all", requested: 1, outbound: 3, valid: 3, balance: 100000},
		{name: "decreased", requested: 3, outbound: 1, valid: 1, balance: 100000},
		{name: "all_filtered", requested: 1, outbound: 3, balance: 100000},
		{name: "upstream_failure", requested: 1, outbound: 3, balance: 100000, upstreamFailure: true},
		{name: "reserve_failure", requested: 1, outbound: 3, balance: 15000, reserveFail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupImageTaskTestDB(t)
			model.InitColForTest()
			gin.SetMode(gin.TestMode)
			common.LogConsumeEnabled = true
			oldBatch := common.BatchUpdateEnabled
			common.BatchUpdateEnabled = false
			t.Cleanup(func() {
				waitForImageTaskTestWorkers(t)
				common.BatchUpdateEnabled = oldBatch
			})
			require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Log{}))
			require.NoError(t, model.DB.Create(&model.User{Id: 701, Username: "async-imagen", Quota: tc.balance, Status: common.UserStatusEnabled}).Error)
			require.NoError(t, model.DB.Create(&model.Token{Id: 702, UserId: 701, Name: "token", Key: "async-imagen-token", Status: common.TokenStatusEnabled, RemainQuota: tc.balance}).Error)
			require.NoError(t, model.DB.Create(&model.Channel{Id: 703, Type: constant.ChannelTypeGemini, Key: "dummy", Name: "gemini", Status: common.ChannelStatusEnabled}).Error)
			body := []byte(fmt.Sprintf(`{"model":"imagen-4.0-generate-001","prompt":"cat","n":%d,"response_format":"b64_json"}`, tc.requested))
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations?async=true", bytes.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
			common.SetContextKey(ctx, constant.ContextKeyChannelId, 703)
			common.SetContextKey(ctx, constant.ContextKeyChannelKey, "dummy")
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, "https://dummy.invalid")
			common.SetContextKey(ctx, constant.ContextKeyChannelParamOverride, map[string]any{"parameters": map[string]any{"sampleCount": tc.outbound}})
			info := &relaycommon.RelayInfo{
				UserId: 701, TokenId: 702, TokenKey: "async-imagen-token", UsingGroup: "default", OriginModelName: "imagen-4.0-generate-001",
				RelayMode: relayconstant.RelayModeImagesGenerations, RequestURLPath: ctx.Request.URL.String(), IsAsyncImageRequest: true, ForcePreConsume: true,
				Request:     &dto.ImageRequest{Model: "imagen-4.0-generate-001", Prompt: "cat", N: common.GetPointer(uint(tc.requested)), ResponseFormat: "b64_json"},
				ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeGemini, ChannelId: 703, ChannelBaseUrl: "https://dummy.invalid", ApiKey: "dummy"},
				PriceData:   types.PriceData{UsePrice: true, ModelPrice: .02, QuotaToPreConsume: tc.requested * 10000, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}, OtherRatios: map[string]float64{"n": float64(tc.requested)}},
			}
			require.Nil(t, service.PreConsumeBilling(ctx, tc.requested*10000, info))
			task, apiErr := insertLocalImageAsyncTask(info, body)
			require.Nil(t, apiErr)
			service.InitHttpClient()
			client := service.GetHttpClient()
			oldTransport := client.Transport
			t.Cleanup(func() {
				waitForImageTaskTestWorkers(t)
				client.Transport = oldTransport
			})
			calls := 0
			client.Transport = imageReservationTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				sent, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprint(tc.outbound), gjsonGetString(t, sent, "parameters.sampleCount"))
				var saved model.Task
				require.NoError(t, model.DB.First(&saved, task.ID).Error)
				require.Equal(t, max(tc.requested, tc.outbound)*10000, saved.Quota, "save the actual reservation before sending")
				require.Equal(t, float64(tc.outbound), saved.PrivateData.BillingContext.OtherRatios["n"])
				var predictions []string
				for i := 0; i < tc.outbound; i++ {
					if i < tc.valid {
						predictions = append(predictions, fmt.Sprintf(`{"bytesBase64Encoded":%q}`, imageTaskTestPNGBase64))
					} else {
						predictions = append(predictions, `{"raiFilteredReason":"blocked"}`)
					}
				}
				status, response := http.StatusOK, `{"predictions":[`+strings.Join(predictions, ",")+`]}`
				if tc.upstreamFailure {
					status, response = http.StatusInternalServerError, `{"error":{"message":"upstream failed"}}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(response)), Request: req}, nil
			})
			// Match startLocalImageAsyncTask, including the pricing map copy.
			workerInfo := *info
			workerInfo.PriceData.OtherRatios = cloneImageAsyncRatios(info.PriceData.OtherRatios)
			worker := cloneImageAsyncGinContext(ctx, body)
			runLocalImageAsyncTask(worker, &workerInfo, task)
			want := tc.valid * 10000
			if tc.reserveFail || tc.upstreamFailure {
				want = 0
			}
			require.Eventually(t, func() bool {
				var user model.User
				var token model.Token
				return model.DB.First(&user, 701).Error == nil && model.DB.First(&token, 702).Error == nil && user.Quota == tc.balance-want && token.RemainQuota == tc.balance-want
			}, time.Second, time.Millisecond)
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			var logs []model.Log
			require.NoError(t, model.DB.Where("user_id = ? AND type = ?", 701, model.LogTypeConsume).Find(&logs).Error)
			if want > 0 {
				require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
				require.Equal(t, want, saved.Quota)
				require.Equal(t, float64(tc.valid), saved.PrivateData.BillingContext.OtherRatios["n"])
				require.Len(t, logs, 1)
				require.Equal(t, want, logs[0].Quota)
				// Re-entering a finished task must not charge or log a second time.
				finishLocalImageAsyncTask(worker, &workerInfo, &saved, nil, nil)
				var user model.User
				require.NoError(t, model.DB.First(&user, 701).Error)
				require.Equal(t, want, user.UsedQuota)
				require.Equal(t, 1, user.RequestCount)
			} else {
				require.Equal(t, model.TaskStatus(model.TaskStatusFailure), saved.Status)
				require.Empty(t, logs)
			}
			if tc.reserveFail {
				require.Zero(t, calls)
			} else {
				require.Equal(t, 1, calls)
			}
			require.Equal(t, uint(tc.requested), *info.Request.(*dto.ImageRequest).N)
		})
	}
}
