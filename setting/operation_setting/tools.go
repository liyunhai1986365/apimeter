package operation_setting

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

// ---------------------------------------------------------------------------
// Tool call prices ($/1K calls, admin-configurable)
// DB key: tool_price_setting.prices
//
// Key format:
//   - "tool_name"              → default price for all models
//   - "tool_name:model_prefix*" → override for models matching the prefix
//
// Effective index: hardcoded defaults → hardcoded model overrides → valid
// operator values. Lookup uses the longest model prefix before the tool
// default, and a matched numeric zero is terminal.
// ---------------------------------------------------------------------------

const ToolPriceOptionKey = "tool_price_setting.prices"

const (
	defaultWebSearchToolPrice        = 10.0
	defaultWebSearchPreviewToolPrice = 10.0
	defaultFileSearchToolPrice       = 2.5
	defaultGoogleSearchToolPrice     = 14.0
	defaultImageGenerationToolPrice  = 150.0
	defaultSearchPreviewModelPrice   = 25.0
)

// seedHardcodedToolPrices injects compile-time built-in fallbacks (tool
// defaults and model-prefix overrides) into the destination. The source is
// constants, not a mutable package map or operator configuration.
func seedHardcodedToolPrices(prices map[string]float64) {
	prices["web_search"] = defaultWebSearchToolPrice
	prices["web_search_preview"] = defaultWebSearchPreviewToolPrice
	prices["file_search"] = defaultFileSearchToolPrice
	prices["google_search"] = defaultGoogleSearchToolPrice
	// Gemini 3+ prices each unique query. Older models price a grounded prompt.
	prices["google_search_grounded_prompt"] = 0
	for _, prefix := range []string{"gemini-2.5*", "gemini-2.0*", "gemini-1.5*"} {
		prices["google_search:"+prefix] = 0
		prices["google_search_grounded_prompt:"+prefix] = 35
	}

	prices["image_generation"] = defaultImageGenerationToolPrice
	prices["web_search_preview:gpt-4o*"] = defaultSearchPreviewModelPrice
	prices["web_search_preview:gpt-4.1*"] = defaultSearchPreviewModelPrice
	prices["web_search_preview:gpt-4o-mini*"] = defaultSearchPreviewModelPrice
	prices["web_search_preview:gpt-4.1-mini*"] = defaultSearchPreviewModelPrice
}

// ToolPriceSetting is managed by config.GlobalConfig.Register.
// Prices holds operator overrides only; hardcoded fallbacks live in the index.
type ToolPriceSetting struct {
	Prices map[string]float64 `json:"prices"`
}

var toolPriceSetting = ToolPriceSetting{
	Prices: make(map[string]float64),
}

func init() {
	config.GlobalConfig.Register("tool_price_setting", &toolPriceSetting)
	RebuildToolPriceIndex()
}

// ---------------------------------------------------------------------------
// Precomputed price index (atomic, lock-free on read path)
// ---------------------------------------------------------------------------

type prefixEntry struct {
	prefix string
	price  float64
}

type toolPriceLookup struct {
	defaults map[string]float64
	prefixes map[string][]prefixEntry
}

type toolPriceIndex struct {
	toolPriceLookup
	operator toolPriceLookup
}

var currentIndex atomic.Pointer[toolPriceIndex]

func isValidToolPrice(price float64) bool {
	return price >= 0 && !math.IsNaN(price) && !math.IsInf(price, 0)
}

