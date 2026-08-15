package dto

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestImageRequestGetTokenCountMetaGPTImage2EstimatesOutputTokens(t *testing.T) {
	tests := []struct {
		name          string
		size          string
		quality       string
		n             *uint
		partialImages string
		wantMaxTokens int
	}{
		{
			name:          "medium square default",
			size:          "1024x1024",
			quality:       "medium",
			wantMaxTokens: 765,
		},
		{
			name:          "high landscape",
			size:          "1536x1024",
			quality:       "high",
			wantMaxTokens: 1530,
		},
		{
			name:          "low square two images",
			size:          "1024x1024",
			quality:       "low",
			n:             common.GetPointer(uint(2)),
			wantMaxTokens: 1530,
		},
		{
			name:          "partial images add per generated image",
			size:          "1024x1024",
			quality:       "medium",
			n:             common.GetPointer(uint(2)),
			partialImages: "3",
			wantMaxTokens: 2130,
		},
		{
			name:          "explicit 4k resolution",
			size:          "1024x1024",
			quality:       "medium",
			wantMaxTokens: 3060,
		},
		{
			name:          "nested 2k resolution",
			size:          "1024x1024",
			quality:       "medium",
			wantMaxTokens: 1530,
		},
		{
			name:          "auto values fall back to medium square",
			size:          "auto",
			quality:       "auto",
			wantMaxTokens: 765,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ImageRequest{
				Model:   "gpt-image-2",
				Prompt:  "draw",
				Size:    tt.size,
				Quality: tt.quality,
				N:       tt.n,
			}
			switch tt.name {
			case "explicit 4k resolution":
				req.Extra = map[string]json.RawMessage{"resolution": []byte(`"4k"`)}
			case "nested 2k resolution":
				req.Extra = map[string]json.RawMessage{"input": []byte(`{"resolution":"2k"}`)}
			}
			if tt.partialImages != "" {
				req.PartialImages = []byte(tt.partialImages)
			}

			meta := req.GetTokenCountMeta()

			require.Equal(t, tt.wantMaxTokens, meta.MaxTokens)
			require.Equal(t, 1.0, meta.ImagePriceRatio)
		})
	}
}

func TestImageRequestGetTokenCountMetaGPTImage1KeepsLegacyEstimate(t *testing.T) {
	req := ImageRequest{
		Model:   "gpt-image-1",
		Prompt:  "draw",
		Size:    "1024x1536",
		Quality: "medium",
	}

	meta := req.GetTokenCountMeta()

	require.Equal(t, 1584, meta.MaxTokens)
	require.Equal(t, 1.0, meta.ImagePriceRatio)
}
