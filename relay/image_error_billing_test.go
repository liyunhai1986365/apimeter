package relay

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestImageHelperDoesNotSettleUpstreamErrors(t *testing.T) {
	for _, statusCode := range []int{http.StatusOK, http.StatusBadRequest} {
		for _, usePrice := range []bool{false, true} {
			t.Run(fmt.Sprintf("HTTP_%d_price_%t", statusCode, usePrice), func(t *testing.T) {
				setupImageTaskTestDB(t)
				require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Log{}))
				require.NoError(t, model.DB.Create(&model.User{Id: 7, Username: "image-error-user", Quota: 50000}).Error)
				require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Type: constant.ChannelTypeOpenAI, Name: "image-error-channel"}).Error)
				common.LogConsumeEnabled = true
				previousBatchUpdate := common.BatchUpdateEnabled
				common.BatchUpdateEnabled = false
				t.Cleanup(func() { common.BatchUpdateEnabled = previousBatchUpdate })
				gin.SetMode(gin.TestMode)
				service.InitHttpClient()
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(statusCode)
					_, _ = w.Write([]byte(`{"error":{"message":"image generation failed","type":"server_error","code":"generation_failed"}}`))
				}))
				defer upstream.Close()
				body := []byte(`{"model":"gpt-image-1","prompt":"cat","n":1}`)
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
				common.SetContextKey(ctx, constant.ContextKeyChannelId, 11)
				common.SetContextKey(ctx, constant.ContextKeyChannelKey, "local-upstream-key")
				common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
				defer common.CleanupBodyStorage(ctx)
				billing := &imageAsyncBillingStub{}
				info := &relaycommon.RelayInfo{
					UserId: 7, OriginModelName: "gpt-image-1", StartTime: time.Now(),
					RelayMode: relayconstant.RelayModeImagesGenerations, RequestURLPath: "/v1/images/generations",
					Request:   &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", N: common.GetPointer(uint(1))},
					Billing:   billing,
					PriceData: types.PriceData{UsePrice: usePrice, ModelPrice: 0.01, ModelRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
				}
				apiErr := ImageHelper(ctx, info)
				var logs []model.Log
				require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeConsume).Find(&logs).Error)
				t.Logf("settlement_calls=%d consumption_logs=%d", billing.settleCalls, len(logs))
				if len(logs) > 0 {
					t.Logf("recorded_prompt_tokens=%d recorded_quota=%d", logs[0].PromptTokens, logs[0].Quota)
				}
				require.NotNil(t, apiErr)
				require.Contains(t, apiErr.Error(), "image generation failed")
				require.Zero(t, billing.settleCalls)
				require.Empty(t, logs, "failed image requests must not create consumption logs")
				require.Empty(t, recorder.Body.String(), "the controller must receive the error before any response is written")
				var user model.User
				require.NoError(t, model.DB.First(&user, 7).Error)
				require.Zero(t, user.UsedQuota)
				require.Zero(t, user.RequestCount)
			})
		}
	}
}

func TestImageHelperDoesNotSettleEmptyImages(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeXai} {
		for _, stream := range []bool{false, true} {
			for _, usePrice := range []bool{false, true} {
				for _, body := range []string{`{"data":[]}`, `{"data":[{"url":"","b64_json":""}],"usage":{"input_tokens":99}}`} {
					t.Run(fmt.Sprintf("channel=%d/stream=%t/price=%t/%s", channelType, stream, usePrice, body), func(t *testing.T) {
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							_, _ = fmt.Fprint(w, body)
						}))
						defer upstream.Close()
						request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Stream: common.GetPointer(stream)}
						ctx, recorder, info := newImageResponsePolicyTestContext(t, channelType, upstream.URL, request)
						require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
						common.LogConsumeEnabled = true
						info.PriceData.UsePrice, info.PriceData.ModelPrice = usePrice, 0.01
						apiErr := ImageHelper(ctx, info)
						require.NotNil(t, apiErr)
						require.Contains(t, apiErr.Error(), "image response contains no completed images")
						require.True(t, types.IsSkipRetryError(apiErr))
						require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls)
						require.False(t, ctx.Writer.Written())
						require.Empty(t, recorder.Body.String())
						var user model.User
						require.NoError(t, model.DB.First(&user, 7).Error)
						require.Zero(t, user.UsedQuota)
						var count int64
						require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error)
						require.Zero(t, count)
					})
				}
			}
		}
	}
}