func decodeToolPricesJSON(value string, ignoreInvalidEntries bool) (map[string]float64, error) {
	rawValue := json.RawMessage(strings.TrimSpace(value))
	if common.GetJsonType(rawValue) != "object" {
		return nil, fmt.Errorf("工具价格必须是 JSON 对象")
	}

	var rawPrices map[string]json.RawMessage
	if err := common.Unmarshal(rawValue, &rawPrices); err != nil {
		return nil, fmt.Errorf("解析工具价格失败: %w", err)
	}

	prices := make(map[string]float64, len(rawPrices))
	for name, rawPrice := range rawPrices {
		var entryErr error
		if common.GetJsonType(rawPrice) != "number" {
			entryErr = fmt.Errorf("工具价格 %q 必须是非负数字", name)
		} else {
			var price float64
			if err := common.Unmarshal(rawPrice, &price); err != nil {
				entryErr = fmt.Errorf("解析工具价格 %q 失败: %w", name, err)
			} else if !isValidToolPrice(price) {
				entryErr = fmt.Errorf("工具价格 %q 必须是有限的非负数字", name)
			} else {
				prices[name] = price
			}
		}

		if entryErr == nil {
			continue
		}
		if !ignoreInvalidEntries {
			return nil, entryErr
		}
		common.SysError(entryErr.Error())
	}
	return prices, nil
}

// ValidateToolPricesJSON validates an operator-supplied complete price map.
// A numeric zero is valid and intentionally disables the matching rule.
func ValidateToolPricesJSON(value string) error {
	_, err := decodeToolPricesJSON(value, false)
	return err
}

// LoadToolPricesFromJSONString replaces the complete operator price map.
// Invalid legacy entries are ignored individually so valid sibling overrides
// survive, while missing built-in keys continue to use hardcoded fallbacks.
func LoadToolPricesFromJSONString(value string) {
	prices, err := decodeToolPricesJSON(value, true)
	if err != nil {
		common.SysError("加载工具价格失败，将使用硬编码兜底: " + err.Error())
		prices = make(map[string]float64)
	}
	toolPriceSetting.Prices = prices
	RebuildToolPriceIndex()
}

// RebuildToolPriceIndex rebuilds the lookup index from the current config.
// Called on init and after config updates. Not on the billing hot path.
func RebuildToolPriceIndex() {
	merged := make(map[string]float64, 9+len(toolPriceSetting.Prices))
	seedHardcodedToolPrices(merged)
	for k, v := range toolPriceSetting.Prices {
		if !isValidToolPrice(v) {
			continue
		}
		merged[k] = v
	}

	currentIndex.Store(&toolPriceIndex{
		toolPriceLookup: buildToolPriceLookup(merged),
		operator:        buildToolPriceLookup(toolPriceSetting.Prices),
	})
}

func buildToolPriceLookup(prices map[string]float64) toolPriceLookup {
	idx := toolPriceLookup{defaults: make(map[string]float64), prefixes: make(map[string][]prefixEntry)}
	for key, price := range prices {
		if !isValidToolPrice(price) {
			continue
		}
		toolName, modelPart, scoped := strings.Cut(key, ":")
		if !scoped {
			idx.defaults[key] = price
			continue
		}
		idx.prefixes[toolName] = append(idx.prefixes[toolName], prefixEntry{prefix: strings.TrimSuffix(modelPart, "*"), price: price})
	}
	for tool, entries := range idx.prefixes {
		sort.Slice(entries, func(i, j int) bool {
			if len(entries[i].prefix) == len(entries[j].prefix) {
				return entries[i].prefix < entries[j].prefix
			}
			return len(entries[i].prefix) > len(entries[j].prefix)
		})
		idx.prefixes[tool] = entries
	}
	return idx
}

func (idx toolPriceLookup) lookup(toolName, modelName string) (float64, bool) {
	if modelName != "" {
		for _, entry := range idx.prefixes[toolName] {
			if strings.HasPrefix(modelName, entry.prefix) {
				return entry.price, true
			}
		}
	}
	price, ok := idx.defaults[toolName]
	return price, ok
}

// GetToolPriceForModel returns $/1K calls: longest prefix, then tool default.
func GetToolPriceForModel(toolName, modelName string) float64 {
	price, _ := loadToolPriceIndex().lookup(toolName, modelName)
	return price
}

