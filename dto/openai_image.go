package dto

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const MaxImageN = 128

type ImageRequest struct {
	Model             string          `json:"model"`
	Prompt            string          `json:"prompt" binding:"required"`
	N                 *uint           `json:"n,omitempty"`
	Size              string          `json:"size,omitempty"`
	Quality           string          `json:"quality,omitempty"`
	ResponseFormat    string          `json:"response_format,omitempty"`
	Style             json.RawMessage `json:"style,omitempty"`
	User              json.RawMessage `json:"user,omitempty"`
	ExtraFields       json.RawMessage `json:"extra_fields,omitempty"`
	Background        json.RawMessage `json:"background,omitempty"`
	Moderation        json.RawMessage `json:"moderation,omitempty"`
	OutputFormat      json.RawMessage `json:"output_format,omitempty"`
	OutputCompression json.RawMessage `json:"output_compression,omitempty"`
	PartialImages     json.RawMessage `json:"partial_images,omitempty"`
	// Stream            bool            `json:"stream,omitempty"`
	Images                           json.RawMessage `json:"images,omitempty"`
	Mask                             json.RawMessage `json:"mask,omitempty"`
	InputFidelity                    json.RawMessage `json:"input_fidelity,omitempty"`
	Watermark                        *bool           `json:"watermark,omitempty"`
	SequentialImageGeneration        string          `json:"sequential_image_generation,omitempty"`
	SequentialImageGenerationOptions json.RawMessage `json:"sequential_image_generation_options,omitempty"`
	Stream                           *bool           `json:"stream,omitempty"`
	Tools                            json.RawMessage `json:"tools,omitempty"`
	OptimizePromptOptions            json.RawMessage `json:"optimize_prompt_options,omitempty"`
	// zhipu 4v
	WatermarkEnabled json.RawMessage `json:"watermark_enabled,omitempty"`
	UserId           json.RawMessage `json:"user_id,omitempty"`
	Image            json.RawMessage `json:"image,omitempty"`
	// 用匿名参数接收额外参数
	Extra map[string]json.RawMessage `json:"-"`
}

func (i *ImageRequest) UnmarshalJSON(data []byte) error {
	// 先解析成 map[string]interface{}
	var rawMap map[string]json.RawMessage
	if err := common.Unmarshal(data, &rawMap); err != nil {
		return err
	}

	// 用 struct tag 获取所有已定义字段名
	knownFields := GetJSONFieldNames(reflect.TypeOf(*i))

	// 再正常解析已定义字段
	type Alias ImageRequest
	var known Alias
	if err := common.Unmarshal(data, &known); err != nil {
		return err
	}
	*i = ImageRequest(known)

	// 提取多余字段
	i.Extra = make(map[string]json.RawMessage)
	for k, v := range rawMap {
		if _, ok := knownFields[k]; !ok {
			i.Extra[k] = v
		}
	}
	return nil
}

// 序列化时需要重新把字段平铺
func (r ImageRequest) MarshalJSON() ([]byte, error) {
	// 将已定义字段转为 map
	type Alias ImageRequest
	alias := Alias(r)
	base, err := common.Marshal(alias)
	if err != nil {
		return nil, err
	}

	var baseMap map[string]json.RawMessage
	if err := common.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}

	for key, value := range r.Extra {
		if _, exists := baseMap[key]; exists || len(value) == 0 {
			continue
		}
		baseMap[key] = append(json.RawMessage(nil), value...)
	}

	return common.Marshal(baseMap)
}

func GetJSONFieldNames(t reflect.Type) map[string]struct{} {
	fields := make(map[string]struct{})
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// 跳过匿名字段（例如 ExtraFields）
		if field.Anonymous {
			continue
		}

		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}

		// 取逗号前字段名（排除 omitempty 等）
		name := tag
		if commaIdx := indexComma(tag); commaIdx != -1 {
			name = tag[:commaIdx]
		}
		fields[name] = struct{}{}
	}
	return fields
}

func indexComma(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			return i
		}
	}
	return -1
}

