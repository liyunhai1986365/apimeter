package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestBuildClaudeUsageFromOpenAIUsagePreservesClaudeCacheDetails(t *testing.T) {
	usage := buildClaudeUsageFromOpenAIUsage(&dto.Usage{
		PromptTokens:     100,
		CompletionTokens: 20,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens:         30,
			CachedCreationTokens: 50,
		},
		ClaudeCacheCreation5mTokens: 10,
		ClaudeCacheCreation1hTokens: 20,
	})

	require.Equal(t, 100, usage.InputTokens)
	require.Equal(t, 20, usage.OutputTokens)
	require.Equal(t, 30, usage.CacheReadInputTokens)
	require.Equal(t, 50, usage.CacheCreationInputTokens)
	require.NotNil(t, usage.CacheCreation)
	require.Equal(t, 30, usage.CacheCreation.Ephemeral5mInputTokens)
	require.Equal(t, 20, usage.CacheCreation.Ephemeral1hInputTokens)
}