// GetGoogleSearchPrices returns per-query and per-grounded-request prices from
// one immutable snapshot. An existing google_search override keeps its old
// per-request unit until an explicit grounded-prompt rule matches this model.
// Numeric zero is an override, so upgrades cannot re-enable disabled charges.
func GetGoogleSearchPrices(modelName string) (queryPrice, groundedPrice float64) {
	idx := loadToolPriceIndex()
	queryPrice, querySet := idx.operator.lookup("google_search", modelName)
	groundedPrice, groundedSet := idx.operator.lookup("google_search_grounded_prompt", modelName)
	if querySet && !groundedSet {
		return 0, queryPrice
	}
	if !querySet {
		queryPrice, _ = idx.lookup("google_search", modelName)
	}
	if !groundedSet {
		groundedPrice, _ = idx.lookup("google_search_grounded_prompt", modelName)
	}
	return queryPrice, groundedPrice
}

func loadToolPriceIndex() *toolPriceIndex {
	if idx := currentIndex.Load(); idx != nil {
		return idx
	}
	RebuildToolPriceIndex()
	return currentIndex.Load()
}

// GetToolPrice is a convenience wrapper when no model name is needed.
func GetToolPrice(toolName string) float64 {
	return GetToolPriceForModel(toolName, "")
}

// SetToolPriceForTest injects a tool price and rebuilds the lookup index. Tests only.
func SetToolPriceForTest(name string, price float64) {
	if toolPriceSetting.Prices == nil {
		toolPriceSetting.Prices = make(map[string]float64)
	}
	toolPriceSetting.Prices[name] = price
	RebuildToolPriceIndex()
}

// DeleteToolPriceForTest removes an injected tool price and rebuilds the index. Tests only.
func DeleteToolPriceForTest(name string) {
	delete(toolPriceSetting.Prices, name)
	RebuildToolPriceIndex()
}

const (
	GPTImage1Low1024x1024    = 0.011
	GPTImage1Low1024x1536    = 0.016
	GPTImage1Low1536x1024    = 0.016
	GPTImage1Medium1024x1024 = 0.042
	GPTImage1Medium1024x1536 = 0.063
	GPTImage1Medium1536x1024 = 0.063
	GPTImage1High1024x1024   = 0.167
	GPTImage1High1024x1536   = 0.25
	GPTImage1High1536x1024   = 0.25
)

func GetGPTImage1PriceOnceCall(quality string, size string) float64 {
	prices := map[string]map[string]float64{
		"low": {
			"1024x1024": GPTImage1Low1024x1024,
			"1024x1536": GPTImage1Low1024x1536,
			"1536x1024": GPTImage1Low1536x1024,
		},
		"medium": {
			"1024x1024": GPTImage1Medium1024x1024,
			"1024x1536": GPTImage1Medium1024x1536,
			"1536x1024": GPTImage1Medium1536x1024,
		},
		"high": {
			"1024x1024": GPTImage1High1024x1024,
			"1024x1536": GPTImage1High1024x1536,
			"1536x1024": GPTImage1High1536x1024,
		},
	}
	if qualityMap, exists := prices[quality]; exists {
		if price, exists := qualityMap[size]; exists {
			return price
		}
	}
	return GPTImage1High1024x1024
}

// ---------------------------------------------------------------------------
// Gemini audio input pricing (per-million tokens, model-specific)
// ---------------------------------------------------------------------------

const (
	Gemini25FlashPreviewInputAudioPrice     = 1.00
	Gemini25FlashProductionInputAudioPrice  = 1.00
	Gemini25FlashLitePreviewInputAudioPrice = 0.50
	Gemini25FlashNativeAudioInputAudioPrice = 3.00
	Gemini20FlashInputAudioPrice            = 0.70
	GeminiRoboticsER15InputAudioPrice       = 1.00
)

func GetGeminiInputAudioPricePerMillionTokens(modelName string) float64 {
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview-native-audio") {
		return Gemini25FlashNativeAudioInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview-lite") {
		return Gemini25FlashLitePreviewInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.5-flash-preview") {
		return Gemini25FlashPreviewInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.5-flash") {
		return Gemini25FlashProductionInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-2.0-flash") {
		return Gemini20FlashInputAudioPrice
	}
	if strings.HasPrefix(modelName, "gemini-robotics-er-1.5") {
		return GeminiRoboticsER15InputAudioPrice
	}
	return 0
}
