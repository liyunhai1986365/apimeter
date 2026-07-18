package volcengine

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestSeedreamSizeFromRequest(t *testing.T) {
	newRequest := func(model, aspect, resolution, size string) dto.ImageRequest {
		extra := map[string]json.RawMessage{}
		if aspect != "" {
			extra["aspect_ratio"] = json.RawMessage(`"` + aspect + `"`)
		}
		if resolution != "" {
			extra["resolution"] = json.RawMessage(`"` + resolution + `"`)
		}
		return dto.ImageRequest{Model: model, Size: size, Extra: extra}
	}

	tests := []struct {
		name, model, aspect, resolution, size, want string
	}{
		{"5 pro 2K", "doubao-seedream-5-0-pro-260628", "16:9", "2K", "", "2816x1584"},
		{"missing resolution defaults to 2K", "doubao-seedream-5-0-pro-260628", "16:9", "", "", "2816x1584"},
		{"5 lite 2K", "doubao-seedream-5-0-260128", "16:9", "2K", "", "2848x1600"},
		{"5 lite alias 4K", "doubao-seedream-5-0-lite-260128", "1:1", "4k", "", "4096x4096"},
		{"4.5 2K", "doubao-seedream-4-5-251128", "16:9", "2K", "", "2848x1600"},
		{"4.0 1K", "doubao-seedream-4-0-250828", "16:9", "1K", "", "1280x720"},
		{"explicit quality is preserved", "doubao-seedream-5-0-pro-260628", "16:9", "2K", "2k", "2k"},
		{"explicit pixels are preserved", "doubao-seedream-5-0-pro-260628", "16:9", "2K", "2048x1024", "2048x1024"},
		{"non seedream", "some-image-model", "16:9", "2K", "", ""},
		{"unknown pair", "doubao-seedream-5-0-pro-260628", "5:4", "2K", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, SeedreamSizeFromRequest(newRequest(tt.model, tt.aspect, tt.resolution, tt.size)))
		})
	}
}
