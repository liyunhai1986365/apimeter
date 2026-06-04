package relay

import (
	"encoding/json"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
)

func ensureEstimatedImageUsageIfMissing(info *relaycommon.RelayInfo, request *dto.ImageRequest, usage *dto.Usage) bool {
	if usage == nil || request == nil || imageUsageHasAnyTokens(usage) {
		return false
	}
	modelName := strings.TrimSpace(request.Model)
	if modelName == "" && info != nil {
		modelName = info.OriginModelName
	}
	if !strings.EqualFold(modelName, "gpt-image-2") {
		return false
	}

	promptTokens := 0
	if info != nil {
		promptTokens = info.GetEstimatePromptTokens()
	}
	if promptTokens <= 0 {
		promptTokens = service.CountTextToken(request.Prompt, modelName)
	}
	if promptTokens <= 0 {
		promptTokens = 1
	}

	imageInputTokens := estimateGPTImage2InputImageTokens(info, request)
	inputTokens := promptTokens + imageInputTokens
	outputTokens := dto.EstimateGPTImage2OutputTokens(request)
	usage.PromptTokens = inputTokens
	usage.CompletionTokens = outputTokens
	usage.TotalTokens = inputTokens + outputTokens
	usage.InputTokens = inputTokens
	usage.OutputTokens = outputTokens
	usage.UsageSource = "estimated"
	usage.PromptTokensDetails.TextTokens = promptTokens
	usage.PromptTokensDetails.ImageTokens = imageInputTokens
	usage.CompletionTokenDetails.ImageTokens = outputTokens
	usage.InputTokensDetails = &dto.InputTokenDetails{
		TextTokens:  promptTokens,
		ImageTokens: imageInputTokens,
	}
	usage.OutputTokensDetails = &dto.OutputTokenDetails{
		ImageTokens: outputTokens,
	}
	return true
}

func estimateGPTImage2InputImageTokens(info *relaycommon.RelayInfo, request *dto.ImageRequest) int {
	inputImageCount := estimateGPTImage2InputImageCount(info, request)
	if inputImageCount <= 0 {
		return 0
	}
	return dto.EstimateGPTImage2ImageTokens(request) * inputImageCount
}

func estimateGPTImage2InputImageCount(info *relaycommon.RelayInfo, request *dto.ImageRequest) int {
	if request == nil {
		return 0
	}

	count := 0
	count += rawImageItemCount(request.Image)
	count += rawImageItemCount(request.Images)
	if raw, ok := request.Extra["image"]; ok {
		count += rawImageItemCount(raw)
	}
	if raw, ok := request.Extra["images"]; ok {
		count += rawImageItemCount(raw)
	}
	if raw, ok := request.Extra["image_urls"]; ok {
		count += rawImageItemCount(raw)
	}
	if raw, ok := request.Extra["input"]; ok {
		count += inputImageURLsCount(raw)
	}
	if count > 0 {
		return count
	}
	if info != nil && info.RelayMode == relayconstant.RelayModeImagesEdits {
		return 1
	}
	return 0
}

func rawImageItemCount(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var values []any
	if err := common.Unmarshal(raw, &values); err == nil {
		return len(values)
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil && strings.TrimSpace(value) != "" {
		return 1
	}
	var object map[string]any
	if err := common.Unmarshal(raw, &object); err == nil && len(object) > 0 {
		return 1
	}
	return 0
}

func inputImageURLsCount(raw json.RawMessage) int {
	var input struct {
		ImageURLs []string `json:"image_urls"`
	}
	if err := common.Unmarshal(raw, &input); err != nil {
		return 0
	}
	return len(input.ImageURLs)
}

func imageUsageHasAnyTokens(usage *dto.Usage) bool {
	if usage == nil {
		return false
	}
	return usage.PromptTokens != 0 ||
		usage.CompletionTokens != 0 ||
		usage.TotalTokens != 0 ||
		usage.InputTokens != 0 ||
		usage.OutputTokens != 0 ||
		usage.PromptTokensDetails.TextTokens != 0 ||
		usage.PromptTokensDetails.ImageTokens != 0 ||
		usage.PromptTokensDetails.AudioTokens != 0 ||
		usage.CompletionTokenDetails.TextTokens != 0 ||
		usage.CompletionTokenDetails.ImageTokens != 0 ||
		usage.CompletionTokenDetails.AudioTokens != 0
}

func imageResponseBodyWithUsage(body []byte, usage *dto.Usage) ([]byte, bool) {
	if !imageUsageHasAnyTokens(usage) {
		return nil, false
	}

	var payload map[string]json.RawMessage
	if err := common.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, false
	}
	if _, exists := payload["usage"]; exists {
		return nil, false
	}

	usageBytes, err := common.Marshal(imageAPIUsageFromUsage(usage))
	if err != nil {
		return nil, false
	}
	payload["usage"] = json.RawMessage(usageBytes)

	updated, err := common.Marshal(payload)
	if err != nil {
		return nil, false
	}
	return updated, true
}

type imageAPIUsage struct {
	InputTokens         int                  `json:"input_tokens"`
	OutputTokens        int                  `json:"output_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	InputTokensDetails  imageAPITokenDetails `json:"input_tokens_details"`
	OutputTokensDetails imageAPITokenDetails `json:"output_tokens_details"`
}

type imageAPITokenDetails struct {
	TextTokens  int `json:"text_tokens"`
	ImageTokens int `json:"image_tokens"`
}

func imageAPIUsageFromUsage(usage *dto.Usage) imageAPIUsage {
	if usage == nil {
		return imageAPIUsage{}
	}
	inputDetails := imageAPITokenDetails{
		TextTokens:  usage.PromptTokensDetails.TextTokens,
		ImageTokens: usage.PromptTokensDetails.ImageTokens,
	}
	if usage.InputTokensDetails != nil {
		inputDetails.TextTokens = usage.InputTokensDetails.TextTokens
		inputDetails.ImageTokens = usage.InputTokensDetails.ImageTokens
	}
	outputDetails := imageAPITokenDetails{
		TextTokens:  usage.CompletionTokenDetails.TextTokens,
		ImageTokens: usage.CompletionTokenDetails.ImageTokens,
	}
	if usage.OutputTokensDetails != nil {
		outputDetails.TextTokens = usage.OutputTokensDetails.TextTokens
		outputDetails.ImageTokens = usage.OutputTokensDetails.ImageTokens
	}
	return imageAPIUsage{
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		TotalTokens:         usage.TotalTokens,
		InputTokensDetails:  inputDetails,
		OutputTokensDetails: outputDetails,
	}
}
