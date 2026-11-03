package reasoning

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

func claudeModelFamily(model, family string) bool {
	return model == family || strings.HasPrefix(model, family+"-")
}

// ClaudeSupportsAdaptiveThinking lists known families; do not infer capabilities
// for unrelated aliases or future versions from a numeric version comparison.
func ClaudeSupportsAdaptiveThinking(model string) bool {
	return claudeModelFamily(model, "claude-opus-4-6") || ClaudeRequiresAdaptiveThinking(model)
}

func ClaudeRequiresAdaptiveThinking(model string) bool {
	return claudeModelFamily(model, "claude-opus-4-7") ||
		claudeModelFamily(model, "claude-opus-4-8") ||
		claudeModelFamily(model, "claude-opus-5-5")
}

// SetClaudeAdaptiveThinking preserves structured-output and other output_config
// fields. An explicit effort takes precedence; otherwise retain the caller's
// effort, falling back to the existing thinking adapter's "high" default.
// A token budget has no exact adaptive-effort equivalent.
func SetClaudeAdaptiveThinking(request *dto.ClaudeRequest, effort string) error {
	config := make(map[string]json.RawMessage)
	if len(request.OutputConfig) > 0 {
		if err := kitutil.Unmarshal(request.OutputConfig, &config); err != nil {
			return fmt.Errorf("invalid Claude output_config: %w", err)
		}
		if config == nil {
			config = make(map[string]json.RawMessage)
		}
	}
	if effort == "" {
		if _, exists := config["effort"]; !exists {
			effort = "high"
		}
	}
	if effort != "" {
		value, err := kitutil.Marshal(effort)
		if err != nil {
			return err
		}
		config["effort"] = value
	}
	outputConfig, err := kitutil.Marshal(config)
	if err != nil {
		return err
	}
	thinking := dto.Thinking{Type: "adaptive"}
	if request.Thinking != nil {
		thinking.Display = request.Thinking.Display
	}
	request.Thinking = &thinking
	request.OutputConfig = outputConfig
	if claudeModelFamily(request.Model, "claude-opus-4-7") || claudeModelFamily(request.Model, "claude-opus-4-8") {
		if request.Thinking.Display == "" {
			request.Thinking.Display = "summarized"
		}
		request.Temperature = nil
		request.TopP = nil
		request.TopK = nil
	}
	return nil
}

// NormalizeClaudeThinking also handles native Messages requests. Keep omitted
// thinking omitted. Opus 5.5 accepts omission rather than the disabled variant.
func NormalizeClaudeThinking(request *dto.ClaudeRequest) error {
	if request.Thinking == nil || !ClaudeRequiresAdaptiveThinking(request.Model) {
		return nil
	}
	switch request.Thinking.Type {
	case "enabled", "adaptive":
		return SetClaudeAdaptiveThinking(request, "")
	case "disabled":
		if claudeModelFamily(request.Model, "claude-opus-5-5") {
			request.Thinking = nil
		}
	}
	return nil
}

func ApplyClaudeReasoningEffort(request *dto.ClaudeRequest, effort string) error {
	switch effort {
	case "low", "medium", "high", "minimal", "xhigh", "max":
		if ClaudeSupportsAdaptiveThinking(request.Model) {
			return SetClaudeAdaptiveThinking(request, effort)
		}
	}
	var budget int
	switch effort {
	case "low":
		budget = 1280
	case "medium":
		budget = 2048
	case "high":
		budget = 4096
	default:
		return nil
	}
	request.Thinking = &dto.Thinking{Type: "enabled", BudgetTokens: &budget}
	return nil
}

func ApplyClaudeThinkingBudget(request *dto.ClaudeRequest, budget int) error {
	if budget <= 0 {
		return nil
	}
	if ClaudeRequiresAdaptiveThinking(request.Model) ||
		(request.Thinking != nil && request.Thinking.Type == "adaptive") {
		return SetClaudeAdaptiveThinking(request, "")
	}
	request.Thinking = &dto.Thinking{Type: "enabled", BudgetTokens: &budget}
	return nil
}
