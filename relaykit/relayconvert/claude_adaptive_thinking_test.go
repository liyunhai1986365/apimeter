package relayconvert

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeAdaptiveThinkingConversions(t *testing.T) {
	for _, format := range []string{"chat", "responses"} {
		for _, tc := range []struct {
			name, model, effort, wantModel, wantType, wantEffort string
			budget                                               int
			preserveSuffix                                       bool
			adapterDisabled                                      bool
		}{
			{name: "omitted", model: "claude-opus-5-5", wantModel: "claude-opus-5-5"},
			{name: "explicit low", model: "claude-opus-5-5", effort: "low", wantType: "adaptive", wantEffort: "low"},
			{name: "explicit high", model: "claude-opus-5-5", effort: "high", wantType: "adaptive", wantEffort: "high"},
			{name: "effort suffix", model: "claude-opus-5-5-high", wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "high"},
			{name: "thinking suffix", model: "claude-opus-5-5-thinking", wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "medium"},
			{name: "explicit effort wins suffix", model: "claude-opus-5-5-thinking", effort: "medium", wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "medium"},
			{name: "explicit effort wins effort suffix", model: "claude-opus-5-5-high", effort: "low", wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "low"},
			{name: "explicit effort wins none suffix", model: "claude-opus-5-5-none", effort: "high", wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "high"},
			{name: "explicit none wins effort suffix", model: "claude-sonnet-5-5-max", effort: "none", wantModel: "claude-sonnet-5-5", wantType: "between_tools", wantEffort: "high"},
			{name: "preserved suffix", model: "claude-opus-5-5-thinking", effort: "low", preserveSuffix: true, wantType: "adaptive", wantEffort: "low"},
			{name: "preserved effort suffix", model: "claude-opus-5-5-high", preserveSuffix: true, wantType: "adaptive", wantEffort: "high"},
			{name: "preserved none suffix", model: "claude-sonnet-5-5-none", preserveSuffix: true, wantType: "between_tools"},
			{name: "dated family", model: "claude-opus-5-5-20261028", effort: "high", wantType: "adaptive", wantEffort: "high"},
			{name: "opus 4.7", model: "claude-opus-4-7", effort: "high", wantType: "adaptive", wantEffort: "high"},
			{name: "opus 4.8", model: "claude-opus-4-8", effort: "medium", wantType: "adaptive", wantEffort: "medium"},
			{name: "opus 5 default", model: "claude-opus-5-thinking", wantModel: "claude-opus-5", wantType: "adaptive", wantEffort: "high"},
			{name: "sonnet 5 default", model: "claude-sonnet-5-thinking", wantModel: "claude-sonnet-5", wantType: "adaptive", wantEffort: "high"},
			{name: "sonnet 5.5 default", model: "claude-sonnet-5-5-thinking", wantModel: "claude-sonnet-5-5", wantType: "adaptive", wantEffort: "high"},
			{name: "haiku 5 default", model: "claude-haiku-5-thinking", wantModel: "claude-haiku-5", wantType: "adaptive", wantEffort: "medium"},
			{name: "haiku 5.5 default", model: "claude-haiku-5-5-thinking", wantModel: "claude-haiku-5-5", wantType: "adaptive", wantEffort: "medium"},
			{name: "fable default", model: "claude-fable-5-thinking", wantModel: "claude-fable-5", wantType: "adaptive", wantEffort: "high"},
			{name: "mythos default", model: "claude-mythos-5-thinking", wantModel: "claude-mythos-5", wantType: "adaptive", wantEffort: "high"},
			{name: "opus 5.5 none", model: "claude-opus-5-5", effort: "none", wantType: "adaptive", wantEffort: "low"},
			{name: "sonnet 5.5 none", model: "claude-sonnet-5-5", effort: "none", wantType: "between_tools"},
			{name: "haiku 5.5 none", model: "claude-haiku-5-5", effort: "none", wantType: "disabled"},
			{name: "opus 5 none", model: "claude-opus-5", effort: "none", wantType: "disabled"},
			{name: "fable none", model: "claude-fable-5", effort: "none", wantType: "adaptive", wantEffort: "low"},
			{name: "mythos none", model: "claude-mythos-5", effort: "none", wantType: "adaptive", wantEffort: "low"},
			{name: "mythos preview none", model: "claude-mythos-preview", effort: "none", wantType: "adaptive", wantEffort: "low"},
			{name: "none suffix", model: "claude-opus-5-5-none", wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "low"},
			{name: "nothinking suffix", model: "claude-sonnet-5-5-nothinking", wantModel: "claude-sonnet-5-5", wantType: "between_tools"},
			{name: "minimal maps to low", model: "claude-opus-5-5", effort: "minimal", wantType: "adaptive", wantEffort: "low"},
			{name: "xhigh supported", model: "claude-sonnet-5-5", effort: "xhigh", wantType: "adaptive", wantEffort: "xhigh"},
			{name: "xhigh maps to max", model: "claude-opus-4-6", effort: "xhigh", wantType: "adaptive", wantEffort: "max"},
			{name: "preview xhigh maps to max", model: "claude-mythos-preview", effort: "xhigh", wantType: "adaptive", wantEffort: "max"},
			{name: "manual max maps to high", model: "claude-opus-4-5", effort: "max", wantType: "enabled", wantEffort: "high", budget: 4096},
			{name: "disabled adapter preserves thinking suffix", model: "claude-opus-5-5-thinking", adapterDisabled: true},
			{name: "disabled adapter preserves explicit effort", model: "claude-opus-5-5-thinking", adapterDisabled: true, effort: "low", wantType: "adaptive", wantEffort: "low"},
			{name: "disabled adapter keeps effort suffix behavior", model: "claude-opus-5-5-high", adapterDisabled: true, wantModel: "claude-opus-5-5", wantType: "adaptive", wantEffort: "high"},
			{name: "legacy model", model: "claude-sonnet-4-5", effort: "high", wantType: "enabled", budget: 4096},
			{name: "legacy none", model: "claude-sonnet-4-5", effort: "none", wantType: "disabled"},
			{name: "family boundary", model: "claude-opus-5-50", effort: "medium", wantType: "enabled", budget: 2048},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				meta := &convmeta.Values{Options: &convmeta.Options{
					Claude:                 convmeta.ClaudeOptions{ThinkingAdapterEnabled: !tc.adapterDisabled, ThinkingAdapterBudgetTokensPercentage: 0.8},
					PreserveThinkingSuffix: func(string) bool { return tc.preserveSuffix },
				}}
				var got *dto.ClaudeRequest
				var err error
				if format == "chat" {
					got, err = OpenAIChatRequestToClaudeMessages(context.Background(), meta, dto.GeneralOpenAIRequest{
						Model: tc.model, ReasoningEffort: tc.effort, MaxTokens: kitutil.GetPointer[uint](8192),
						Messages: []dto.Message{{Role: "user", Content: "hello"}},
					})
				} else {
					req := &dto.OpenAIResponsesRequest{Model: tc.model, MaxOutputTokens: kitutil.GetPointer[uint](8192), Input: json.RawMessage(`"hello"`)}
					if tc.effort != "" {
						req.Reasoning = &dto.Reasoning{Effort: tc.effort}
					}
					got, err = OpenAIResponsesRequestToClaudeMessages(context.Background(), meta, req)
				}
				require.NoError(t, err)
				wantModel := tc.wantModel
				if wantModel == "" {
					wantModel = tc.model
				}
				require.Equal(t, wantModel, got.Model)
				body, err := kitutil.Marshal(got)
				require.NoError(t, err)
				require.Equal(t, tc.wantType, gjson.GetBytes(body, "thinking.type").String())
				require.Equal(t, tc.wantEffort, gjson.GetBytes(body, "output_config.effort").String())
				if tc.budget > 0 {
					require.Equal(t, int64(tc.budget), gjson.GetBytes(body, "thinking.budget_tokens").Int())
				} else {
					require.False(t, gjson.GetBytes(body, "thinking.budget_tokens").Exists())
				}
				if tc.wantType == "" {
					require.False(t, gjson.GetBytes(body, "thinking").Exists())
				}
			})
		}
	}
}

