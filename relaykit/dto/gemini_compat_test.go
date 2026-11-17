package dto

import (
	"testing"

	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
)

func TestGeminiNewFieldsRoundTrip(t *testing.T) {
	var request GeminiChatRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{
	 "continuation_token":"resume", "contents":[{"parts":[{"media_processing":{"mode":"test"},"functionCall":{"id":"fc_1","name":"f","partialArgs":[{"jsonPath":"$.n","numberValue":0},{"jsonPath":"$.ok","boolValue":false}],"willContinue":false}}]}],
	 "generationConfig":{"seed":0,"media_processing":{"fps":0},"thinking_config":{"include_thoughts":false,"thinking_budget":0}},
	 "toolConfig":{"functionCallingConfig":{"streamFunctionCallArguments":false}},
	 "tools":[{"googleMaps":{},"enterpriseWebSearch":{},"fileSearch":{"fileSearchStoreNames":["store"]},"computerUse":{"environment":"ENVIRONMENT_BROWSER"},"retrieval":{}}]
	}`, &request))
	encoded, err := kitutil.Marshal(request)
	require.NoError(t, err)
	for _, field := range []string{`"continuationToken":"resume"`, `"includeThoughts":false`, `"thinkingBudget":0`, `"seed":0`, `"mediaProcessing":{"fps":0}`, `"streamFunctionCallArguments":false`, `"numberValue":0`, `"boolValue":false`, `"willContinue":false`} {
		require.Contains(t, string(encoded), field)
	}
	tools := request.GetTools()
	require.Len(t, tools, 1)
	require.JSONEq(t, `{}`, string(tools[0].GoogleMaps))
	require.NotEmpty(t, tools[0].EnterpriseWebSearch)
	require.NotEmpty(t, tools[0].FileSearch)
	require.NotEmpty(t, tools[0].ComputerUse)
	require.NotEmpty(t, tools[0].Retrieval)
	var absent GeminiThinkingConfig
	encoded, err = kitutil.Marshal(absent)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(encoded))
}

func TestGeminiUsageSnapshotsKeepOmittedFieldsAndReplaceModalities(t *testing.T) {
	first := &GeminiUsageMetadata{PromptTokenCount: 100, CachedContentTokenCount: 30, ThoughtsTokenCount: 7,
		PromptTokensDetails:     []GeminiPromptTokensDetails{{Modality: "AUDIO", TokenCount: 10}, {Modality: "TEXT", TokenCount: 90}},
		CandidatesTokensDetails: []GeminiPromptTokensDetails{{Modality: "IMAGE", TokenCount: 50}},
	}
	first.BillingUsage = NewGeminiChatBillingUsage(first)
	next := &GeminiUsageMetadata{CandidatesTokenCount: 80,
		PromptTokensDetails:     []GeminiPromptTokensDetails{{Modality: "AUDIO", TokenCount: 20}},
		CandidatesTokensDetails: []GeminiPromptTokensDetails{{Modality: "IMAGE", TokenCount: 60}, {Modality: "IMAGE", TokenCount: 10}},
	}
	merged := MergeGeminiUsageMetadataNonZero(first, next)
	require.Equal(t, 100, merged.PromptTokenCount)
	require.Equal(t, 30, merged.CachedContentTokenCount)
	require.Equal(t, 7, merged.ThoughtsTokenCount)
	require.Equal(t, 187, merged.TotalTokenCount)
	require.Equal(t, []GeminiPromptTokensDetails{{Modality: "TEXT", TokenCount: 90}, {Modality: "AUDIO", TokenCount: 20}}, merged.PromptTokensDetails)
	require.Equal(t, next.CandidatesTokensDetails, merged.CandidatesTokensDetails)
	require.Equal(t, 80, merged.BillingUsage.GeminiUsageMetadata.CandidatesTokenCount)
	require.Equal(t, 10, first.PromptTokensDetails[0].TokenCount)
	require.Equal(t, 0, first.CandidatesTokenCount)
	repeated := MergeGeminiUsageMetadataNonZero(merged, next)
	require.Equal(t, merged, repeated)
}

func TestFunctionSchemaOrderSurvivesCopyAndHonorsEdits(t *testing.T) {
	var function FunctionRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"name":"f","parameters":{"type":"object","properties":{"z":{"const":1},"a":{"const":2}}}}`, &function))
	encoded, err := kitutil.Marshal(function)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"properties":{"z":{"const":1},"a":{"const":2}}`)
	function.Parameters = map[string]any{"const": 3}
	encoded, err = kitutil.Marshal(function)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"parameters":{"const":3}`)
}
