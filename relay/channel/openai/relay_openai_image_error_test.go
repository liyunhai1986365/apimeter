package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenaiImageResponseRejectsErrorBeforeWritingUsage(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{"standard error", `{"error":{"message":"image generation failed","type":"server_error","code":"generation_failed"}}`},
		{"message without type", `{"error":{"message":"image generation failed"}}`},
		{"string error", `{"error":"image generation failed"}`},
		{"error with usage", `{"error":{"message":"image generation failed","type":"server_error"},"usage":{"input_tokens":1,"total_tokens":1}}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tt.body)), Header: make(http.Header)}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}
			usage, apiErr := OpenaiHandlerWithUsage(ctx, info, resp)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
			require.Contains(t, apiErr.Error(), "image generation failed")
			require.Nil(t, usage, "an upstream error must not return billable usage")
			require.False(t, ctx.Writer.Written(), "the error must be detected before copying a success response")
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestOpenaiImageResponseAllowsEmptyErrors(t *testing.T) {
	for _, errorJSON := range []string{`null`, `""`, `{}`, `false`} {
		for _, stream := range []bool{false, true} {
			t.Run(errorJSON+fmt.Sprint(stream), func(t *testing.T) {
				body := `{"data":[{"url":"https://example.test/image.png"}],"error":` + errorJSON + `,"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`
				recorder := httptest.NewRecorder()
				ctx, _ := gin.CreateTestContext(recorder)
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				resp := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{"Content-Type": {"application/json"}}}
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}, IsStream: stream}
				usage, apiErr := OpenaiHandlerWithUsage(ctx, info, resp)
				require.Nil(t, apiErr)
				require.Equal(t, 10, usage.PromptTokens)
				require.Equal(t, 20, usage.CompletionTokens)
				if stream {
					require.Contains(t, recorder.Body.String(), "event: image_generation.completed")
				} else {
					require.Equal(t, body, recorder.Body.String())
				}
			})
		}
	}
}
