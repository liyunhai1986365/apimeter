package reasoning

import (
	"context"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// IsKnownNonReasoningOpenAIModel only identifies established non-reasoning
// families. Unknown names may be private deployments or compatible providers;
// false does not assert that such a model supports any particular effort.
func IsKnownNonReasoningOpenAIModel(model string) bool {
	model = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "openai/")
	for _, family := range []string{
		"gpt-3.5-turbo", "gpt-4", "gpt-4o", "gpt-4.1", "gpt-4.5-preview", "chatgpt-4o-latest",
	} {
		if model == family || strings.HasPrefix(model, family+"-") {
			return true
		}
	}
	return false
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

func IsKnownOpenAITarget(model string) bool {
	lowest, _ := openAIEffortRange(model)
	return lowest != "" || IsKnownNonReasoningOpenAIModel(model)
}

func EffortForOpenAITarget(ctx context.Context, model, effort, path string, explicit bool) (string, error) {
	if effort == "" {
		return "", nil
	}
	if IsKnownNonReasoningOpenAIModel(model) {
		if explicit && effort != "none" {
			convdiag.Add(ctx, types.ConversionDiagnostic{
				Code: "openai_reasoning_unsupported", Path: path,
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
	parsed, err := ParseEffort(effort)
	if err != nil {
		return "", AsClientError(err)
	}
	adjusted := string(parsed)
	switch parsed {
	case EffortNone:
		adjusted = lowest
	case EffortMinimal:
		if lowest != "minimal" {
			adjusted = "low"
		}
	case EffortXHigh, EffortMax:
		adjusted = highest
	}
	if explicit && adjusted != string(parsed) {
		convdiag.Add(ctx, types.ConversionDiagnostic{
			Code: "openai_reasoning_effort_adjusted", Path: path,
			Message:  fmt.Sprintf("target model %q does not support reasoning effort %q; using %q", model, parsed, adjusted),
			Severity: types.ConversionDiagnosticWarning,
		})
	}
	return adjusted, nil
}
