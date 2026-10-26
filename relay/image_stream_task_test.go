package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageWaitsForTaskAndSettlesFinalResult(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, channelType := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeXai} {
			for _, format := range []string{"url", "b64_json"} {
				for _, usePrice := range []bool{false, true} {
					t.Run(fmt.Sprintf("stream=%t/channel=%d/%s/price=%t", stream, channelType, format, usePrice), func(t *testing.T) {
						imageDir := t.TempDir()
						t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
						t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://test-cdn.example.test")
						t.Setenv("API_TEMP_IMAGE_DIR", imageDir)
						var submits, polls atomic.Int32
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							if r.Method == http.MethodPost {
								submits.Add(1)
								_, _ = fmt.Fprint(w, `{"id":"stream-task","usage":{"input_tokens":1}}`)
								return
							}
							polls.Add(1)
							require.Equal(t, "/v1/tasks/stream-task", r.URL.Path)
							_, _ = fmt.Fprint(w, `{"id":"stream-task","state":"succeeded","data":{"images":[{"b64_json":"`+imageTaskTestPNGBase64+`"}]},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
						}))
						defer upstream.Close()
						request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", N: common.GetPointer(uint(3)), Stream: common.GetPointer(stream)}
						ctx, recorder, info := newImageResponsePolicyTestContext(t, channelType, upstream.URL, request)
						info.TokenImageSettings = model.TokenImageSettings{Format: format, Store: dto.TokenImageStoreForceStoreURLAndBase64}
						info.PriceData.UsePrice, info.PriceData.ModelPrice = usePrice, 0.01
						require.Nil(t, ImageHelper(ctx, info))
						require.EqualValues(t, 1, submits.Load(), "poll the accepted task instead of resubmitting generation")
						require.EqualValues(t, 1, polls.Load())
						if stream {
							require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
							require.Equal(t, 1, strings.Count(recorder.Body.String(), "event: image_generation.completed"))
							require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
						} else {
							require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
							require.Len(t, gjson.Get(recorder.Body.String(), "data").Array(), 1)
						}
						require.Contains(t, recorder.Body.String(), `"total_tokens":30`)
						require.Equal(t, 1, *info.BillingImageCount)
						require.Equal(t, 1, info.Billing.(*imageAsyncBillingStub).settleCalls)
						var user model.User
						require.NoError(t, model.DB.First(&user, 7).Error)
						wantQuota := 30
						if usePrice {
							wantQuota = 5000
						}
						require.Equal(t, wantQuota, user.UsedQuota, "bill final usage or actual image count, not submission usage/requested n")
						files, err := os.ReadDir(imageDir)
						require.NoError(t, err)
						require.Len(t, files, 1, "task conversion must store the image only once")
						if format == "url" {
							require.Contains(t, recorder.Body.String(), "https://test-cdn.example.test/api/relay-temp-images/")
							require.NotContains(t, recorder.Body.String(), imageTaskTestPNGBase64)
						} else {
							require.Contains(t, recorder.Body.String(), imageTaskTestPNGBase64)
						}
						var taskCount int64
						require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
						require.Zero(t, taskCount, "synchronously resolved tasks must not be settled again by a task worker")
						require.EqualValues(t, 30, gjson.GetBytes(info.BillingRequestInput.ResponseBody, "usage.total_tokens").Int())
					})
				}
			}
		}
	}
}

func TestImageStreamTaskFailuresDoNotSettle(t *testing.T) {
	for _, tt := range []struct {
		name, body, wantError string
		cancel                bool
	}{
		{"failed", `{"state":"failed","error":{"message":"generation failed"}}`, "generation failed", false},
		{"timeout", `{"state":"running"}`, "image async task wait timeout", false},
		{"canceled", `{"state":"running"}`, "context canceled", true},
		{"empty result", `{"state":"succeeded","data":{"images":[]}}`, "image task succeeded without images", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requestContext, cancel := context.WithCancel(context.Background())
			defer cancel()
			var polls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					_, _ = fmt.Fprint(w, `{"id":"stream-task","usage":{"input_tokens":999}}`)
					return
				}
				polls.Add(1)
				if tt.cancel {
					cancel()
				}
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer upstream.Close()
			request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Stream: common.GetPointer(true)}
			ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
			ctx.Request = ctx.Request.WithContext(requestContext)
			waitSeconds := 2
			if tt.cancel {
				waitSeconds = 10
			}
			common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{ImageAsyncWaitTimeoutSeconds: &waitSeconds},
			})
			apiErr := ImageHelper(ctx, info)
			require.NotNil(t, apiErr)
			require.Contains(t, apiErr.Error(), tt.wantError)
			require.True(t, types.IsSkipRetryError(apiErr), "an accepted image task must not be generated twice")
			require.EqualValues(t, 1, polls.Load())
			require.False(t, ctx.Writer.Written(), "polling must finish before starting SSE")
			require.Empty(t, recorder.Body.String())
			require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls)
			var user model.User
			require.NoError(t, model.DB.First(&user, 7).Error)
			require.Zero(t, user.UsedQuota)
		})
	}
}

func TestImageTaskWaitSkipsCompletedResponseWithID(t *testing.T) {
	body := []byte(`{"id":"completed-image","data":[{"b64_json":"` + imageTaskTestPNGBase64 + `"}]}`)
	resolved, apiErr := waitImageAsyncSubmitResponse(nil, &relaycommon.RelayInfo{}, body)
	require.Nil(t, apiErr)
	require.Nil(t, resolved, "an image ID must not trigger task polling")
}

func TestImageTaskWaitInterruptsInflightFetch(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, readBody := range []bool{false, true} {
			for _, timeout := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/read_body=%t/timeout=%t", stream, readBody, timeout), func(t *testing.T) {
					started, upstreamCanceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if r.Method == http.MethodPost {
							_, _ = fmt.Fprint(w, `{"id":"blocked-task"}`)
							return
						}
						if readBody {
							_, _ = fmt.Fprint(w, `{"state":`)
							w.(http.Flusher).Flush()
						}
						close(started)
						select {
						case <-r.Context().Done():
							close(upstreamCanceled)
						case <-release:
						}
					}))
					defer upstream.Close()
					defer close(release)
					request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Stream: common.GetPointer(stream)}
					ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
					requestContext, cancel := context.WithCancel(ctx.Request.Context())
					defer cancel()
					ctx.Request = ctx.Request.WithContext(requestContext)
					waitSeconds := 30
					if timeout {
						waitSeconds = 1
					}
					common.SetContextKey(ctx, constant.ContextKeyChannelSetting, dto.ChannelSettings{
						Protocol: &dto.ChannelProtocolSettings{ImageAsyncWaitTimeoutSeconds: &waitSeconds},
					})
					done := make(chan *types.NewAPIError, 1)
					go func() { done <- ImageHelper(ctx, info) }()
					select {
					case <-started:
					case <-time.After(2 * time.Second):
						t.Fatal("task fetch did not start")
					}
					if !timeout {
						cancel()
					}
					select {
					case apiErr := <-done:
						require.NotNil(t, apiErr)
						if timeout {
							require.Contains(t, apiErr.Error(), "image async task wait timeout")
						} else {
							require.Contains(t, apiErr.Error(), "context canceled")
						}
						require.True(t, types.IsSkipRetryError(apiErr))
					case <-time.After(3 * time.Second):
						t.Fatal("cancel/deadline did not interrupt the in-flight task query")
					}
					select {
					case <-upstreamCanceled:
					case <-time.After(time.Second):
						t.Fatal("upstream connection was not canceled")
					}
					require.False(t, ctx.Writer.Written())
					require.Empty(t, recorder.Body.String())
					require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls)
					var user model.User
					require.NoError(t, model.DB.First(&user, 7).Error)
					require.Zero(t, user.UsedQuota)
				})
			}
		}
	}
}

func TestImageSplitTasksSettleFinalUsage(t *testing.T) {
	var submits, polls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			var request dto.ImageRequest
			require.NoError(t, common.DecodeJson(r.Body, &request))
			if request.N != nil && *request.N > 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"error":{"message":"n must be 1","type":"invalid_value","param":"n"}}`)
				return
			}
			id := submits.Add(1)
			_, _ = fmt.Fprintf(w, `{"id":"split-task-%d","usage":{"input_tokens":1}}`, id)
			return
		}
		polls.Add(1)
		_, _ = fmt.Fprint(w, `{"state":"succeeded","data":{"images":[{"b64_json":"`+imageTaskTestPNGBase64+`"}]},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
	}))
	defer upstream.Close()
	request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", N: common.GetPointer(uint(3))}
	ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
	require.Nil(t, ImageHelper(ctx, info))
	require.EqualValues(t, 3, submits.Load())
	require.EqualValues(t, 3, polls.Load())
	require.Len(t, gjson.Get(recorder.Body.String(), "data").Array(), 3)
	require.EqualValues(t, 90, gjson.Get(recorder.Body.String(), "usage.total_tokens").Int())
	var user model.User
	require.NoError(t, model.DB.First(&user, 7).Error)
	require.Equal(t, 90, user.UsedQuota)
	require.Equal(t, 1, info.Billing.(*imageAsyncBillingStub).settleCalls)
}