func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0
	maxTokens := 1584

	if strings.HasPrefix(i.Model, "dall-e") {
		// Size
		if i.Size == "256x256" {
			sizeRatio = 0.4
		} else if i.Size == "512x512" {
			sizeRatio = 0.45
		} else if i.Size == "1024x1024" {
			sizeRatio = 1
		} else if i.Size == "1024x1792" || i.Size == "1792x1024" {
			sizeRatio = 2
		}

		if i.Model == "dall-e-3" && i.Quality == "hd" {
			qualityRatio = 2.0
			if i.Size == "1024x1792" || i.Size == "1792x1024" {
				qualityRatio = 1.5
			}
		}
	} else if strings.EqualFold(strings.TrimSpace(i.Model), "gpt-image-2") {
		maxTokens = EstimateGPTImage2OutputTokens(i)
	}

	// For legacy image models, n is NOT included here; it is handled via OtherRatio("n") in
	// image_handler.go (default) or channel adaptors (actual count).
	// Including n here caused double-counting for channels that also
	// set OtherRatio("n") (e.g. Ali/Bailian).
	return &types.TokenCountMeta{
		CombineText:     i.Prompt,
		MaxTokens:       maxTokens,
		ImagePriceRatio: sizeRatio * qualityRatio,
	}
}

func EstimateGPTImage2OutputTokens(request *ImageRequest) int {
	baseTokens := EstimateGPTImage2ImageTokens(request)
	imageCount := imageRequestCount(request.N)

	tokensPerImage := baseTokens
	partialImages := parsePartialImages(request.PartialImages)
	if partialImages > 0 {
		tokensPerImage += 100 * partialImages
	}
	return tokensPerImage * imageCount
}

func EstimateGPTImage2ImageTokens(request *ImageRequest) int {
	switch gptImage2Resolution(request) {
	case "4k":
		return 3060
	case "2k":
		return 1530
	default:
		return 765
	}
}

func gptImage2Resolution(request *ImageRequest) string {
	if request == nil {
		return "1k"
	}
	if resolution := normalizeGPTImage2Resolution(rawStringField(request.Extra["resolution"])); resolution != "" {
		return resolution
	}
	if raw, ok := request.Extra["input"]; ok {
		var input struct {
			Resolution string `json:"resolution"`
		}
		if err := common.Unmarshal(raw, &input); err == nil {
			if resolution := normalizeGPTImage2Resolution(input.Resolution); resolution != "" {
				return resolution
			}
		}
	}
	return gptImage2ResolutionFromSize(request.Size)
}

func normalizeGPTImage2Resolution(resolution string) string {
	switch strings.ToLower(strings.TrimSpace(resolution)) {
	case "1k", "2k", "4k":
		return strings.ToLower(strings.TrimSpace(resolution))
	default:
		return ""
	}
}

func rawStringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		return value
	}
	return ""
}

func gptImage2ResolutionFromSize(size string) string {
	normalized := strings.ToLower(strings.TrimSpace(size))
	if normalized == "" || normalized == "auto" {
		return "1k"
	}
	normalized = strings.ReplaceAll(normalized, "×", "x")
	parts := strings.Split(normalized, "x")
	if len(parts) != 2 {
		return "1k"
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil || width <= 0 || height <= 0 {
		return "1k"
	}
	maxSide := width
	if height > maxSide {
		maxSide = height
	}
	if maxSide > 2048 {
		return "4k"
	}
	if maxSide > 1024 {
		return "2k"
	}
	return "1k"
}

func imageRequestCount(n *uint) int {
	if n == nil || *n == 0 {
		return 1
	}
	return int(*n)
}

func parsePartialImages(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var value int
	if err := common.Unmarshal(raw, &value); err == nil && value > 0 {
		return value
	}
	var stringValue string
	if err := common.Unmarshal(raw, &stringValue); err == nil {
		value, err := strconv.Atoi(strings.TrimSpace(stringValue))
		if err == nil && value > 0 {
			return value
		}
	}
	return 0
}

func (i *ImageRequest) IsStream(c *gin.Context) bool {
	if i.Stream == nil || !*i.Stream {
		return false
	}
	// Seedream 5.0 is the image family with a documented SSE contract. Keep
	// legacy image providers on their existing non-streaming response path.
	return strings.Contains(strings.ToLower(i.Model), "seedream-5-0")
}

func (i *ImageRequest) SetModelName(modelName string) {
	if modelName != "" {
		i.Model = modelName
	}
}

type ImageResponse struct {
	Data     []ImageData     `json:"data"`
	Created  int64           `json:"created"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}
type ImageData struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}
