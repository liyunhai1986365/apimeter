package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInterruptedStreamNeverRetriesOrFallsBack(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	err := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("incomplete function arguments"), types.ErrorCodeBadResponse, 502), &dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120})
	err = service.NormalizeViolationFeeError(err)
	require.False(t, shouldRetryWithTokenGroupPlan(c, err, 3, true))
	require.False(t, canTryFallbackModel(c, err))
}

func TestGeminiChatRetryAfterFirstFrameRejection(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{IsStream: true, DisablePing: true, RelayFormat: types.RelayFormatOpenAI, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-test"}}
	info.SetEstimatePromptTokens(99)
	const rejection = `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"first upstream rejected"}}`
	for i, data := range []string{
		rejection,
		`{"modelVersion":"gemini-test","responseId":"rejected-attempt"}` + "\n\ndata: " + rejection,
		`{"candidates":[{"content":{"parts":[{"text":"backup answer"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":3,"totalTokenCount":13}}`,
	} {
		info.BeginAttempt()
		usage, err := gemini.GeminiChatStreamHandler(c, info, &http.Response{Body: io.NopCloser(strings.NewReader("data: " + data + "\n\n"))})
		if i < 2 {
			require.NotNil(t, err)
			require.Nil(t, usage)
			require.False(t, c.Writer.Written())
			require.True(t, shouldRetryWithTokenGroupPlan(c, err, 1, true))
			require.True(t, canTryFallbackModel(c, err))
			continue
		}
		require.Nil(t, err)
		require.Equal(t, 13, usage.TotalTokens)
		require.Contains(t, w.Body.String(), "backup answer")
		require.NotContains(t, w.Body.String(), "first upstream rejected")
		require.NotContains(t, w.Body.String(), "rejected-attempt")
	}
}
