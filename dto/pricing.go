package dto

import "github.com/QuantumNous/new-api/constant"

// 这里不好动就不动了，本来想独立出来的（
type OpenAIModels struct {
	Id                     string                  `json:"id"`
	Object                 string                  `json:"object"`
	Created                int                     `json:"created"`
	OwnedBy                string                  `json:"owned_by"`
	SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
}

type ModelMetadataPricing struct {
	RangeStart     int    `json:"range_start"`
	RangeEnd       *int   `json:"range_end"`
	Label          string `json:"label"`
	InputMode      string `json:"inputMode"`
	Resolution     string `json:"resolution"`
	Prompt         string `json:"prompt"`
	Completion     string `json:"completion"`
	Image          string `json:"image"`
	Request        string `json:"request"`
	Duration       string `json:"duration"`
	InputCacheRead string `json:"input_cache_read"`
}

type ModelMetadata struct {
	Id                string                 `json:"id"`
	Name              string                 `json:"name"`
	InputModalities   []string               `json:"input_modalities"`
	OutputModalities  []string               `json:"output_modalities"`
	ContextLength     int                    `json:"context_length"`
	MaxOutputLength   int                    `json:"max_output_length"`
	Currency          string                 `json:"currency"`
	Description       string                 `json:"description"`
	ProviderLogoURL   string                 `json:"provider_logo_url"`
	Pricing           []ModelMetadataPricing `json:"pricing"`
	SupportedFeatures []string               `json:"supported_features"`
	IsReady           bool                   `json:"is_ready"`
}

type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

type GeminiModel struct {
	Name                       interface{}   `json:"name"`
	BaseModelId                interface{}   `json:"baseModelId"`
	Version                    interface{}   `json:"version"`
	DisplayName                interface{}   `json:"displayName"`
	Description                interface{}   `json:"description"`
	InputTokenLimit            interface{}   `json:"inputTokenLimit"`
	OutputTokenLimit           interface{}   `json:"outputTokenLimit"`
	SupportedGenerationMethods []interface{} `json:"supportedGenerationMethods"`
	Thinking                   interface{}   `json:"thinking"`
	Temperature                interface{}   `json:"temperature"`
	MaxTemperature             interface{}   `json:"maxTemperature"`
	TopP                       interface{}   `json:"topP"`
	TopK                       interface{}   `json:"topK"`
}
