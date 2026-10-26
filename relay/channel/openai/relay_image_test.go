package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Regression cases adapted from new-api 4013729f6a2a14b9b5988d3c1a529ea97d2b2d8d.
func TestOpenAIImageStreamMultipleImagesAndLateUsage(t *testing.T) {
	for _, mode := range []string{"image_generation", "image_edit"} {
		for _, end := range []string{"", "data: [DONE]\n\n"} {
			t.Run(mode+"/"+end, func(t *testing.T) {
				c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
				info.Request = &dto.ImageRequest{N: common.GetPointer(uint(3))}
				info.PriceData.UsePrice = true
				body := imageEvent(mode+".completed", `"b64_json":"first","usage":{"input_tokens":1,"output_tokens":2}`) +
					imageEvent(mode+".completed", `"url":"https://example.test/second.png"`) +
					"data: {\"usage\":{\"input_tokens\":10,\"output_tokens\":20}}\n\n" +
					"data: {\"message\":\"all images generated\"}\n\n" + end
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}
				usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
				require.Nil(t, apiErr)
				require.Equal(t, body, recorder.Body.String())
				require.Equal(t, 30, usage.TotalTokens)
				require.Equal(t, 2, *info.BillingImageCount)
				require.Equal(t, 2.0, info.PriceData.OtherRatios["n"])
			})
		}
	}
}

func TestOpenAIImageJSONFallbackAndCount(t *testing.T) {
	for _, tt := range []struct {
		name, data    string
		events, count int
	}{
		{"array", `[{"b64_json":"first"},{"b64_json":"second"}]`, 2, 2},
		{"object", `{"url":"https://example.test/a.png","b64_json":"first"}`, 1, 1},
		{"mixed formats", `[{"url":"https://example.test/a.png"},{"b64_json":"first"}]`, 2, 2},
		{"ignore empty entries", `[{"revised_prompt":"unused"},{"url":""},{"b64_json":"first"}]`, 1, 1},
		{"Base64 aliases", `[{"base64":"first"},{"image_base64":"second"},{"imageBase64":"third"}]`, 3, 3},
		{"object with Base64 alias", `{"imageBase64":"first"}`, 1, 1},
		{"multiple aliases on one image", `[{"b64_json":"first","base64":"first","image_base64":"first","imageBase64":"first"}]`, 1, 1},
		{"mixed formats with alias", `[{"url":"https://example.test/a.png"},{"base64":"first"}]`, 2, 2},
	} {
		for _, stream := range []bool{false, true} {
			for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
				t.Run(fmt.Sprintf("%s/stream=%t/mode=%d", tt.name, stream, mode), func(t *testing.T) {
					c, recorder, info := newOpenAIImageStreamTest(t, mode)
					info.IsStream = stream
					info.PriceData.UsePrice = true
					info.Request = &dto.ImageRequest{N: common.GetPointer(uint(3))}
					body := `{"created":1710000000,"data":` + tt.data + `,"usage":{"input_tokens":10,"output_tokens":20}}`
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
					usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
					require.Nil(t, apiErr)
					require.Equal(t, 30, usage.TotalTokens)
					require.Equal(t, tt.count, *info.BillingImageCount)
					require.Equal(t, float64(tt.count), info.PriceData.OtherRatios["n"])
					if !stream {
						require.Equal(t, body, recorder.Body.String())
						return
					}
					event := "image_generation.completed"
					if mode == relayconstant.RelayModeImagesEdits {
						event = "image_edit.completed"
					}
					require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
					require.Equal(t, tt.events, strings.Count(recorder.Body.String(), "event: "+event))
					require.Equal(t, tt.events, info.ReceivedResponseCount)
					require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: {") {
							continue
						}
						var payload struct {
							CreatedAt int64     `json:"created_at"`
							Usage     dto.Usage `json:"usage"`
						}
						require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &payload))
						require.EqualValues(t, 1710000000, payload.CreatedAt)
						require.Equal(t, 30, payload.Usage.TotalTokens)
					}
				})
			}
		}
	}
}

