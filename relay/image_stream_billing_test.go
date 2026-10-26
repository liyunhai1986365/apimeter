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

func TestImageHelperStreamBilling(t *testing.T) {
	const partial = "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"preview\"}\n\n"
	const completed = "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"" + imageTaskTestPNGBase64 + "\",\"usage\":{\"input_tokens\":10,\"output_tokens\":20,\"total_tokens\":30}}\n\n"
	for _, tt := range []struct {
		name        string
		contentType string
		body        string
		wantError   bool
		wantStream  bool
		requestedN  uint
		billedN     int
	}{
		{"multiple completed with late usage", "text/event-stream", partial + completed + "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",\"b64_json\":\"" + imageTaskTestPNGBase64 + "\"}\n\ndata: {\"usage\":{\"input_tokens\":10,\"output_tokens\":20}}\n\ndata: [DONE]\n\n", false, true, 3, 2},
		{"one success then error", "text/event-stream", completed + "event: error\ndata: {\"type\":\"error\",\"message\":\"second image failed\"}\n\n", false, true, 3, 1},
		{"upstream read failure", "text/event-stream", completed, false, true, 3, 1},
		{"usage without image", "text/event-stream", "data: {\"usage\":{\"input_tokens\":10,\"output_tokens\":20}}\n\ndata: [DONE]\n\n", true, true, 1, 0},
		{"empty JSON", "application/json", `{"data":[],"usage":{"input_tokens":1}}`, true, true, 1, 0},
		{"JSON actual count", "application/json", `{"data":[{"b64_json":"` + imageTaskTestPNGBase64 + `"}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`, false, true, 3, 1},
		{"completed", "text/event-stream", partial + completed, false, true, 1, 1},
		{"error", "text/event-stream", partial + "event: error\ndata: {\"type\":\"error\",\"message\":\"generation failed\",\"code\":\"generation_failed\"}\n\n", true, true, 1, 0},
		{"interrupted", "text/event-stream", partial, true, true, 1, 0},
		{"empty", "text/event-stream", "", true, true, 1, 0},
		{"JSON fallback", "application/json", `{"data":[{"b64_json":"` + imageTaskTestPNGBase64 + `"}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`, false, true, 1, 1},
		{"JSON error", "application/json", `{"error":{"message":"generation failed","type":"server_error"}}`, true, true, 1, 0},
	} {
		for _, usePrice := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/price_%t", tt.name, usePrice), func(t *testing.T) {
				setupImageTaskTestDB(t)
				require.NoError(t, model.DB.AutoMigrate(&model.User{}))
				require.NoError(t, model.DB.Create(&model.User{Id: 7, Username: "image-stream-user", Quota: 50000}).Error)
				require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Type: constant.ChannelTypeOpenAI, Name: "image-stream-channel"}).Error)
				previousBatchUpdate, previousTimeout := common.BatchUpdateEnabled, constant.StreamingTimeout
				common.BatchUpdateEnabled, constant.StreamingTimeout = false, 30
				t.Cleanup(func() { common.BatchUpdateEnabled, constant.StreamingTimeout = previousBatchUpdate, previousTimeout })
				service.InitHttpClient()
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", tt.contentType)
					if tt.name == "upstream read failure" {
						// A body shorter than Content-Length produces unexpected EOF.
						w.Header().Set("Content-Length", fmt.Sprint(len(tt.body)+100))
					}
					_, _ = w.Write([]byte(tt.body))
				}))
				defer upstream.Close()
				body := []byte(fmt.Sprintf(`{"model":"gpt-image-1","prompt":"cat","n":%d,"stream":true}`, tt.requestedN))
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
				ctx.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(ctx, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
				common.SetContextKey(ctx, constant.ContextKeyChannelId, 11)
				common.SetContextKey(ctx, constant.ContextKeyChannelKey, "local-test-key")
				common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream.URL)
				defer common.CleanupBodyStorage(ctx)
				billing := &imageAsyncBillingStub{}
				info := &relaycommon.RelayInfo{
					UserId: 7, OriginModelName: "gpt-image-1", StartTime: time.Now(), IsStream: true, DisablePing: true,
					RelayMode: relayconstant.RelayModeImagesGenerations, RequestURLPath: "/v1/images/generations",
					Request:   &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", N: common.GetPointer(tt.requestedN), Stream: common.GetPointer(true)},
					Billing:   billing,
					PriceData: types.PriceData{UsePrice: usePrice, ModelPrice: 0.01, ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
				}
				apiErr := ImageHelper(ctx, info)
				require.Equal(t, tt.wantStream, info.IsStream)
				var user model.User
				require.NoError(t, model.DB.First(&user, 7).Error)
				if tt.wantError {
					require.NotNil(t, apiErr)
					require.Zero(t, billing.settleCalls)
					require.Zero(t, user.UsedQuota)
					require.Zero(t, user.RequestCount)
				} else {
					require.Nil(t, apiErr)
					require.Equal(t, 1, billing.settleCalls)
					wantQuota := 30
					if usePrice {
						wantQuota = 5000 * tt.billedN
					}
					require.Equal(t, wantQuota, user.UsedQuota)
					require.Equal(t, 1, user.RequestCount)
					if tt.contentType == "text/event-stream" {
						if tt.name == "upstream read failure" {
							require.Contains(t, recorder.Body.String(), tt.body)
							require.Contains(t, recorder.Body.String(), "event: error")
						} else {
							require.Equal(t, tt.body, recorder.Body.String())
						}
					} else {
						require.Contains(t, recorder.Body.String(), "event: image_generation.completed")
						require.Contains(t, recorder.Body.String(), imageTaskTestPNGBase64)
						require.Contains(t, recorder.Body.String(), "data: [DONE]")
					}
				}
			})
		}
	}
}