func TestClaudeAdaptiveThinkingBudgetDoesNotOverrideEffort(t *testing.T) {
	for _, tc := range []struct {
		model, effort, wantEffort string
		wantBudget                int
	}{
		{"claude-opus-5-5", "", "medium", 0},
		{"claude-opus-5-5", "low", "low", 0},
		{"claude-opus-5-5-high", "medium", "medium", 0},
		{"claude-opus-4-7-high", "low", "low", 0},
		{"claude-opus-4-6-high", "medium", "medium", 0},
		{"claude-sonnet-4-5", "low", "", 6000},
	} {
		t.Run(tc.model+"/"+tc.effort, func(t *testing.T) {
			got, err := OpenAIChatRequestToClaudeMessages(context.Background(), nil, dto.GeneralOpenAIRequest{
				Model: tc.model, ReasoningEffort: tc.effort, Reasoning: json.RawMessage(`{"max_tokens":6000}`),
				MaxTokens: kitutil.GetPointer[uint](8192), Messages: []dto.Message{{Role: "user", Content: "hello"}},
			})
			require.NoError(t, err)
			require.Equal(t, tc.wantBudget, got.Thinking.GetBudgetTokens())
			require.Equal(t, tc.wantEffort, gjson.GetBytes(got.OutputConfig, "effort").String())
			if tc.wantBudget == 0 {
				require.Equal(t, "adaptive", got.Thinking.Type)
			}
		})
	}
}

func TestResponsesClaudeEffortSuffixPreservation(t *testing.T) {
	const model = "claude-opus-4-7-high"
	for _, tc := range []struct {
		name, preservedModel, effort, wantModel, wantEffort string
	}{
		{name: "default strips suffix", wantModel: "claude-opus-4-7", wantEffort: "high"},
		{name: "unlisted model strips suffix", preservedModel: "claude-opus-4-7-low", wantModel: "claude-opus-4-7", wantEffort: "high"},
		{name: "listed model keeps suffix", preservedModel: model, wantModel: model, wantEffort: "high"},
		{name: "explicit effort wins with preserved suffix", preservedModel: model, effort: "low", wantModel: model, wantEffort: "low"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &convmeta.Options{}
			if tc.preservedModel != "" {
				opts.PreserveThinkingSuffix = func(modelName string) bool {
					return modelName == tc.preservedModel
				}
			}
			req := &dto.OpenAIResponsesRequest{
				Model: model, MaxOutputTokens: kitutil.GetPointer[uint](8192), Input: json.RawMessage(`"hello"`),
				Temperature: kitutil.GetPointer(0.5), TopP: kitutil.GetPointer(0.9),
			}
			if tc.effort != "" {
				req.Reasoning = &dto.Reasoning{Effort: tc.effort}
			}
			got, err := OpenAIResponsesRequestToClaudeMessages(context.Background(), &convmeta.Values{Options: opts}, req)
			require.NoError(t, err)
			require.Equal(t, tc.wantModel, got.Model)
			require.NotNil(t, got.Thinking)
			require.Equal(t, "adaptive", got.Thinking.Type)
			require.Equal(t, tc.wantEffort, gjson.GetBytes(got.OutputConfig, "effort").String())
			require.Nil(t, got.Temperature)
			require.Nil(t, got.TopP)
		})
	}
}
