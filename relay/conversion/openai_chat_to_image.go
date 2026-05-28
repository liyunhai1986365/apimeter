package conversion

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type openAIChatToImageGenerationsConverter struct{}

func (openAIChatToImageGenerationsConverter) ID() ConversionID {
	return ConversionOpenAIChatToImageGenerations
}

func (openAIChatToImageGenerationsConverter) From() RequestMode {
	return RequestModeOpenAIChat
}

func (openAIChatToImageGenerationsConverter) To() RequestMode {
	return RequestModeOpenAIImageGenerations
}

func (openAIChatToImageGenerationsConverter) Match(_ *gin.Context, request dto.Request) bool {
	textRequest, ok := request.(*dto.GeneralOpenAIRequest)
	return ok && textRequest != nil && common.IsImageGenerationModel(textRequest.Model)
}

func (openAIChatToImageGenerationsConverter) ConvertRequest(_ *gin.Context, request dto.Request) (dto.Request, error) {
	textRequest, ok := request.(*dto.GeneralOpenAIRequest)
	if !ok {
		return nil, errors.New("request is not a GeneralOpenAIRequest")
	}
	return ConvertOpenAIChatToImageGenerationsRequest(textRequest)
}

func (converter openAIChatToImageGenerationsConverter) Plan(c *gin.Context, sourceFormat types.RelayFormat, sourceMode int) *Plan {
	return targetImageGenerationsPlan(c, converter.ID(), sourceFormat, sourceMode)
}

func ConvertOpenAIChatToImageGenerationsRequest(textRequest *dto.GeneralOpenAIRequest) (*dto.ImageRequest, error) {
	if textRequest == nil {
		return nil, errors.New("request is nil")
	}
	promptParts := make([]string, 0, len(textRequest.Messages))
	imageURLs := make([]string, 0)
	for _, message := range textRequest.Messages {
		switch content := message.Content.(type) {
		case string:
			if strings.TrimSpace(content) != "" {
				promptParts = append(promptParts, strings.TrimSpace(content))
			}
		default:
			for _, media := range message.ParseContent() {
				switch media.Type {
				case dto.ContentTypeText:
					if strings.TrimSpace(media.Text) != "" {
						promptParts = append(promptParts, strings.TrimSpace(media.Text))
					}
				case dto.ContentTypeImageURL:
					if image := media.GetImageMedia(); image != nil && strings.TrimSpace(image.Url) != "" {
						imageURLs = append(imageURLs, strings.TrimSpace(image.Url))
					}
				}
			}
		}
	}

	prompt := strings.Join(promptParts, "\n")
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("field prompt is required")
	}

	imageRequest := &dto.ImageRequest{
		Model:  textRequest.Model,
		Prompt: prompt,
		Size:   textRequest.Size,
		User:   textRequest.User,
	}
	if textRequest.N != nil && *textRequest.N > 0 {
		imageRequest.N = common.GetPointer(uint(*textRequest.N))
	}
	if len(imageURLs) > 0 {
		rawImages, err := common.Marshal(imageURLs)
		if err != nil {
			return nil, err
		}
		imageRequest.Images = json.RawMessage(rawImages)
	}
	return imageRequest, nil
}
