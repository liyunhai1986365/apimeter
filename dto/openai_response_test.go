package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestOpenAITextResponseMarshalUsesChatCompletionUsageSchema(t *testing.T) {
	response := OpenAITextResponse{
		Id:      "chatcmpl-test",
		Model:   "gpt-test",
		Object:  "chat.completion",
		Created: 1,
		Choices: []OpenAITextResponseChoice{},
		Usage: Usage{
			PromptTokens:     100,
			CompletionTokens: 20,
			TotalTokens:      120,
			PromptTokensDetails: InputTokenDetails{
				CachedTokens:         80,
				CachedCreationTokens: 12,
				CacheWriteTokens:     8,
				TextTokens:           15,
				AudioTokens:          2,
				ImageTokens:          3,
			},
			CompletionTokenDetails: OutputTokenDetails{
				TextTokens:               10,
				AudioTokens:              1,
				ImageTokens:              4,
				ReasoningTokens:          5,
				AcceptedPredictionTokens: 6,
				RejectedPredictionTokens: 7,
			},
			InputTokens:                 100,
			OutputTokens:                20,
			InputTokensDetails:          &InputTokenDetails{CachedTokens: 80},
			OutputTokensDetails:         &OutputTokenDetails{ReasoningTokens: 5},
			ClaudeCacheCreation5mTokens: 12,
			ClaudeCacheCreation1hTokens: 6,
		},
	}

	body, err := common.Marshal(response)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(body, &payload))
	usage := payload["usage"].(map[string]any)

	require.EqualValues(t, 100, usage["prompt_tokens"])
	require.EqualValues(t, 20, usage["completion_tokens"])
	require.EqualValues(t, 120, usage["total_tokens"])
	require.NotContains(t, usage, "input_tokens")
	require.NotContains(t, usage, "output_tokens")
	require.NotContains(t, usage, "input_tokens_details")
	require.NotContains(t, usage, "output_tokens_details")
	require.NotContains(t, usage, "claude_cache_creation_5_m_tokens")
	require.NotContains(t, usage, "claude_cache_creation_1_h_tokens")

	promptDetails := usage["prompt_tokens_details"].(map[string]any)
	require.EqualValues(t, 80, promptDetails["cached_tokens"])
	require.EqualValues(t, 2, promptDetails["audio_tokens"])
	require.EqualValues(t, 8, promptDetails["cache_write_tokens"])
	require.NotContains(t, promptDetails, "text_tokens")
	require.NotContains(t, promptDetails, "image_tokens")
	require.NotContains(t, promptDetails, "cached_creation_tokens")

	completionDetails := usage["completion_tokens_details"].(map[string]any)
	require.EqualValues(t, 5, completionDetails["reasoning_tokens"])
	require.EqualValues(t, 1, completionDetails["audio_tokens"])
	require.EqualValues(t, 6, completionDetails["accepted_prediction_tokens"])
	require.EqualValues(t, 7, completionDetails["rejected_prediction_tokens"])
	require.NotContains(t, completionDetails, "text_tokens")
	require.NotContains(t, completionDetails, "image_tokens")
}

func TestChatCompletionsStreamResponseMarshalUsesChatCompletionUsageSchema(t *testing.T) {
	response := ChatCompletionsStreamResponse{
		Id:      "chatcmpl-test",
		Object:  "chat.completion.chunk",
		Created: 1,
		Model:   "gpt-test",
		Choices: []ChatCompletionsStreamResponseChoice{},
		Usage: &Usage{
			PromptTokens:                10,
			CompletionTokens:            2,
			TotalTokens:                 12,
			InputTokens:                 10,
			OutputTokens:                2,
			ClaudeCacheCreation5mTokens: 4,
			ClaudeCacheCreation1hTokens: 3,
		},
	}

	body, err := common.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(body), "input_tokens")
	require.NotContains(t, string(body), "output_tokens")
	require.NotContains(t, string(body), "claude_cache_creation")
	require.Contains(t, string(body), `"prompt_tokens":10`)
}
