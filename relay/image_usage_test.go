package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestEnsureEstimatedImageUsageForGPTImage2WhenUpstreamUsageMissing(t *testing.T) {
	imageCount := uint(2)
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2",
		RelayMode:       relayconstant.RelayModeImagesEdits,
	}
	info.SetEstimatePromptTokens(11)
	request := &dto.ImageRequest{
		Model:         "gpt-image-2",
		Prompt:        "edit this image",
		Size:          "1024x1024",
		Quality:       "medium",
		N:             &imageCount,
		PartialImages: []byte("1"),
	}
	usage := &dto.Usage{}

	estimated := ensureEstimatedImageUsageIfMissing(info, request, usage)

	require.True(t, estimated)
	require.Equal(t, 776, usage.PromptTokens)
	require.Equal(t, 1730, usage.CompletionTokens)
	require.Equal(t, 2506, usage.TotalTokens)
	require.Equal(t, 776, usage.InputTokens)
	require.Equal(t, 1730, usage.OutputTokens)
	require.Equal(t, 11, usage.PromptTokensDetails.TextTokens)
	require.Equal(t, 765, usage.PromptTokensDetails.ImageTokens)
	require.NotNil(t, usage.InputTokensDetails)
	require.Equal(t, 765, usage.InputTokensDetails.ImageTokens)
	require.Equal(t, 1730, usage.CompletionTokenDetails.ImageTokens)
	require.NotNil(t, usage.OutputTokensDetails)
	require.Equal(t, 1730, usage.OutputTokensDetails.ImageTokens)
}

func TestEnsureEstimatedImageUsageDoesNotOverrideRealUsage(t *testing.T) {
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-image-2"}
	info.SetEstimatePromptTokens(11)
	request := &dto.ImageRequest{Model: "gpt-image-2", Prompt: "edit this image"}
	usage := &dto.Usage{
		PromptTokens:     7,
		CompletionTokens: 13,
		TotalTokens:      20,
	}

	estimated := ensureEstimatedImageUsageIfMissing(info, request, usage)

	require.False(t, estimated)
	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 13, usage.CompletionTokens)
	require.Equal(t, 20, usage.TotalTokens)
}

func TestImageResponseBodyWithUsageInjectsUsageWhenMissing(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     11,
		CompletionTokens: 765,
		TotalTokens:      776,
		InputTokens:      11,
		OutputTokens:     765,
		InputTokensDetails: &dto.InputTokenDetails{
			TextTokens: 11,
		},
		OutputTokensDetails: &dto.OutputTokenDetails{
			ImageTokens: 765,
		},
		UsageSource: "estimated",
	}
	body := []byte(`{"data":[{"url":"https://file.apixo.ai/temp/out.png","b64_json":"","revised_prompt":""}],"created":1780475455}`)

	updated, injected := imageResponseBodyWithUsage(body, usage)

	require.True(t, injected)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(updated, &payload))
	require.Contains(t, payload, "usage")
	require.Contains(t, string(updated), `"output_tokens":765`)
	require.Contains(t, string(updated), `"image_tokens":765`)
	require.NotContains(t, string(updated), "prompt_tokens")
	require.NotContains(t, string(updated), "completion_tokens")
	require.NotContains(t, string(updated), "usage_source")
	require.NotContains(t, string(updated), "claude_cache_creation")
}

func TestImageResponseBodyWithUsageKeepsExistingUsage(t *testing.T) {
	body := []byte(`{"data":[],"usage":{"total_tokens":3}}`)

	updated, injected := imageResponseBodyWithUsage(body, &dto.Usage{TotalTokens: 10})

	require.False(t, injected)
	require.Nil(t, updated)
}
