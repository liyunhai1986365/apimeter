package relay

import (
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
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageMixedFormatsSettleEachImage(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, usePrice := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/price=%t", stream, usePrice), func(t *testing.T) {
				t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
				t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.test")
				t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"data":[{"url":"https://upstream.example.test/first.png"},{"b64_json":"`+imageTaskTestPNGBase64+`"}],"usage":{"input_tokens":10,"output_tokens":20}}`)
				}))
				defer upstream.Close()
				request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "two images", N: common.GetPointer(uint(2)), Stream: common.GetPointer(stream), ResponseFormat: "url"}
				ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
				info.PriceData.UsePrice, info.PriceData.ModelPrice = usePrice, 0.01
				require.Nil(t, ImageHelper(ctx, info))
				if stream {
					require.Equal(t, 2, strings.Count(recorder.Body.String(), "event: image_generation.completed"))
				} else {
					require.Len(t, gjson.Get(recorder.Body.String(), "data").Array(), 2)
				}
				require.Equal(t, 2, *info.BillingImageCount)
				var user model.User
				require.NoError(t, model.DB.First(&user, 7).Error)
				wantQuota := 30
				if usePrice {
					wantQuota = 10000
				}
				require.Equal(t, wantQuota, user.UsedQuota)
			})
		}
	}
}

func TestImageCompletedObjectAppliesPolicyWithoutPolling(t *testing.T) {
	for _, mode := range []struct {
		name          string
		stream, async bool
	}{{"JSON", false, false}, {"JSON as SSE", true, false}, {"async and stream", true, true}} {
		for _, withID := range []bool{false, true} {
			for _, format := range []string{"url", "b64_json"} {
				for _, field := range []string{"b64_json", "imageBase64"} {
					t.Run(fmt.Sprintf("%s/id=%t/%s/%s", mode.name, withID, format, field), func(t *testing.T) {
						dir := t.TempDir()
						t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
						t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.test")
						t.Setenv("API_TEMP_IMAGE_DIR", dir)
						var polls atomic.Int32
						upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							if r.Method == http.MethodGet {
								polls.Add(1)
								w.WriteHeader(http.StatusNotFound)
								_, _ = fmt.Fprint(w, `{"error":"completed image has no task"}`)
								return
							}
							id := ""
							if withID {
								id = `"id":"completed-image",`
							}
							_, _ = fmt.Fprintf(w, `{%s"data":{"%s":"%s","revised_prompt":"cat"},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`, id, field, imageTaskTestPNGBase64)
						}))
						defer upstream.Close()
						request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", N: common.GetPointer(uint(3)), Stream: common.GetPointer(mode.stream)}
						ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
						info.IsAsyncImageRequest = mode.async
						info.TokenImageSettings = model.TokenImageSettings{Format: format, Store: dto.TokenImageStoreForceStoreURLAndBase64}
						info.PriceData.UsePrice, info.PriceData.ModelPrice = true, 0.01
						require.Nil(t, ImageHelper(ctx, info))
						require.Zero(t, polls.Load())
						require.Equal(t, 1, *info.BillingImageCount)
						files, err := os.ReadDir(dir)
						require.NoError(t, err)
						require.Len(t, files, 1)
						if format == "url" {
							require.Contains(t, recorder.Body.String(), "https://cdn.example.test/api/relay-temp-images/")
							require.NotContains(t, recorder.Body.String(), imageTaskTestPNGBase64)
						} else {
							require.Contains(t, recorder.Body.String(), imageTaskTestPNGBase64)
						}
						require.Contains(t, recorder.Body.String(), `"revised_prompt":"cat"`)
						if !mode.stream || mode.async {
							require.True(t, gjson.Get(recorder.Body.String(), "data").IsArray())
							// The local async worker and chat wrappers must also be able
							// to consume the canonical response, including alias images.
							taskBody, err := localImageTaskResponseFromOpenAIImageBody("local-task", "", time.Now(), recorder.Body.Bytes())
							require.NoError(t, err)
							require.Len(t, gjson.GetBytes(taskBody, "data.images").Array(), 1)
						} else {
							require.Contains(t, recorder.Body.String(), "event: image_generation.completed")
							require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
						}
						var user model.User
						require.NoError(t, model.DB.First(&user, 7).Error)
						require.Equal(t, 5000, user.UsedQuota)
						var taskCount int64
						require.NoError(t, model.DB.Model(&model.Task{}).Count(&taskCount).Error)
						require.Zero(t, taskCount, "completed images must not create upstream polling tasks")
					})
				}
			}
		}
	}
}

func TestImageSplitResultsApplyStorageOnce(t *testing.T) {
	for _, mode := range []string{"sync", "tasks", "mixed", "partial failure"} {
		for _, format := range []string{"url", "b64_json"} {
			for _, usePrice := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/price=%t", mode, format, usePrice), func(t *testing.T) {
					dir := t.TempDir()
					t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
					t.Setenv("API_TEMP_IMAGE_DIR", dir)
					var submits, polls, storageReads atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if strings.HasPrefix(r.URL.Path, "/api/relay-temp-images/") {
							storageReads.Add(1)
							w.WriteHeader(http.StatusNotFound)
							return
						}
						if r.Method == http.MethodPost {
							var request dto.ImageRequest
							require.NoError(t, common.DecodeJson(r.Body, &request))
							if request.N != nil && *request.N > 1 {
								w.WriteHeader(http.StatusBadRequest)
								_, _ = fmt.Fprint(w, `{"error":{"message":"n must be 1","type":"invalid_value","param":"n"}}`)
								return
							}
							id := submits.Add(1)
							if mode == "partial failure" && id == 3 {
								w.WriteHeader(http.StatusInternalServerError)
								_, _ = fmt.Fprint(w, `{"error":{"message":"generation failed"}}`)
								return
							}
							if mode == "sync" || (mode != "tasks" && id == 2) {
								_, _ = fmt.Fprint(w, `{"id":"completed-image","data":{"b64_json":"`+imageTaskTestPNGBase64+`"},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
								return
							}
							_, _ = fmt.Fprintf(w, `{"id":"split-task-%d","usage":{"input_tokens":1}}`, id)
							return
						}
						polls.Add(1)
						_, _ = fmt.Fprint(w, `{"state":"succeeded","data":{"images":[{"b64_json":"`+imageTaskTestPNGBase64+`"}]},"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
					}))
					defer upstream.Close()
					t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", upstream.URL)
					request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", N: common.GetPointer(uint(3))}
					ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
					info.TokenImageSettings = model.TokenImageSettings{Format: format, Store: dto.TokenImageStoreForceStoreURLAndBase64}
					info.PriceData.UsePrice, info.PriceData.ModelPrice = usePrice, 0.01
					require.Nil(t, ImageHelper(ctx, info))
					require.EqualValues(t, 3, submits.Load())
					wantPolls := map[string]int{"sync": 0, "tasks": 3, "mixed": 2, "partial failure": 1}[mode]
					require.EqualValues(t, wantPolls, polls.Load())
					require.Zero(t, storageReads.Load(), "already stored URLs must not be downloaded again")
					wantImages := 3
					if mode == "partial failure" {
						wantImages = 2
					}
					files, err := os.ReadDir(dir)
					require.NoError(t, err)
					require.Len(t, files, wantImages)
					require.Len(t, gjson.Get(recorder.Body.String(), "data").Array(), wantImages)
					require.EqualValues(t, wantImages*30, gjson.Get(recorder.Body.String(), "usage.total_tokens").Int())
					if format == "url" {
						require.NotContains(t, recorder.Body.String(), imageTaskTestPNGBase64)
					} else {
						require.Contains(t, recorder.Body.String(), imageTaskTestPNGBase64)
					}
					var user model.User
					require.NoError(t, model.DB.First(&user, 7).Error)
					wantQuota := wantImages * 30
					if usePrice {
						wantQuota = wantImages * 5000
					}
					require.Equal(t, wantQuota, user.UsedQuota)
					require.Equal(t, 1, info.Billing.(*imageAsyncBillingStub).settleCalls)
				})
			}
		}
	}
}
