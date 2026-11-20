package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kitreasoning "github.com/QuantumNous/new-api/relaykit/relayconvert/reasoning"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/reasoning"
)

// Apply the GPT-6.2 Chat rules from upstream #8375 without changing the
// existing rules for other model families or private deployment names.
func applyGPT62ChatCompatibility(info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) error {
	preserveSuffix := model_setting.ShouldPreserveThinkingSuffix(info.OriginModelName) ||
		model_setting.ShouldPreserveThinkingSuffix(info.UpstreamModelName)
	candidate := info.UpstreamModelName
	if info.ChannelType == constant.ChannelTypeOpenRouter && !preserveSuffix {
		// The existing OpenRouter conversion removes this alias and emits its
		// nested reasoning object after GPT-6.2 parameter normalization.
		candidate = strings.TrimSuffix(candidate, "-thinking")
	}
	effort, model := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(candidate)
	if preserveSuffix {
		effort, model = "", info.UpstreamModelName
	}
	if !dto.IsOpenAIGPT62Model(model) {
		return nil
	}
	if effort != "" {
		request.ReasoningEffort = effort
		request.Model = model
		info.UpstreamModelName = model
	}
	if !preserveSuffix {
		// A mapping can remove the client's suffix. Match upstream precedence:
		// client model suffix, mapped model suffix, explicit request effort.
		if originEffort, _ := reasoning.ParseOpenAIReasoningEffortFromModelSuffix(info.OriginModelName); originEffort != "" {
			effort = originEffort
			request.ReasoningEffort = originEffort
		}
	}
	if info.ChannelType == constant.ChannelTypeOpenRouter && effort != "" && len(request.Reasoning) > 0 {
		if err := mergeGPT62OpenRouterReasoningSuffix(request, effort); err != nil {
			return kitreasoning.AsClientError(err)
		}
	}
	if info.ChannelType != constant.ChannelTypeOpenRouter {
		info.SetReasoningEffort(relaycommon.ReasoningEffortFromRequest(request))
	}
	if request.MaxCompletionTokens == nil {
		request.MaxCompletionTokens = request.MaxTokens
	}
	request.MaxTokens = nil
	request.Temperature = nil
	request.TopP = nil
	request.LogProbs = nil
	request.TopLogProbs = nil
	if len(request.Messages) > 0 && request.Messages[0].Role == "system" {
		request.Messages[0].Role = "developer"
	}
	return nil
}

// A converted Claude request may already carry OpenRouter's richer reasoning
// object. Apply the suffix to that object before the generic adapter retains
// it and clears reasoning_effort, so the wire and recorded effort agree.
func mergeGPT62OpenRouterReasoningSuffix(request *dto.GeneralOpenAIRequest, effort string) error {
	intent, err := openRouterReasoningIntent(request.Reasoning)
	if err != nil {
		return err
	}
	suffix := kitreasoning.Intent{Mode: kitreasoning.ModeEnabled, Effort: kitreasoning.Effort(effort), Source: kitreasoning.SourceSuffix}
	if suffix.Effort == kitreasoning.EffortNone {
		suffix.Mode = kitreasoning.ModeDisabled
	}
	merged, _, err := kitreasoning.MergeExplicitAndSuffix(intent, suffix, request.Model)
	if err != nil {
		return err
	}
	// Preserve unknown fields and their JSON values, including explicit false
	// and large numbers, while replacing only the strength chosen by the suffix.
	var fields map[string]json.RawMessage
	if err := common.Unmarshal(request.Reasoning, &fields); err != nil {
		return fmt.Errorf("invalid reasoning config: %w", err)
	}
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	delete(fields, "max_tokens")
	if merged.Mode == kitreasoning.ModeDisabled || merged.Effort == kitreasoning.EffortNone {
		fields["enabled"] = json.RawMessage("false")
		delete(fields, "effort")
	} else {
		fields["enabled"] = json.RawMessage("true")
		fields["effort"], err = common.Marshal(merged.Effort)
		if err != nil {
			return err
		}
	}
	request.Reasoning, err = common.Marshal(fields)
	return err
}
