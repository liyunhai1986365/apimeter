package reasoning

import "strings"

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
