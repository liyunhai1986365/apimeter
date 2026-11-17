package reasoning

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
)

func TestClaudeAdaptiveFamilyDefaultsAndExtensions(t *testing.T) {
	for _, tc := range []struct{ model, effort string }{
		{"claude-opus-5-5", "medium"}, {"claude-haiku-5-5", "medium"},
		{"claude-sonnet-5-5", "high"}, {"claude-opus-5", "high"},
		{"claude-opus-4-7", "high"}, {"claude-fable-5", "high"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var req dto.ClaudeRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"thinking":{"type":"enabled","budget_tokens":8192,"display":"omitted","block_binding":{"type":"test"},"future":{"n":9007199254740993}},"output_config":{"format":{"type":"json_schema","schema":{"const":9007199254740993}}},"temperature":0,"top_p":0,"top_k":0}`, &req))
			req.Model = tc.model
			require.NoError(t, NormalizeClaudeThinking(&req))
			require.Equal(t, "adaptive", req.Thinking.Type)
			require.Nil(t, req.Thinking.BudgetTokens)
			require.Equal(t, tc.effort, req.GetEfforts())
			require.Equal(t, "omitted", req.Thinking.Display)
			require.JSONEq(t, `{"type":"test"}`, string(req.Thinking.BlockBinding))
			require.JSONEq(t, `{"n":9007199254740993}`, string(req.Thinking.Extra["future"]))
			require.Contains(t, string(req.OutputConfig), "9007199254740993")
			require.Nil(t, req.Temperature)
			require.Nil(t, req.TopP)
			require.Nil(t, req.TopK)
		})
	}
}

func TestClaudeNativeOmissionAndExplicitEffort(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-haiku-5-5", "claude-sonnet-5-5"} {
		t.Run(model, func(t *testing.T) {
			req := &dto.ClaudeRequest{Model: model}
			require.NoError(t, NormalizeClaudeThinking(req))
			require.Nil(t, req.Thinking)
			require.Empty(t, req.OutputConfig)
			want := "medium"
			if model == "claude-sonnet-5-5" {
				want = "high"
			}
			require.Equal(t, want, ClaudeEffectiveEffort(req, model))
			req.Thinking = &dto.Thinking{Type: "adaptive"}
			require.NoError(t, NormalizeClaudeThinking(req))
			require.Empty(t, req.OutputConfig, "native adaptive omission must not synthesize an output effort")
			require.Equal(t, want, ClaudeEffectiveEffort(req, model))
			req.OutputConfig = []byte(`{"effort":"high"}`)
			require.NoError(t, NormalizeClaudeThinking(req))
			require.Equal(t, "high", req.GetEfforts())
			require.Equal(t, "high", ClaudeEffectiveEffort(req, model))
		})
	}
}

func TestClaudeEffortOnlyCapabilityMapping(t *testing.T) {
	for _, tc := range []struct{ model, requested, want string }{
		{"claude-opus-4-5", "max", "high"}, {"claude-opus-4-5", "xhigh", "high"},
		{"claude-opus-4-6", "xhigh", "max"}, {"claude-sonnet-4-6", "minimal", "low"},
		{"claude-sonnet-4-5", "high", ""}, {"claude-haiku-4-5", "high", ""},
		{"claude-mythos-preview", "xhigh", "max"}, {"claude-fable-5", "max", "max"},
		{"private-model", "xhigh", "xhigh"}, {"claude-opus-5-50", "max", "max"},
	} {
		t.Run(tc.model+"/"+tc.requested, func(t *testing.T) {
			req := &dto.ClaudeRequest{Model: tc.model, OutputConfig: []byte(`{"effort":"` + tc.requested + `","format":{"type":"json_schema","schema":{"const":9007199254740993}}}`)}
			require.NoError(t, NormalizeClaudeThinking(req))
			require.Nil(t, req.Thinking)
			require.Equal(t, tc.want, req.GetEfforts())
			require.Contains(t, string(req.OutputConfig), "9007199254740993")
		})
	}
}

func TestClaudeThinkingOffModesAndEffort(t *testing.T) {
	for _, tc := range []struct{ model, kind, effort string }{
		{"claude-opus-5-5", "adaptive", "low"}, {"claude-sonnet-5-5", "between_tools", "high"},
		{"claude-haiku-5-5", "disabled", "high"}, {"claude-opus-5", "disabled", "high"},
		{"claude-sonnet-5", "disabled", "max"}, {"claude-opus-4-8", "disabled", "max"},
		{"claude-fable-5", "adaptive", "low"}, {"claude-mythos-5", "adaptive", "low"},
		{"claude-mythos-preview", "adaptive", "low"}, {"claude-opus-4-5", "disabled", "high"},
		{"claude-sonnet-4-5", "disabled", ""},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var req dto.ClaudeRequest
			require.NoError(t, kitutil.UnmarshalJsonStr(`{"thinking":{"type":"enabled","budget_tokens":2048,"display":"omitted","block_binding":{"x":1},"future":true},"output_config":{"effort":"max","format":{"x":1}}}`, &req))
			req.Model = tc.model
			require.NoError(t, ApplyClaudeReasoningEffort(&req, "none"))
			require.Equal(t, tc.kind, req.Thinking.Type)
			require.Equal(t, tc.effort, req.GetEfforts())
			require.Nil(t, req.Thinking.BudgetTokens)
			if tc.kind != "adaptive" {
				encoded, err := kitutil.Marshal(req.Thinking)
				require.NoError(t, err)
				require.JSONEq(t, `{"type":"`+tc.kind+`"}`, string(encoded))
				require.Equal(t, "none", ClaudeEffectiveEffort(&req, tc.model))
			}
		})
	}
}

func TestClaudeUnknownModelKeepsNativeThinking(t *testing.T) {
	for _, model := range []string{"private-model", "claude-opus-5-50", "claude-haiku-5-6"} {
		var req dto.ClaudeRequest
		require.NoError(t, kitutil.UnmarshalJsonStr(`{"thinking":{"type":"enabled","budget_tokens":4096,"future":false},"output_config":{"effort":"future"},"temperature":0}`, &req))
		req.Model = model
		before, err := kitutil.Marshal(req)
		require.NoError(t, err)
		require.NoError(t, NormalizeClaudeThinking(&req))
		after, err := kitutil.Marshal(req)
		require.NoError(t, err)
		require.Equal(t, string(before), string(after))
	}
}

func TestClaudeManualModelAndSuffixBoundaries(t *testing.T) {
	for _, model := range []string{"claude-opus-4-6", "claude-sonnet-4-6", "claude-sonnet-4-5"} {
		req := &dto.ClaudeRequest{Model: model, Thinking: &dto.Thinking{Type: "enabled", BudgetTokens: kitutil.GetPointer(4096)}}
		require.NoError(t, NormalizeClaudeThinking(req))
		require.Equal(t, "enabled", req.Thinking.Type)
		require.Equal(t, 4096, *req.Thinking.BudgetTokens)
	}
	for _, model := range []string{"claude-opus-5-50", "claude-opus-5-6", "claude-haiku-5-6"} {
		require.False(t, ClaudeRequiresAdaptiveThinking(model), model)
	}
	require.True(t, ClaudeRequiresAdaptiveThinking("claude-opus-5-5-20261001"))
	require.True(t, ClaudeRequiresAdaptiveThinking("claude-opus-5-20261001"))
	base, effort, ok := TrimClaudeEffortSuffix("claude-sonnet-5-5-nothinking")
	require.True(t, ok)
	require.Equal(t, "claude-sonnet-5-5", base)
	require.Equal(t, "none", effort)
}
