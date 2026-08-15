package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/imageconv"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func ConvertOpenAIImageToGenerateContent(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*dto.GeminiChatRequest, error) {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return nil, types.MarkRequestError(errors.New("prompt is required"))
	}

	content := make([]any, 0, 2)
	content = append(content, dto.MediaContent{
		Type: dto.ContentTypeText,
		Text: prompt,
	})
	imageReferences, err := imageconv.RequestImageReferences(request)
	if err != nil {
		return nil, fmt.Errorf("invalid image references: %w", err)
	}
	for _, reference := range imageReferences {
		content = append(content, dto.MediaContent{
			Type: dto.ContentTypeImageURL,
			ImageUrl: &dto.MessageImageUrl{
				Url: reference,
			},
		})
	}

	openAIRequest := dto.GeneralOpenAIRequest{
		Model: request.Model,
		Messages: []dto.Message{
			{
				Role:    "user",
				Content: content,
			},
		},
	}
	if imageConfig := geminiImageConfigFromOpenAIRequest(request); len(imageConfig) > 0 {
		extraBody, err := common.Marshal(map[string]any{
			"google": map[string]any{
				"image_config": imageConfig,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal Gemini image config: %w", err)
		}
		openAIRequest.ExtraBody = extraBody
	}

	geminiRequest, err := CovertOpenAI2Gemini(c, openAIRequest, info)
	if err != nil {
		return nil, err
	}
	geminiRequest.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
	if request.N != nil && *request.N > 0 {
		count := int(*request.N)
		geminiRequest.GenerationConfig.CandidateCount = &count
	}
	return geminiRequest, nil
}

func geminiImageConfigFromOpenAIRequest(request dto.ImageRequest) map[string]any {
	config := make(map[string]any)
	if aspectRatio := imageRequestExtraString(request, "aspect_ratio"); aspectRatio != "" {
		config["aspect_ratio"] = aspectRatio
	} else if aspectRatio := geminiAspectRatioFromSize(request.Size); aspectRatio != "" {
		config["aspect_ratio"] = aspectRatio
	}

	imageSize := imageRequestExtraString(request, "image_size")
	if imageSize == "" {
		imageSize = imageRequestExtraString(request, "resolution")
	}
	if imageSize == "" {
		imageSize = geminiImageSizeFromOpenAIRequest(request)
	}
	if imageSize = normalizeGeminiImageSize(imageSize); imageSize != "" {
		config["image_size"] = imageSize
	}
	return config
}

func imageRequestExtraString(request dto.ImageRequest, key string) string {
	raw := request.Extra[key]
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func geminiAspectRatioFromSize(size string) string {
	size = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(size, "×", "x")))
	if strings.Contains(size, ":") {
		return size
	}
	known := map[string]string{
		"256x256":   "1:1",
		"512x512":   "1:1",
		"1024x1024": "1:1",
		"2048x2048": "1:1",
		"1024x1536": "2:3",
		"1536x1024": "3:2",
		"1024x1792": "9:16",
		"1792x1024": "16:9",
		"1152x864":  "4:3",
		"864x1152":  "3:4",
	}
	return known[size]
}

func geminiImageSizeFromOpenAIRequest(request dto.ImageRequest) string {
	size := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(request.Size, "×", "x")))
	parts := strings.Split(size, "x")
	if len(parts) == 2 {
		width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if widthErr == nil && heightErr == nil && width > 0 && height > 0 {
			maxSide := width
			if height > maxSide {
				maxSide = height
			}
			switch {
			case maxSide > 2048:
				return "4K"
			case maxSide > 1024:
				return "2K"
			default:
				return "1K"
			}
		}
	}
	if strings.EqualFold(request.Quality, "high") || strings.EqualFold(request.Quality, "hd") {
		return "2K"
	}
	return ""
}

func normalizeGeminiImageSize(size string) string {
	switch strings.ToUpper(strings.TrimSpace(size)) {
	case "0.5K":
		return "0.5K"
	case "1K":
		return "1K"
	case "2K":
		return "2K"
	case "4K":
		return "4K"
	default:
		return ""
	}
}

func GeminiGenerateContentImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	openAIResponse := dto.ImageResponse{
		Created: common.GetTimestamp(),
		Data:    make([]dto.ImageData, 0),
	}
	for _, candidate := range geminiResponse.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.InlineData == nil || part.InlineData.Data == "" || !strings.HasPrefix(strings.ToLower(part.InlineData.MimeType), "image/") {
				continue
			}
			openAIResponse.Data = append(openAIResponse.Data, dto.ImageData{
				B64Json: part.InlineData.Data,
			})
		}
	}
	if len(openAIResponse.Data) == 0 {
		return nil, types.NewOpenAIError(errors.New("Gemini response contains no generated images"), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	jsonResponse, err := common.Marshal(openAIResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	usage := buildUsageFromGeminiMetadata(geminiResponse.UsageMetadata, info.GetEstimatePromptTokens())
	return &usage, nil
}
