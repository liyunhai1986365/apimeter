package relay

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func maybeWrapChatImageResponse(c *gin.Context, info *relaycommon.RelayInfo, body []byte, usage *dto.Usage) (bool, *types.NewAPIError) {
	if c == nil || info == nil || !isChatImageResponseCompatibility(c, info) {
		return false, nil
	}
	imageResponse := dto.ImageResponse{}
	if err := common.Unmarshal(body, &imageResponse); err != nil {
		return false, types.NewOpenAIError(fmt.Errorf("failed to parse image response for chat compatibility: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	content, err := chatImageContent(imageResponse)
	if err != nil {
		return false, types.NewOpenAIError(fmt.Errorf("failed to build image response for chat compatibility: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	created := imageResponse.Created
	if created == 0 {
		created = time.Now().Unix()
	}
	response := dto.OpenAITextResponse{
		Id:      "chatcmpl-" + info.RequestId,
		Object:  "chat.completion",
		Created: created,
		Model:   info.OriginModelName,
		Choices: []dto.OpenAITextResponseChoice{
			{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: chatImageUsage(usage),
	}
	resetChatImageResponseHeaders(c)
	c.JSON(http.StatusOK, response)
	return true, nil
}

func resetChatImageResponseHeaders(c *gin.Context) {
	header := c.Writer.Header()
	header.Del("Content-Length")
	header.Del("Content-Encoding")
	header.Del("Transfer-Encoding")
	header.Del("Api-Request-Path")
	header.Set("Content-Type", "application/json; charset=utf-8")
}

func chatImageContent(imageResponse dto.ImageResponse) (string, error) {
	parts := make([]string, 0, len(imageResponse.Data))
	for _, image := range imageResponse.Data {
		if strings.TrimSpace(image.Url) != "" {
			parts = append(parts, fmt.Sprintf("![image](%s)", strings.TrimSpace(image.Url)))
			continue
		}
		if strings.TrimSpace(image.B64Json) != "" {
			parts = append(parts, fmt.Sprintf("![image](data:image/png;base64,%s)", strings.TrimSpace(image.B64Json)))
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, "\n"), nil
	}
	content, err := common.Marshal(imageResponse)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func isChatImageResponseCompatibility(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info != nil && info.ConversionID == string(conversion.ConversionOpenAIChatToImageGenerations) && info.PreserveResponseMode {
		return true
	}
	return conversion.ActiveConversionID(c) == conversion.ConversionOpenAIChatToImageGenerations && conversion.ShouldPreserveResponseMode(c)
}

func chatImageUsage(usage *dto.Usage) dto.Usage {
	if usage == nil {
		return dto.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 1}
	}
	out := *usage
	if out.TotalTokens == 0 {
		out.TotalTokens = 1
	}
	if out.PromptTokens == 0 {
		out.PromptTokens = 1
	}
	return out
}
