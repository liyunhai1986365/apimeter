package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsGeminiImageGenerationModel(t *testing.T) {
	tests := map[string]bool{
		"gemini-3.1-flash-image-preview": true,
		" GEMINI-future-IMAGE ":          true,
		"gemini-3.1-pro-preview":         false,
		"imagen-4.0-generate-001":        false,
		"proxy-gemini-image":             false,
	}

	for model, expected := range tests {
		require.Equal(t, expected, IsGeminiImageGenerationModel(model), model)
	}
}

func TestIsImageGenerationModelRecognizesFutureGeminiImageModel(t *testing.T) {
	require.True(t, IsImageGenerationModel("gemini-future-image-model"))
}
