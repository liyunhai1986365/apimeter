package claudemessages

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/relaykit/types"
)

type openRouterRequestReasoning struct {
	Enabled   *bool  `json:"enabled,omitempty"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens *int   `json:"max_tokens,omitempty"`
	Exclude   *bool  `json:"exclude,omitempty"`
}

func applyClaudeChatReasoning(ctx context.Context, request dto.ClaudeRequest, info convmeta.Meta, out *dto.GeneralOpenAIRequest) error {
	sourceModel := request.Model
	if info != nil && info.GetOriginModelName() != "" {
		sourceModel = info.GetOriginModelName()
	}
	effort := reasoning.ClaudeEffectiveEffort(&request, sourceModel)
	if convmeta.OptionsOf(info).OpenRouterDialect {
		if effort != "" {
			config := openRouterRequestReasoning{Enabled: kitutil.GetPointer(effort != "none")}
			if effort != "none" {
				config.Effort = effort
				if config.Effort == "max" {
					config.Effort = "xhigh"
				}
				if request.Thinking != nil && request.Thinking.Type == "enabled" && request.Thinking.BudgetTokens != nil {
					// OpenRouter accepts either an exact budget or effort. Preserve
					// the native manual budget instead of sending both controls.
					config.MaxTokens = request.Thinking.BudgetTokens
					config.Effort = ""
					effort = string(reasoning.EffortFromBudget(*config.MaxTokens))
				}
			}
			if request.Thinking != nil && request.Thinking.Display != "" {
				config.Exclude = kitutil.GetPointer(request.Thinking.Display == "omitted")
			}
			var err error
			out.Reasoning, err = kitutil.Marshal(config)
			if err != nil {
				return fmt.Errorf("failed to marshal reasoning: %w", err)
			}
			// The Responses pivot consumes the portable effort. The OpenRouter
			// chat adapter removes it after retaining the richer reasoning object.
			out.ReasoningEffort = effort
		}
	} else {
		explicit := request.Thinking != nil || request.GetEfforts() != ""
		var err error
		effort, err = claudeEffortForOpenAI(ctx, out.Model, effort, explicit)
		if err != nil {
			return err
		}
		out.ReasoningEffort = effort
	}
	if info != nil {
		info.SetReasoningEffort(effort)
	}
	return nil
}

// Keep the capability check bounded to established model IDs and their dated
// snapshots. Private deployments and later model families retain their effort.
func openAIEffortRange(model string) (lowest, highest string) {
	model = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "openai/")
	matches := func(family string) bool {
		if model == family {
			return true
		}
		suffix := strings.TrimPrefix(model, family+"-")
		if suffix == model || len(suffix) != 10 || suffix[4] != '-' || suffix[7] != '-' {
			return false
		}
		for index, digit := range suffix {
			if index != 4 && index != 7 && (digit < '0' || digit > '9') {
				return false
			}
		}
		return true
	}
	switch {
	case matches("o1"), matches("o3"), matches("o3-mini"), matches("o4-mini"):
		return "low", "high"
	case matches("gpt-5"), matches("gpt-5-mini"), matches("gpt-5-nano"):
		return "minimal", "high"
	case matches("gpt-5.1"):
		return "none", "high"
	case matches("gpt-5.2"):
		return "none", "xhigh"
	}
	return "", ""
}

func knownOpenAIReasoningTarget(model string) bool {
	lowest, _ := openAIEffortRange(model)
	return lowest != "" || reasoning.IsKnownNonReasoningOpenAIModel(model)
}

func claudeEffortForOpenAI(ctx context.Context, model, effort string, explicit bool) (string, error) {
	if effort == "" {
		return "", nil
	}
	if reasoning.IsKnownNonReasoningOpenAIModel(model) {
		if explicit && effort != "none" {
			convdiag.Add(ctx, types.ConversionDiagnostic{
				Code: "openai_reasoning_unsupported", Path: "thinking",
				Message:  fmt.Sprintf("target model %q does not support reasoning_effort; the requested thinking strength was omitted", model),
				Severity: types.ConversionDiagnosticWarning,
			})
		}
		return "", nil
	}
	lowest, highest := openAIEffortRange(model)
	if lowest == "" {
		return effort, nil
	}
	parsed, err := reasoning.ParseEffort(effort)
	if err != nil {
		return "", reasoning.AsClientError(err)
	}
	adjusted := string(parsed)
	switch parsed {
	case reasoning.EffortNone:
		adjusted = lowest
	case reasoning.EffortMinimal:
		if lowest != "minimal" {
			adjusted = "low"
		}
	case reasoning.EffortXHigh, reasoning.EffortMax:
		adjusted = highest
	}
	if adjusted != string(parsed) {
		convdiag.Add(ctx, types.ConversionDiagnostic{
			Code: "openai_reasoning_effort_adjusted", Path: "thinking",
			Message:  fmt.Sprintf("target model %q does not support reasoning effort %q; using %q", model, parsed, adjusted),
			Severity: types.ConversionDiagnosticWarning,
		})
	}
	return adjusted, nil
}
