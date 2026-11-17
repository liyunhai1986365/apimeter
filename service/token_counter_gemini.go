package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

func getGeminiImageToken(c *gin.Context, file *types.FileMeta, model string, stream bool) (int, error) {
	version := strings.TrimPrefix(strings.ToLower(model), "gemini-")
	end := 0
	for end < len(version) && version[end] >= '0' && version[end] <= '9' {
		end++
	}
	major, _ := strconv.Atoi(version[:end])
	if end == 0 || major >= 3 {
		return geminiMediaResolutionTokens(file.Detail), nil
	}
	w, h := 1024, 1024
	if constant.GetMediaToken && (stream || constant.GetMediaTokenNotStream) {
		config, _, err := GetImageConfig(c, file.Source)
		// This is an estimate, not input validation: an upstream may be able
		// to fetch or decode an image that this gateway cannot. Keep the same
		// fallback as disabled local media counting when dimensions are unavailable.
		if err == nil && config.Width > 0 && config.Height > 0 {
			w, h = config.Width, config.Height
		}
	}
	// Cap dimensions before arithmetic, as in the official estimator.
	if w > 65535 || h > 65535 {
		if w >= h {
			h = max(int(int64(h)*65535/int64(w)), 1)
			w = 65535
		} else {
			w = max(int(int64(w)*65535/int64(h)), 1)
			h = 65535
		}
	}
	return geminiTileImageTokens(w, h), nil
}

// geminiMediaResolutionTokens returns the documented per-image cost for a
// Gemini media resolution. Measured costs run 2-9% lower depending on aspect.
func geminiMediaResolutionTokens(detail string) int {
	level, ok := strings.CutPrefix(strings.ToLower(detail), "media_resolution_")
	if !ok {
		return 1120
	}
	switch level {
	case "low":
		return 280
	case "medium":
		return 560
	case "ultra_high":
		return 2240
	default:
		return 1120
	}
}

// geminiTileImageTokens follows the Gemini 2.x rule measured on gemini-2.5-flash:
// images up to 384px on both sides cost one 258-token tile; larger images are
// cut into square tiles of a third less than the shortest side (256 to 768px),
// at most 12, plus one thumbnail tile. 4K images bill below this cap.
func geminiTileImageTokens(w, h int) int {
	if w <= 384 && h <= 384 {
		return 258
	}
	tile := min(max(min(w, h)*2/3, 256), 768)
	tiles := min(((w+tile-1)/tile)*((h+tile-1)/tile), 12)
	return (tiles + 1) * 258
}
