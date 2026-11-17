package reasoning

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

type claudeCapabilities struct {
	known, adaptive, requiresAdaptive, defaultThinking bool
	supportsEffort, supportsXHigh, supportsMax         bool
	cannotDisable, disableAsBetweenTools               bool
	offEffortAtMostHigh, strictSampling                bool
	defaultEffort                                      Effort
}

func claudeModelFamily(model, family string) bool {
	if model == family {
		return true
	}
	if !strings.HasPrefix(model, family+"-") {
		return false
	}
	suffix := strings.TrimPrefix(model, family+"-")
	if suffix != "" && suffix[0] >= '0' && suffix[0] <= '9' {
		date, _, _ := strings.Cut(suffix, "-")
		if len(date) != 8 {
			return false
		}
		for _, digit := range date {
			if digit < '0' || digit > '9' {
				return false
			}
		}
	}
	return true
}

// Match only known versions (plus date/deployment suffixes). In particular,
// Opus 5.50 must not inherit the rules for Opus 5.5 or Opus 5.
func claudeCapabilitiesFor(model string) claudeCapabilities {
	model = strings.ToLower(model)
	caps := claudeCapabilities{defaultEffort: EffortHigh}
	family := func(name string) bool { return claudeModelFamily(model, name) }
	switch {
	case family("claude-fable-5"), family("claude-mythos-5"):
		caps = claudeCapabilities{known: true, adaptive: true, requiresAdaptive: true, defaultThinking: true, cannotDisable: true, supportsEffort: true, supportsXHigh: true, supportsMax: true, strictSampling: true, defaultEffort: EffortHigh}
	case family("claude-mythos-preview"):
		caps = claudeCapabilities{known: true, adaptive: true, defaultThinking: true, cannotDisable: true, supportsEffort: true, supportsMax: true, strictSampling: true, defaultEffort: EffortHigh}
	case family("claude-opus-5-5"), family("claude-sonnet-5-5"), family("claude-haiku-5-5"):
		caps = claudeCapabilities{known: true, adaptive: true, requiresAdaptive: true, defaultThinking: true, supportsEffort: true, supportsXHigh: true, supportsMax: true, strictSampling: true, defaultEffort: EffortHigh}
		caps.cannotDisable = !family("claude-haiku-5-5")
		caps.disableAsBetweenTools = family("claude-sonnet-5-5")
		caps.offEffortAtMostHigh = !family("claude-opus-5-5")
		if !family("claude-sonnet-5-5") {
			caps.defaultEffort = EffortMedium
		}
	case family("claude-opus-5"), family("claude-sonnet-5"), family("claude-haiku-5"):
		caps = claudeCapabilities{known: true, adaptive: true, requiresAdaptive: true, defaultThinking: true, supportsEffort: true, supportsXHigh: true, supportsMax: true, strictSampling: true, defaultEffort: EffortHigh}
		caps.offEffortAtMostHigh = !family("claude-sonnet-5")
		if family("claude-haiku-5") {
			caps.defaultEffort = EffortMedium
		}
	case family("claude-opus-4-7"), family("claude-opus-4-8"):
		caps = claudeCapabilities{known: true, adaptive: true, requiresAdaptive: true, supportsEffort: true, supportsXHigh: true, supportsMax: true, strictSampling: true, defaultEffort: EffortHigh}
	case family("claude-opus-4-6"), family("claude-sonnet-4-6"):
		caps.known, caps.adaptive, caps.supportsEffort, caps.supportsMax = true, true, true, true
	case family("claude-opus-4-5"):
		caps.known, caps.supportsEffort = true, true
	case family("claude-sonnet-4-5"), family("claude-haiku-4-5"), family("claude-opus-4-1"),
		family("claude-opus-4"), family("claude-sonnet-4"), family("claude-3-7-sonnet"),
		family("claude-3-5-sonnet"), family("claude-3-5-haiku"), family("claude-3-opus"),
		family("claude-3-sonnet"), family("claude-3-haiku"):
		caps.known = true
	}
	return caps
}

