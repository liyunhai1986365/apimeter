package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAdaptNativeClaudeThinking(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	oldEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = true
	t.Cleanup(func() { settings.ThinkingAdapterEnabled = oldEnabled })
	for _, tc := range []struct {
		name, model                     string
		thinking                        *dto.Thinking
		config                          json.RawMessage
		wantType, wantEffort, wantModel string
		wantBudget                      int
	}{
		{name: "omitted", model: "claude-opus-5-5"},
		{name: "legacy enabled", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, wantType: "adaptive", wantEffort: "high"},
		{name: "preserve config", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(8192)}, config: json.RawMessage(`{"effort":"low","format":{"type":"json_schema","schema":{"type":"object","properties":{"n":{"const":9007199254740993}}}}}`), wantType: "adaptive", wantEffort: "low"},
		{name: "adaptive budget removed", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "adaptive", BudgetTokens: common.GetPointer(0), Display: "omitted"}, config: json.RawMessage(`{"effort":"medium"}`), wantType: "adaptive", wantEffort: "medium"},
		{name: "disabled uses omission", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "disabled"}},
		{name: "thinking suffix", model: "claude-opus-5-5-thinking", wantType: "adaptive", wantEffort: "high", wantModel: "claude-opus-5-5"},
		{name: "thinking suffix with explicit budget", model: "claude-opus-5-5-thinking", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(8192)}, wantType: "adaptive", wantEffort: "high", wantModel: "claude-opus-5-5"},
		{name: "effort suffix", model: "claude-opus-5-5-high", config: json.RawMessage(`{"format":{"type":"json_schema","schema":{"type":"object"}}}`), wantType: "adaptive", wantEffort: "high", wantModel: "claude-opus-5-5"},
		{name: "legacy model unchanged", model: "claude-sonnet-4-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, wantType: "enabled", wantBudget: 4096},
		{name: "opus 4.6 manual unchanged", model: "claude-opus-4-6", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, wantType: "enabled", wantBudget: 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &dto.ClaudeRequest{Model: tc.model, MaxTokens: common.GetPointer[uint](16384), Thinking: tc.thinking, OutputConfig: tc.config, Temperature: common.GetPointer(0.0)}
			require.NoError(t, adaptClaudeThinking(req, tc.model))
			wantModel := tc.wantModel
			if wantModel == "" {
				wantModel = tc.model
			}
			require.Equal(t, wantModel, req.Model)
			body, err := common.Marshal(req)
			require.NoError(t, err)
			require.Equal(t, tc.wantType, gjson.GetBytes(body, "thinking.type").String())
			require.Equal(t, tc.wantEffort, gjson.GetBytes(body, "output_config.effort").String())
			if tc.wantBudget > 0 {
				require.Equal(t, int64(tc.wantBudget), gjson.GetBytes(body, "thinking.budget_tokens").Int())
			} else {
				require.False(t, gjson.GetBytes(body, "thinking.budget_tokens").Exists())
			}
			if tc.wantType == "" {
				require.False(t, gjson.GetBytes(body, "thinking").Exists())
			}
			if gjson.GetBytes(tc.config, "format").Exists() {
				require.JSONEq(t, gjson.GetBytes(tc.config, "format").Raw, gjson.GetBytes(body, "output_config.format").Raw)
			}
			if tc.name == "preserve config" {
				require.Equal(t, "9007199254740993", gjson.GetBytes(body, "output_config.format.schema.properties.n.const").Raw)
			}
			if tc.thinking != nil && tc.thinking.Display != "" {
				require.Equal(t, tc.thinking.Display, req.Thinking.Display)
			}
			// No evidence that Opus 5.5 shares 4.7's sampling restriction.
			require.Equal(t, 0.0, *req.Temperature)
		})
	}
}

func TestAdaptNativeClaudeThinkingRejectsInvalidOutputConfig(t *testing.T) {
	req := &dto.ClaudeRequest{Model: "claude-opus-5-5", Thinking: &dto.Thinking{Type: "enabled"}, OutputConfig: json.RawMessage(`[]`)}
	require.ErrorContains(t, adaptClaudeThinking(req, req.Model), "output_config")
}
