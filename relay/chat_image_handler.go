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
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func maybeWrapChatImageResponse(c *gin.Context, info *relaycommon.RelayInfo, body []byte, usage *dto.Usage) (bool, *types.NewAPIError) {
	return maybeWrapImageResponse(c, info, body, usage)
}

func maybeWrapImageResponse(c *gin.Context, info *relaycommon.RelayInfo, body []byte, usage *dto.Usage) (bool, *types.NewAPIError) {
	if c == nil || info == nil || !shouldPreserveImageResponseMode(c, info) {
		return false, nil
	}
	switch sourceMode := imageResponseSourceMode(c, info); sourceMode {
	case string(conversion.RequestModeOpenAIChat):
		return wrapOpenAIChatImageResponse(c, info, body, usage)
	case string(conversion.RequestModeGeminiGenerateContent):
		return wrapGeminiImageResponse(c, info, body, usage)
	default:
		return false, nil
	}
}

func wrapOpenAIChatImageResponse(c *gin.Context, info *relaycommon.RelayInfo, body []byte, usage *dto.Usage) (bool, *types.NewAPIError) {
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

func wrapGeminiImageResponse(c *gin.Context, info *relaycommon.RelayInfo, body []byte, usage *dto.Usage) (bool, *types.NewAPIError) {
	imageResponse := dto.ImageResponse{}
	if err := common.Unmarshal(body, &imageResponse); err != nil {
		return false, types.NewOpenAIError(fmt.Errorf("failed to parse image response for Gemini compatibility: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	parts := make([]dto.GeminiPart, 0, len(imageResponse.Data))
	for _, image := range imageResponse.Data {
		part, err := geminiImagePartFromImageData(image)
		if err != nil {
			return false, types.NewOpenAIError(fmt.Errorf("failed to build image response for Gemini compatibility: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if part.InlineData != nil {
			parts = append(parts, part)
		}
	}
	if len(parts) == 0 {
		return false, types.NewOpenAIError(fmt.Errorf("failed to build image response for Gemini compatibility: no images"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	finishReason := "STOP"
	response := dto.GeminiChatResponse{
		Candidates: []dto.GeminiChatCandidate{
			{
				Index:        0,
				FinishReason: &finishReason,
				Content: dto.GeminiChatContent{
					Role:  "model",
					Parts: parts,
				},
			},
		},
		UsageMetadata: geminiImageUsageMetadata(usage),
	}
	resetChatImageResponseHeaders(c)
	c.JSON(http.StatusOK, response)
	return true, nil
}

func geminiImagePartFromImageData(image dto.ImageData) (dto.GeminiPart, error) {
	if b64Json := strings.TrimSpace(image.B64Json); b64Json != "" {
		return geminiInlineDataPartFromBase64(b64Json)
	}
	if imageURL := strings.TrimSpace(image.Url); imageURL != "" {
		lowerURL := strings.ToLower(imageURL)
		if strings.HasPrefix(lowerURL, "http://") || strings.HasPrefix(lowerURL, "https://") {
			mimeType, data, err := service.GetImageFromUrl(imageURL)
			if err != nil {
				return dto.GeminiPart{}, err
			}
			return geminiInlineDataPart(mimeType, data), nil
		}
		return geminiInlineDataPartFromBase64(imageURL)
	}
	return dto.GeminiPart{}, nil
}

func geminiInlineDataPartFromBase64(data string) (dto.GeminiPart, error) {
	cleanData := strings.TrimSpace(data)
	mimeType := "image/png"
	if strings.HasPrefix(strings.ToLower(cleanData), "data:") {
		decodedMimeType, decodedData, err := service.DecodeBase64FileData(cleanData)
		if err != nil {
			return dto.GeminiPart{}, err
		}
		mimeType = decodedMimeType
		cleanData = decodedData
	} else if decodedMimeType, decodedData, err := service.DecodeBase64FileData(cleanData); err == nil {
		mimeType = decodedMimeType
		cleanData = decodedData
	}
	return geminiInlineDataPart(mimeType, cleanData), nil
}

func geminiInlineDataPart(mimeType string, data string) dto.GeminiPart {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return dto.GeminiPart{InlineData: &dto.GeminiInlineData{
		MimeType: mimeType,
		Data:     strings.TrimSpace(data),
	}}
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

func shouldPreserveImageResponseMode(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info != nil && info.PreserveResponseMode && info.ConversionID != "" {
		return true
	}
	return conversion.ActiveConversionID(c) != "" && conversion.ShouldPreserveResponseMode(c)
}

func imageResponseSourceMode(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info != nil && strings.TrimSpace(info.SourceRequestMode) != "" {
		return strings.TrimSpace(info.SourceRequestMode)
	}
	if info != nil {
		switch conversion.ConversionID(info.ConversionID) {
		case conversion.ConversionOpenAIChatToImageGenerations:
			return string(conversion.RequestModeOpenAIChat)
		case conversion.ConversionGeminiGenerateContentToImageGenerations:
			return string(conversion.RequestModeGeminiGenerateContent)
		}
	}
	if c != nil {
		if mode := strings.TrimSpace(c.GetString(conversion.ContextKeySourceRequestMode)); mode != "" {
			return mode
		}
		switch conversion.ActiveConversionID(c) {
		case conversion.ConversionOpenAIChatToImageGenerations:
			return string(conversion.RequestModeOpenAIChat)
		case conversion.ConversionGeminiGenerateContentToImageGenerations:
			return string(conversion.RequestModeGeminiGenerateContent)
		}
	}
	return ""
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

func geminiImageUsageMetadata(usage *dto.Usage) dto.GeminiUsageMetadata {
	if usage == nil {
		return dto.GeminiUsageMetadata{PromptTokenCount: 1, CandidatesTokenCount: 1, TotalTokenCount: 1}
	}
	promptTokens := usage.PromptTokens
	if promptTokens == 0 {
		promptTokens = 1
	}
	candidateTokens := usage.CompletionTokens
	if candidateTokens == 0 {
		candidateTokens = 1
	}
	totalTokens := usage.TotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + candidateTokens
	}
	return dto.GeminiUsageMetadata{
		PromptTokenCount:     promptTokens,
		CandidatesTokenCount: candidateTokens,
		TotalTokenCount:      totalTokens,
	}
}
