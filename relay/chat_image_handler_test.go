package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMaybeWrapChatImageResponseWrapsOpenAIImagePayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-image-2")
	c.Set(conversion.ContextKeyConversionID, string(conversion.ConversionOpenAIChatToImageGenerations))
	c.Set(conversion.ContextKeyPreserveResponse, true)

	info := &relaycommon.RelayInfo{
		RelayMode:            relayconstant.RelayModeImagesGenerations,
		RelayFormat:          types.RelayFormatOpenAIImage,
		OriginModelName:      "gpt-image-2",
		Request:              &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw"},
		ConversionID:         string(conversion.ConversionOpenAIChatToImageGenerations),
		PreserveResponseMode: true,
	}
	body := []byte(`{"created":1779125744,"data":[{"url":"https://cdn.example.com/output.png","b64_json":"","revised_prompt":""}]}`)

	wrapped, err := maybeWrapChatImageResponse(c, info, body, &dto.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7})
	require.Nil(t, err)
	require.True(t, wrapped)

	var response dto.OpenAITextResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, "chat.completion", response.Object)
	require.Equal(t, "gpt-image-2", response.Model)
	require.Len(t, response.Choices, 1)
	require.Equal(t, "assistant", response.Choices[0].Message.Role)
	require.Equal(t, "stop", response.Choices[0].FinishReason)
	require.Equal(t, 7, response.Usage.TotalTokens)
	require.Equal(t, "![image](https://cdn.example.com/output.png)", response.Choices[0].Message.StringContent())
}

func TestMaybeWrapChatImageResponseClearsCapturedImageContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Writer.Header().Set("Content-Length", "45")
	c.Writer.Header().Set("Api-Request-Path", "/v1/images/generations")

	info := &relaycommon.RelayInfo{
		RelayMode:            relayconstant.RelayModeImagesGenerations,
		RelayFormat:          types.RelayFormatOpenAIImage,
		OriginModelName:      "gpt-image-2",
		Request:              &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw"},
		ConversionID:         string(conversion.ConversionOpenAIChatToImageGenerations),
		PreserveResponseMode: true,
	}

	wrapped, err := maybeWrapChatImageResponse(c, info, []byte(`{"created":1779125744,"data":[{"url":"https://cdn.example.com/output.png"}]}`), &dto.Usage{})
	require.Nil(t, err)
	require.True(t, wrapped)
	require.NotEqual(t, "45", recorder.Header().Get("Content-Length"))
	require.Empty(t, recorder.Header().Get("Api-Request-Path"))
	require.Contains(t, recorder.Body.String(), "https://cdn.example.com/output.png")
}
