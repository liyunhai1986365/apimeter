package relay

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newImageResponsePolicyTestContext(t *testing.T, channelType int, upstream string, request *dto.ImageRequest) (*gin.Context, *httptest.ResponseRecorder, *relaycommon.RelayInfo) {
	t.Helper()
	setupImageTaskTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	require.NoError(t, model.DB.Create(&model.User{Id: 7, Username: "image-policy-test", Quota: 900000}).Error)
	require.NoError(t, model.DB.Create(&model.Channel{Id: 11, Type: channelType, Name: "image-policy-test"}).Error)
	previousBatch, previousTimeout := common.BatchUpdateEnabled, constant.StreamingTimeout
	common.BatchUpdateEnabled, constant.StreamingTimeout = false, 30
	t.Cleanup(func() { common.BatchUpdateEnabled, constant.StreamingTimeout = previousBatch, previousTimeout })
	service.InitHttpClient()
	body, err := common.Marshal(request)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyChannelType, channelType)
	common.SetContextKey(ctx, constant.ContextKeyChannelId, 11)
	common.SetContextKey(ctx, constant.ContextKeyChannelKey, "local-test-key")
	common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, upstream)
	t.Cleanup(func() { common.CleanupBodyStorage(ctx) })
	return ctx, recorder, &relaycommon.RelayInfo{
		UserId: 7, OriginModelName: request.Model, StartTime: time.Now(), IsStream: request.IsStream(nil), DisablePing: true,
		RelayMode: relayconstant.RelayModeImagesGenerations, RequestURLPath: "/v1/images/generations",
		Request: request, Billing: &imageAsyncBillingStub{},
		PriceData: types.PriceData{UsePrice: false, ModelRatio: 1, CompletionRatio: 1, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
}

func TestGeminiImageRequestWithStreamUsesJSONUpstream(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload := `{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"` + imageTaskTestPNGBase64 + `"}}]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":20,"totalTokenCount":30}}`
				if strings.Contains(r.URL.Path, "streamGenerateContent") {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = fmt.Fprint(w, "data: "+payload+"\n\n")
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, payload)
				}
			}))
			defer upstream.Close()
			request := &dto.ImageRequest{Model: "gemini-3-pro-image-preview", Prompt: "cat", Stream: common.GetPointer(stream)}
			ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeGemini, upstream.URL, request)
			require.Nil(t, ImageHelper(ctx, info), "valid upstream image must not be rejected")
			require.Contains(t, recorder.Body.String(), imageTaskTestPNGBase64)
		})
	}
}

// The same upstream result must keep its final pricing and storage behavior
// across JSON, a JSON fallback for a stream request, and native SSE.
var imageResponsePolicyModes = []struct {
	name        string
	stream, sse bool
}{
	{"JSON", false, false},
	{"JSON as SSE", true, false},
	{"SSE", true, true},
}

