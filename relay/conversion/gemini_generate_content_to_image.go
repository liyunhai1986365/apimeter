package conversion

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type geminiGenerateContentToImageGenerationsConverter struct{}

func (geminiGenerateContentToImageGenerationsConverter) ID() ConversionID {
	return ConversionGeminiGenerateContentToImageGenerations
}

func (geminiGenerateContentToImageGenerationsConverter) From() RequestMode {
	return RequestModeGeminiGenerateContent
}

func (geminiGenerateContentToImageGenerationsConverter) To() RequestMode {
	return RequestModeOpenAIImageGenerations
}

func (geminiGenerateContentToImageGenerationsConverter) Match(c *gin.Context, request dto.Request) bool {
	geminiRequest, ok := request.(*dto.GeminiChatRequest)
	if !ok || geminiRequest == nil {
		return false
	}
	if geminiResponseModalitiesIncludeImage(geminiRequest.GenerationConfig.ResponseModalities) {
		return true
	}
	modelName := geminiModelNameFromPath(c)
	return common.IsImageGenerationModel(modelName)
}

func (geminiGenerateContentToImageGenerationsConverter) ConvertRequest(c *gin.Context, request dto.Request) (dto.Request, error) {
	geminiRequest, ok := request.(*dto.GeminiChatRequest)
	if !ok {
		return nil, errors.New("request is not a GeminiChatRequest")
	}
	return ConvertGeminiGenerateContentToImageGenerationsRequest(c, geminiRequest)
}

func (converter geminiGenerateContentToImageGenerationsConverter) Plan(c *gin.Context, sourceFormat types.RelayFormat, sourceMode int) *Plan {
	return targetImageGenerationsPlan(c, converter.ID(), sourceFormat, sourceMode)
}

func ConvertGeminiGenerateContentToImageGenerationsRequest(c *gin.Context, geminiRequest *dto.GeminiChatRequest) (*dto.ImageRequest, error) {
	if geminiRequest == nil {
		return nil, errors.New("request is nil")
	}
	promptParts := make([]string, 0)
	imageRefs := make([]string, 0)
	for _, content := range geminiRequest.Contents {
		for _, part := range content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				promptParts = append(promptParts, strings.TrimSpace(part.Text))
			}
			if part.FileData != nil && strings.TrimSpace(part.FileData.FileUri) != "" {
				imageRefs = append(imageRefs, strings.TrimSpace(part.FileData.FileUri))
			}
			if part.InlineData != nil && strings.TrimSpace(part.InlineData.Data) != "" {
				imageRefs = append(imageRefs, geminiInlineDataURL(part.InlineData))
			}
		}
	}

	prompt := strings.Join(promptParts, "\n")
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("field prompt is required")
	}

	imageRequest := &dto.ImageRequest{
		Model:  geminiModelNameFromPath(c),
		Prompt: prompt,
		Extra:  map[string]json.RawMessage{},
	}
	if imageRequest.Model == "" {
		return nil, errors.New("model is required")
	}
	if geminiRequest.GenerationConfig.CandidateCount != nil && *geminiRequest.GenerationConfig.CandidateCount > 0 {
		imageRequest.N = common.GetPointer(uint(*geminiRequest.GenerationConfig.CandidateCount))
	}
	if len(imageRefs) > 0 {
		rawImages, err := common.Marshal(imageRefs)
		if err != nil {
			return nil, err
		}
		imageRequest.Images = json.RawMessage(rawImages)
	}
	if len(geminiRequest.GenerationConfig.ImageConfig) > 0 {
		copyGeminiImageConfigExtra(imageRequest, geminiRequest.GenerationConfig.ImageConfig)
	}
	return imageRequest, nil
}

func geminiResponseModalitiesIncludeImage(modalities []string) bool {
	for _, modality := range modalities {
		if strings.EqualFold(strings.TrimSpace(modality), "IMAGE") {
			return true
		}
	}
	return false
}

func geminiModelNameFromPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	path := c.Request.URL.Path
	const marker = "/models/"
	idx := strings.Index(path, marker)
	if idx < 0 {
		return ""
	}
	modelAndAction := path[idx+len(marker):]
	if colon := strings.Index(modelAndAction, ":"); colon >= 0 {
		modelAndAction = modelAndAction[:colon]
	}
	return strings.TrimSpace(modelAndAction)
}

func geminiInlineDataURL(inline *dto.GeminiInlineData) string {
	mimeType := strings.TrimSpace(inline.MimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, strings.TrimSpace(inline.Data))
}

func copyGeminiImageConfigExtra(imageRequest *dto.ImageRequest, raw json.RawMessage) {
	for _, field := range []struct {
		path string
		key  string
	}{
		{path: "aspectRatio", key: "aspect_ratio"},
		{path: "aspect_ratio", key: "aspect_ratio"},
		{path: "imageSize", key: "image_size"},
		{path: "image_size", key: "image_size"},
	} {
		result := gjson.GetBytes(raw, field.path)
		if !result.Exists() || result.Type == gjson.Null {
			continue
		}
		data, err := common.Marshal(result.Value())
		if err != nil {
			continue
		}
		imageRequest.Extra[field.key] = data
	}
}