func IsKnownClaudeModel(model string) bool             { return claudeCapabilitiesFor(model).known }
func ClaudeSupportsAdaptiveThinking(model string) bool { return claudeCapabilitiesFor(model).adaptive }
func ClaudeRequiresAdaptiveThinking(model string) bool {
	return claudeCapabilitiesFor(model).requiresAdaptive
}
func ClaudeDefaultEffort(model string) Effort { return claudeCapabilitiesFor(model).defaultEffort }

// ClaudeEffort maps portable effort onto the model's supported output_config
// values. Unknown deployment names keep their explicit effort unchanged.
func ClaudeEffort(model string, effort Effort) Effort {
	caps := claudeCapabilitiesFor(model)
	if !caps.known {
		return effort
	}
	if !caps.supportsEffort || effort == EffortNone {
		return ""
	}
	switch effort {
	case EffortMinimal:
		return EffortLow
	case EffortXHigh:
		if !caps.supportsXHigh {
			if caps.supportsMax {
				return EffortMax
			}
			return EffortHigh
		}
	case EffortMax:
		if !caps.supportsMax {
			return EffortHigh
		}
	}
	return effort
}

// ClaudeEffectiveEffort records the model default without adding request fields.
// model is the source family when a mapped request is converted to another API.
func ClaudeEffectiveEffort(request *dto.ClaudeRequest, model string) string {
	if request == nil {
		return ""
	}
	if request.Thinking != nil {
		switch request.Thinking.Type {
		case "disabled", "between_tools":
			return string(EffortNone)
		}
	}
	if effort := request.GetEfforts(); effort != "" {
		return effort
	}
	if request.Thinking != nil {
		if request.Thinking.Type == "enabled" && request.Thinking.BudgetTokens != nil {
			return string(EffortFromBudget(*request.Thinking.BudgetTokens))
		}
		if request.Thinking.Type == "adaptive" || request.Thinking.Type == "enabled" {
			return string(ClaudeDefaultEffort(model))
		}
	}
	if claudeCapabilitiesFor(model).defaultThinking {
		return string(ClaudeDefaultEffort(model))
	}
	return ""
}

func claudeOutputConfig(request *dto.ClaudeRequest) (map[string]json.RawMessage, error) {
	config := make(map[string]json.RawMessage)
	if len(request.OutputConfig) > 0 {
		if err := kitutil.Unmarshal(request.OutputConfig, &config); err != nil {
			return nil, fmt.Errorf("invalid Claude output_config: %w", err)
		}
		if config == nil {
			config = make(map[string]json.RawMessage)
		}
	}
	return config, nil
}

func setClaudeOutputEffort(request *dto.ClaudeRequest, effort Effort) error {
	config, err := claudeOutputConfig(request)
	if err != nil {
		return err
	}
	if effort == "" {
		delete(config, "effort")
	} else {
		config["effort"], err = kitutil.Marshal(effort)
		if err != nil {
			return err
		}
	}
	if len(config) == 0 {
		request.OutputConfig = nil
		return nil
	}
	request.OutputConfig, err = kitutil.Marshal(config)
	return err
}

func copyClaudeThinking(request *dto.ClaudeRequest, kind string) *dto.Thinking {
	thinking := dto.Thinking{Type: kind}
	if request.Thinking != nil {
		thinking = *request.Thinking
		thinking.Type = kind
	}
	thinking.BudgetTokens = nil
	return &thinking
}

func clearClaudeSampling(request *dto.ClaudeRequest) {
	if claudeCapabilitiesFor(request.Model).strictSampling {
		request.Temperature, request.TopP, request.TopK = nil, nil, nil
	}
}

// SetClaudeAdaptiveThinking is used for generated/adapted thinking. Preserve
// native extension fields and structured output while setting the new strength.
func SetClaudeAdaptiveThinking(request *dto.ClaudeRequest, effort string) error {
	if effort == "" {
		effort = request.GetEfforts()
	}
	if effort == "" {
		effort = string(ClaudeDefaultEffort(request.Model))
	}
	parsed, err := ParseEffort(effort)
	if err != nil {
		return AsClientError(err)
	}
	if parsed == EffortNone {
		return ApplyClaudeReasoningEffort(request, effort)
	}
	if err := setClaudeOutputEffort(request, ClaudeEffort(request.Model, parsed)); err != nil {
		return err
	}
	request.Thinking = copyClaudeThinking(request, "adaptive")
	if (claudeModelFamily(request.Model, "claude-opus-4-7") || claudeModelFamily(request.Model, "claude-opus-4-8")) && request.Thinking.Display == "" {
		request.Thinking.Display = "summarized"
	}
	clearClaudeSampling(request)
	return nil
}