func imagePolicyTestUpstream(w http.ResponseWriter, sse bool, image, suffix string) {
	const usage = `{"input_tokens":10,"output_tokens":20,"total_tokens":30}`
	if sse {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: image_generation.partial_image\ndata: {\"type\":\"image_generation.partial_image\",\"b64_json\":\"preview\"}\n\n")
		_, _ = fmt.Fprintf(w, "event: image_generation.completed\ndata: {\"type\":\"image_generation.completed\",%s}\n\n", strings.TrimSuffix(strings.TrimPrefix(image, "{"), "}"))
		_, _ = fmt.Fprint(w, "data: {\"usage\":"+usage+"}\n\n"+suffix)
	} else {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[`+image+`],"usage":`+usage+`}`)
	}
}

func TestImageStreamUsesResponseBillingMetadata(t *testing.T) {
	for _, mode := range imageResponsePolicyModes {
		t.Run(mode.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				imagePolicyTestUpstream(w, mode.sse, `{"size":"4096x4096","b64_json":"`+imageTaskTestPNGBase64+`"}`, "data: [DONE]\n\n")
			}))
			defer upstream.Close()
			request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Size: "1024x1024", Stream: common.GetPointer(mode.stream)}
			ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
			info.BillingRequestInput = &billingexpr.RequestInput{Body: []byte(`{"size":"1024x1024"}`)}
			info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{
				BillingMode: "tiered_expr", ModelName: request.Model, GroupRatio: 1, QuotaPerUnit: 500000, ExprVersion: 1,
				ExprString: `pixels(response("data.0.size") != nil ? response("data.0.size") : param("size")) > 2360000 ? tier("large", 600000.0) : tier("small", 300000.0)`,
			}
			require.Nil(t, ImageHelper(ctx, info))
			var user model.User
			require.NoError(t, model.DB.First(&user, 7).Error)
			require.Equal(t, 300000, user.UsedQuota, "stream flag must not change the response-size pricing tier")
			require.Equal(t, "4096x4096", gjson.GetBytes(info.BillingRequestInput.ResponseBody, "data.0.size").String())
			require.EqualValues(t, 1, gjson.GetBytes(info.BillingRequestInput.ResponseBody, "data.#").Int(), "previews must not enter data[]")
			require.EqualValues(t, 30, gjson.GetBytes(info.BillingRequestInput.ResponseBody, "usage.total_tokens").Int())
			require.JSONEq(t, `{"size":"1024x1024"}`, string(info.BillingRequestInput.Body))
			require.Contains(t, recorder.Body.String(), `"size":"4096x4096"`)
			if mode.sse {
				require.Equal(t, "4096x4096", gjson.GetBytes(info.BillingRequestInput.ResponseBody, "size").String())
			}
		})
	}
}

func TestImageStreamAppliesStoragePolicy(t *testing.T) {
	for _, mode := range imageResponsePolicyModes {
		for _, format := range []string{"url", "b64_json"} {
			for _, field := range []string{"b64_json", "base64", "image_base64", "imageBase64"} {
				t.Run(mode.name+"/"+format+"/"+field, func(t *testing.T) {
					imageDir := t.TempDir()
					t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
					t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://test-cdn.example.test")
					t.Setenv("API_TEMP_IMAGE_DIR", imageDir)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						imagePolicyTestUpstream(w, mode.sse, `{"`+field+`":"`+imageTaskTestPNGBase64+`"}`, "data: [DONE]\n\n")
					}))
					defer upstream.Close()
					request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Stream: common.GetPointer(mode.stream)}
					ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
					info.TokenImageSettings = model.TokenImageSettings{Format: format, Store: dto.TokenImageStoreForceStoreURLAndBase64}
					require.Nil(t, ImageHelper(ctx, info))
					files, err := os.ReadDir(imageDir)
					require.NoError(t, err)
					require.Len(t, files, 1, "store the completed image, not the preview")
					if format == "url" {
						require.Contains(t, recorder.Body.String(), "https://test-cdn.example.test/api/relay-temp-images/")
						require.NotContains(t, recorder.Body.String(), imageTaskTestPNGBase64)
					} else {
						require.Contains(t, recorder.Body.String(), `"b64_json":"`+imageTaskTestPNGBase64+`"`)
						require.NotContains(t, recorder.Body.String(), "https://test-cdn.example.test")
					}
					if mode.sse {
						require.Contains(t, recorder.Body.String(), `"b64_json":"preview"`, "preview frames remain unchanged")
					}
				})
			}
		}
	}
}

func TestImageStreamStorageFailureDoesNotSettle(t *testing.T) {
	for _, mode := range imageResponsePolicyModes[1:] {
		t.Run(mode.name, func(t *testing.T) {
			// Use a file as the parent directory so failure is deterministic
			// even when tests run as root.
			parent := filepath.Join(t.TempDir(), "file")
			require.NoError(t, os.WriteFile(parent, []byte("file"), 0600))
			t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
			t.Setenv("API_TEMP_IMAGE_DIR", filepath.Join(parent, "images"))
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				imagePolicyTestUpstream(w, mode.sse, `{"b64_json":"`+imageTaskTestPNGBase64+`"}`, "data: [DONE]\n\n")
			}))
			defer upstream.Close()
			request := &dto.ImageRequest{Model: "gpt-image-1", Prompt: "cat", Stream: common.GetPointer(true)}
			ctx, recorder, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
			info.TokenImageSettings = model.TokenImageSettings{Format: "url", Store: dto.TokenImageStoreForceStoreURLAndBase64}
			require.NotNil(t, ImageHelper(ctx, info))
			require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls)
			require.NotContains(t, recorder.Body.String(), "event: image_generation.completed")
			if mode.sse {
				require.Contains(t, recorder.Body.String(), "event: error")
			} else {
				require.False(t, ctx.Writer.Written(), "JSON fallback must finish storage before starting SSE")
			}
		})
	}
}

func TestGoogleImageRoutesDoNotAffectChatStreaming(t *testing.T) {
	for _, apiType := range []int{constant.APITypeGemini, constant.APITypeVertexAi} {
		for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits, relayconstant.RelayModeChatCompletions, relayconstant.RelayModeGemini} {
			t.Run(fmt.Sprintf("api=%d/mode=%d", apiType, mode), func(t *testing.T) {
				info := &relaycommon.RelayInfo{
					IsStream: true, RelayMode: mode, OriginModelName: "gemini-3-pro-image-preview",
					ChannelMeta: &relaycommon.ChannelMeta{
						UpstreamModelName: "gemini-3-pro-image-preview", ChannelBaseUrl: "https://upstream.example.test", ApiKey: "test-key",
						ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey},
					},
				}
				adaptor := GetAdaptor(apiType)
				adaptor.Init(info)
				upstreamURL, err := adaptor.GetRequestURL(info)
				require.NoError(t, err)
				if mode == relayconstant.RelayModeImagesGenerations || mode == relayconstant.RelayModeImagesEdits {
					require.Contains(t, upstreamURL, ":generateContent")
					require.NotContains(t, upstreamURL, "streamGenerateContent")
				} else {
					require.Contains(t, upstreamURL, ":streamGenerateContent?alt=sse")
				}
			})
		}
	}
}