func TestOpenAIImageStreamNormalizesBase64Aliases(t *testing.T) {
	for _, tt := range []struct{ name, fields, want string }{
		{"base64", `"base64":"first"`, "first"},
		{"image_base64", `"image_base64":"second"`, "second"},
		{"imageBase64", `"imageBase64":"third"`, "third"},
		{"canonical precedence", `"b64_json":"first","base64":"second","image_base64":"third","imageBase64":"fourth"`, "first"},
		{"empty canonical", `"b64_json":"","base64":"second"`, "second"},
		{"blank canonical", `"b64_json":"  ","base64":" second ","image_base64":"third"`, "second"},
		{"empty alias", `"base64":"","image_base64":"third","imageBase64":"fourth"`, "third"},
	} {
		for _, sse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/sse=%t", tt.name, sse), func(t *testing.T) {
				c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
				fields := tt.fields + `,"revised_prompt":"cat","usage":{"input_tokens":10,"output_tokens":20}`
				body, contentType := `{"data":[{`+fields+`}],"usage":{"input_tokens":10,"output_tokens":20}}`, "application/json"
				if sse {
					body, contentType = imageEvent("image_generation.completed", fields)+"data: [DONE]\n\n", "text/event-stream"
				}
				resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
				usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
				require.Nil(t, apiErr)
				require.Equal(t, 30, usage.TotalTokens)
				require.Equal(t, 1, *info.BillingImageCount)
				require.Contains(t, recorder.Body.String(), "event: image_generation.completed")
				completed := 0
				for _, line := range strings.Split(recorder.Body.String(), "\n") {
					if !strings.HasPrefix(line, "data: {") {
						continue
					}
					var event map[string]any
					require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
					require.Equal(t, "image_generation.completed", event["type"])
					require.Equal(t, tt.want, event["b64_json"])
					require.Equal(t, "cat", event["revised_prompt"])
					for _, alias := range []string{"base64", "image_base64", "imageBase64"} {
						require.NotContains(t, event, alias)
					}
					completed++
				}
				require.Equal(t, 1, completed)
				require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
			})
		}
	}
}

func TestOpenAIImageUsageNormalizationAcrossFormats(t *testing.T) {
	const usageJSON = `{"input_tokens":1000,"prompt_tokens":1000,"output_tokens":100,"completion_tokens":100,"input_tokens_details":{"text_tokens":400,"image_tokens":600,"cached_tokens":300,"cache_write_tokens":20},"output_tokens_details":{"text_tokens":10,"image_tokens":90}}`
	for _, format := range []string{"JSON", "SSE", "JSON as SSE"} {
		t.Run(format, func(t *testing.T) {
			c, _, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
			info.IsStream = format != "JSON"
			body, contentType := `{"data":[{"b64_json":"image"}],"usage":`+usageJSON+`}`, "application/json"
			if format == "SSE" {
				body, contentType = imageEvent("image_generation.completed", `"b64_json":"image","usage":`+usageJSON), "text/event-stream"
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {contentType}}, Body: io.NopCloser(strings.NewReader(body))}
			usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
			require.Nil(t, apiErr)
			require.Equal(t, 1000, usage.PromptTokens)
			require.Equal(t, 100, usage.CompletionTokens)
			require.Equal(t, 1100, usage.TotalTokens)
			require.Equal(t, 300, usage.PromptTokensDetails.CachedTokens)
			require.Equal(t, 20, usage.PromptTokensDetails.CacheWriteTokens)
			require.Equal(t, 600, usage.PromptTokensDetails.ImageTokens)
			require.Equal(t, 90, usage.CompletionTokenDetails.ImageTokens)
			require.Equal(t, 10, usage.CompletionTokenDetails.TextTokens)
			require.Empty(t, info.PriceData.OtherRatios, "aggregated tokens must not get an image count multiplier")
		})
	}
}

func TestOpenAIImageJSONStreamRejectsMissingImages(t *testing.T) {
	for _, body := range []string{`{}`, `{"data":[]}`, `{"data":[{"revised_prompt":"cat"}],"usage":{"input_tokens":1}}`, `{"data":{"b64_json":""}}`} {
		c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
		resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
		usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
		require.Nil(t, usage)
		require.NotNil(t, apiErr)
		require.False(t, c.Writer.Written())
		require.Empty(t, recorder.Body.String())
	}
}

func TestOpenAIImageJSONFallbackPreservesMetadata(t *testing.T) {
	const metadata = `"size":"1536x1024","quality":"low","output_format":"png","background":"opaque"`
	for _, tt := range []struct {
		name, image, wantSize, wantQuality, wantFormat, wantBackground string
	}{
		{"top-level metadata", `{"b64_json":"image"}`, "1536x1024", "low", "png", "opaque"},
		{"per-image override", `{"b64_json":"image","size":"1024x1024","quality":"high","output_format":"webp","background":"transparent"}`, "1024x1024", "high", "webp", "transparent"},
		{"empty per-image fields", `{"b64_json":"image","size":"","quality":null,"output_format":" ","background":""}`, "1536x1024", "low", "png", "opaque"},
	} {
		for _, objectData := range []bool{false, true} {
			for _, mode := range []int{relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits} {
				t.Run(fmt.Sprintf("%s/object=%t/mode=%d", tt.name, objectData, mode), func(t *testing.T) {
					c, recorder, info := newOpenAIImageStreamTest(t, mode)
					data := tt.image
					if !objectData {
						data = "[" + data + "]"
					}
					body := `{"created":1710000000,"data":` + data + `,` + metadata + `}`
					resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}
					_, apiErr := OpenaiHandlerWithUsage(c, info, resp)
					require.Nil(t, apiErr)
					completed := 0
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if !strings.HasPrefix(line, "data: {") {
							continue
						}
						var event map[string]any
						require.NoError(t, common.UnmarshalJsonStr(strings.TrimPrefix(line, "data: "), &event))
						require.Equal(t, tt.wantSize, event["size"])
						require.Equal(t, tt.wantQuality, event["quality"])
						require.Equal(t, tt.wantFormat, event["output_format"])
						require.Equal(t, tt.wantBackground, event["background"])
						require.Equal(t, "image", event["b64_json"])
						completed++
					}
					require.Equal(t, 1, completed)
					require.True(t, strings.HasSuffix(recorder.Body.String(), "data: [DONE]\n\n"))
				})
			}
		}
	}
}