// NormalizeClaudeThinking retains the native omitted-thinking state. On known
// models it corrects legacy thinking and effort combinations that are rejected
// by the provider, without dropping unrelated output_config/thinking fields.
func NormalizeClaudeThinking(request *dto.ClaudeRequest) error {
	if request == nil {
		return nil
	}
	caps := claudeCapabilitiesFor(request.Model)
	if !caps.known {
		return nil
	}
	clearClaudeSampling(request)
	if effort := request.GetEfforts(); effort != "" {
		parsed, err := ParseEffort(effort)
		if err != nil {
			return AsClientError(err)
		}
		if err := setClaudeOutputEffort(request, ClaudeEffort(request.Model, parsed)); err != nil {
			return err
		}
	}
	if request.Thinking == nil {
		return nil
	}
	switch request.Thinking.Type {
	case "enabled":
		if caps.requiresAdaptive {
			return SetClaudeAdaptiveThinking(request, "")
		}
	case "adaptive":
		if caps.adaptive {
			request.Thinking = copyClaudeThinking(request, "adaptive")
		}
	case "disabled", "between_tools":
		if caps.disableAsBetweenTools {
			request.Thinking = &dto.Thinking{Type: "between_tools"}
		} else if caps.cannotDisable {
			return SetClaudeAdaptiveThinking(request, string(EffortLow))
		} else if request.Thinking.Type == "disabled" {
			request.Thinking = &dto.Thinking{Type: "disabled"}
		}
		if caps.offEffortAtMostHigh {
			effort := request.GetEfforts()
			if effort == string(EffortXHigh) || effort == string(EffortMax) {
				return setClaudeOutputEffort(request, EffortHigh)
			}
		}
	}
	return nil
}

func ApplyClaudeReasoningEffort(request *dto.ClaudeRequest, effort string) error {
	if effort == "" {
		return nil
	}
	parsed, err := ParseEffort(effort)
	if err != nil {
		return AsClientError(err)
	}
	if parsed == EffortNone {
		request.Thinking = copyClaudeThinking(request, "disabled")
		return NormalizeClaudeThinking(request)
	}
	if ClaudeSupportsAdaptiveThinking(request.Model) {
		return SetClaudeAdaptiveThinking(request, string(parsed))
	}
	var budget int
	switch parsed {
	case EffortMinimal, EffortLow:
		budget = 1280
	case EffortMedium:
		budget = 2048
	case EffortHigh, EffortXHigh, EffortMax:
		budget = 4096
	}
	thinking := copyClaudeThinking(request, "enabled")
	thinking.BudgetTokens = &budget
	request.Thinking = thinking
	if caps := claudeCapabilitiesFor(request.Model); caps.known && caps.supportsEffort {
		return setClaudeOutputEffort(request, ClaudeEffort(request.Model, parsed))
	}
	return nil
}

func ApplyClaudeThinkingBudget(request *dto.ClaudeRequest, budget int) error {
	if budget <= 0 {
		return nil
	}
	if ClaudeRequiresAdaptiveThinking(request.Model) || (request.Thinking != nil && request.Thinking.Type == "adaptive") {
		return SetClaudeAdaptiveThinking(request, "")
	}
	thinking := copyClaudeThinking(request, "enabled")
	thinking.BudgetTokens = &budget
	request.Thinking = thinking
	return nil
}

// Claude's off alias does not belong in the shared Gemini suffix table.
func TrimClaudeEffortSuffix(model string) (string, string, bool) {
	for _, suffix := range []string{"-none", "-nothinking"} {
		if strings.HasSuffix(model, suffix) {
			return strings.TrimSuffix(model, suffix), string(EffortNone), true
		}
	}
	return TrimEffortSuffix(model)
}
