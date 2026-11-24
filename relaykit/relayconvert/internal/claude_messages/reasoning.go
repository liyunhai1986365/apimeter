package claudemessages

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
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
		effort, err = reasoning.EffortForOpenAITarget(ctx, out.Model, effort, "thinking", explicit)
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
