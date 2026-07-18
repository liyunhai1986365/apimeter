package volcengine

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// SeedreamSizeFromRequest returns the provider's pixel size when callers use
// the OpenAI-compatible aspect_ratio/resolution fields. An explicit size is
// intentionally returned unchanged, since the caller may use a provider
// specific value such as "2k".
func SeedreamSizeFromRequest(request dto.ImageRequest) string {
	if strings.TrimSpace(request.Size) != "" {
		return request.Size
	}
	if !strings.Contains(strings.ToLower(request.Model), "seedream") {
		return ""
	}
	aspectRatio := imageRequestExtraString(request, "aspect_ratio")
	resolution := imageRequestExtraString(request, "resolution")
	if aspectRatio == "" {
		return ""
	}
	if resolution == "" {
		resolution = "2K"
	}
	return seedreamSizeFromAspectRatio(request.Model, aspectRatio, resolution)
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

func seedreamSizeFromAspectRatio(model, aspectRatio, resolution string) string {
	aspectRatio = strings.TrimSpace(aspectRatio)
	resolution = strings.ToUpper(strings.TrimSpace(resolution))
	model = strings.ToLower(strings.TrimSpace(model))

	// The 5.0 Pro model has a distinct 1K/2K table. The regular 5.0 model and
	// its lite alias use the Lite table (2K/3K/4K).
	var table map[string]map[string]string
	switch {
	case strings.Contains(model, "seedream-5-0-pro"):
		table = seedream5ProSizes
	case strings.Contains(model, "seedream-5-0"):
		table = seedream5LiteSizes
	case strings.Contains(model, "seedream-4-5"):
		table = seedream5LiteSizes
	case strings.Contains(model, "seedream-4-0"), model == "seedream":
		table = seedream4Sizes
	default:
		return ""
	}
	return table[resolution][aspectRatio]
}

var seedream5ProSizes = map[string]map[string]string{
	"1K": {"1:1": "1024x1024", "4:3": "1152x864", "3:4": "864x1152", "16:9": "1424x800", "9:16": "800x1424", "3:2": "1248x832", "2:3": "832x1248", "21:9": "1568x672"},
	"2K": {"1:1": "2048x2048", "4:3": "2368x1776", "3:4": "1776x2368", "16:9": "2816x1584", "9:16": "1584x2816", "3:2": "2496x1664", "2:3": "1664x2496", "21:9": "3136x1344"},
}

var seedream5LiteSizes = map[string]map[string]string{
	"2K": {"1:1": "2048x2048", "4:3": "2304x1728", "3:4": "1728x2304", "16:9": "2848x1600", "9:16": "1600x2848", "3:2": "2496x1664", "2:3": "1664x2496", "21:9": "3136x1344"},
	"3K": {"1:1": "3072x3072", "4:3": "3456x2592", "3:4": "2592x3456", "16:9": "4096x2304", "9:16": "2304x4096", "3:2": "3744x2496", "2:3": "2496x3744", "21:9": "4704x2016"},
	"4K": {"1:1": "4096x4096", "4:3": "4704x3520", "3:4": "3520x4704", "16:9": "5504x3040", "9:16": "3040x5504", "3:2": "4992x3328", "2:3": "3328x4992", "21:9": "6240x2656"},
}

var seedream4Sizes = map[string]map[string]string{
	"1K": {"1:1": "1024x1024", "4:3": "1152x864", "3:4": "864x1152", "16:9": "1280x720", "9:16": "720x1280", "3:2": "1248x832", "2:3": "832x1248", "21:9": "1512x648"},
	"2K": {"1:1": "2048x2048", "4:3": "2304x1728", "3:4": "1728x2304", "16:9": "2848x1600", "9:16": "1600x2848", "3:2": "2496x1664", "2:3": "1664x2496", "21:9": "3136x1344"},
	"3K": {"1:1": "3072x3072", "4:3": "3456x2592", "3:4": "2592x3456", "16:9": "4096x2304", "9:16": "2304x4096", "3:2": "3744x2496", "2:3": "2496x3744", "21:9": "4704x2016"},
	"4K": {"1:1": "4096x4096", "4:3": "4704x3520", "3:4": "3520x4704", "16:9": "5504x3040", "9:16": "3040x5504", "3:2": "4992x3328", "2:3": "3328x4992", "21:9": "6240x2656"},
}