type imageCancelWriter struct {
	gin.ResponseWriter
	cancel context.CancelFunc
	needle string
	fail   bool
}

func (w *imageCancelWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), w.needle) {
		w.cancel()
		if w.fail {
			return 0, io.ErrClosedPipe
		}
	}
	return w.ResponseWriter.Write(p)
}

func (w *imageCancelWriter) WriteString(s string) (int, error) {
	if strings.Contains(s, w.needle) {
		w.cancel()
		if w.fail {
			return 0, io.ErrClosedPipe
		}
	}
	return io.WriteString(w.ResponseWriter, s)
}

func TestOpenAIImageStreamClientAbortProtectsCount(t *testing.T) {
	for _, tt := range []struct {
		requested, completed, want int
		fail                       bool
	}{
		{3, 1, 3, false}, {1, 2, 2, false}, {3, 1, 3, true},
	} {
		t.Run(fmt.Sprint(tt), func(t *testing.T) {
			c, _, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
			info.Request = &dto.ImageRequest{N: common.GetPointer(uint(tt.requested))}
			info.PriceData.UsePrice = true
			ctx, cancel := context.WithCancel(c.Request.Context())
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			c.Writer = &imageCancelWriter{ResponseWriter: c.Writer, cancel: cancel, needle: fmt.Sprintf("image-%d", tt.completed), fail: tt.fail}
			reader, writer := io.Pipe()
			defer reader.Close()
			defer writer.Close()
			go func() {
				for n := 1; n <= tt.completed; n++ {
					_, _ = io.WriteString(writer, imageEvent("image_generation.completed", fmt.Sprintf(`"b64_json":"image-%d"`, n)))
				}
			}()
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: reader}
			usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Contains(t, []relaycommon.StreamEndReason{relaycommon.StreamEndReasonClientGone, relaycommon.StreamEndReasonHandlerStop}, info.StreamStatus.EndReason)
			require.Equal(t, tt.want, *info.BillingImageCount)
			require.Equal(t, float64(tt.want), info.PriceData.OtherRatios["n"])
		})
	}
}

func TestOpenAIImageStreamUpstreamFailureKeepsCompletedCount(t *testing.T) {
	for _, failure := range []string{"read error", "timeout", "invalid event"} {
		t.Run(failure, func(t *testing.T) {
			c, recorder, info := newOpenAIImageStreamTest(t, relayconstant.RelayModeImagesGenerations)
			info.Request = &dto.ImageRequest{N: common.GetPointer(uint(3))}
			info.PriceData.UsePrice = true
			completed := imageEvent("image_generation.completed", `"b64_json":"first","usage":{"input_tokens":10,"output_tokens":20}`)
			var body io.ReadCloser
			switch failure {
			case "read error":
				body = io.NopCloser(io.MultiReader(strings.NewReader(completed), iotest.ErrReader(io.ErrUnexpectedEOF)))
			case "timeout":
				constant.StreamingTimeout = 1
				reader, writer := io.Pipe()
				t.Cleanup(func() { _ = writer.Close() })
				go func() { _, _ = io.WriteString(writer, completed) }()
				body = reader
			case "invalid event":
				body = io.NopCloser(strings.NewReader(completed + "data: invalid-json\n\n"))
			}
			resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
			usage, apiErr := OpenaiHandlerWithUsage(c, info, resp)
			require.Nil(t, apiErr, "the completed image must still settle")
			require.NoError(t, c.Request.Context().Err(), "the client has not disconnected")
			require.Equal(t, 1, *info.BillingImageCount)
			require.Equal(t, 1.0, info.PriceData.OtherRatios["n"])
			require.Equal(t, 30, usage.TotalTokens)
			require.Contains(t, recorder.Body.String(), completed)
			require.Contains(t, recorder.Body.String(), "event: error")
		})
	}
}
