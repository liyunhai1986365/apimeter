package awsbedrock

import (
	"net/http"
	"strings"
)

type betaProfile struct {
	modelMarkers []string
	allowed      map[string]struct{}
}

func betaSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

// These are the beta headers currently documented by Amazon Bedrock for
// InvokeModel/InvokeModelWithResponseStream. Unknown Anthropic-only or client
// beta headers must not be forwarded to Bedrock Runtime.
var documentedBetaHeaders = betaSet(
	"computer-use-2024-10-22",
	"computer-use-2025-01-24",
	"computer-use-2025-11-24",
	"token-efficient-tools-2025-02-19",
	"fine-grained-tool-streaming-2025-05-14",
	"interleaved-thinking-2025-05-14",
	"output-128k-2025-02-19",
	"dev-full-thinking-2025-05-14",
	"context-1m-2025-08-07",
	"context-management-2025-06-27",
	"effort-2025-11-24",
	"tool-search-tool-2025-10-19",
	"tool-examples-2025-10-29",
	"compact-2026-01-12",
	"fallback-credit-2026-06-09",
)

// Bedrock validates beta headers against the selected model. Profiles are
// ordered from newest/most specific to older model families. If a future or
// custom model alias cannot be identified, the documented Bedrock-wide set is
// used so that known AWS features remain forward-compatible.
var betaProfiles = []betaProfile{
	{[]string{"claude-opus-4-8"}, betaSet("computer-use-2025-11-24", "fallback-credit-2026-06-09")},
	{[]string{"claude-opus-4-7"}, betaSet("computer-use-2025-11-24")},
	{[]string{"claude-opus-4-6", "claude-sonnet-4-6"}, betaSet("computer-use-2025-11-24", "context-1m-2025-08-07", "compact-2026-01-12")},
	{[]string{"claude-opus-4-5"}, betaSet("computer-use-2025-01-24", "computer-use-2025-11-24", "token-efficient-tools-2025-02-19", "fine-grained-tool-streaming-2025-05-14", "interleaved-thinking-2025-05-14", "dev-full-thinking-2025-05-14", "context-management-2025-06-27", "effort-2025-11-24", "tool-search-tool-2025-10-19", "tool-examples-2025-10-29")},
	{[]string{"claude-sonnet-4-5"}, betaSet("computer-use-2025-01-24", "token-efficient-tools-2025-02-19", "fine-grained-tool-streaming-2025-05-14", "interleaved-thinking-2025-05-14", "dev-full-thinking-2025-05-14", "context-management-2025-06-27", "tool-search-tool-2025-10-19")},
	{[]string{"claude-haiku-4-5"}, betaSet("computer-use-2025-01-24", "token-efficient-tools-2025-02-19", "fine-grained-tool-streaming-2025-05-14", "interleaved-thinking-2025-05-14", "dev-full-thinking-2025-05-14", "context-management-2025-06-27")},
	{[]string{"claude-opus-4-1", "claude-opus-4-20250514", "claude-sonnet-4-20250514"}, betaSet("computer-use-2025-01-24", "token-efficient-tools-2025-02-19", "fine-grained-tool-streaming-2025-05-14", "interleaved-thinking-2025-05-14", "dev-full-thinking-2025-05-14", "context-1m-2025-08-07", "context-management-2025-06-27")},
	{[]string{"claude-3-7-sonnet"}, betaSet("computer-use-2025-01-24", "token-efficient-tools-2025-02-19", "output-128k-2025-02-19")},
	{[]string{"claude-3-5-sonnet-20241022"}, betaSet("computer-use-2024-10-22")},
}

func allowedBetaHeaders(modelNames ...string) map[string]struct{} {
	for _, modelName := range modelNames {
		modelName = strings.ToLower(modelName)
		for _, profile := range betaProfiles {
			for _, marker := range profile.modelMarkers {
				if strings.Contains(modelName, marker) {
					return profile.allowed
				}
			}
		}
	}
	return documentedBetaHeaders
}

func filterBetaValues(values []string, modelNames ...string) []string {
	allowed := allowedBetaHeaders(modelNames...)
	filtered := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			item = strings.ToLower(strings.TrimSpace(item))
			if _, ok := allowed[item]; !ok {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func FilterAnthropicBetaHeader(header *http.Header, modelNames ...string) {
	if header == nil || len(header.Values("anthropic-beta")) == 0 {
		return
	}
	filtered := filterBetaValues(header.Values("anthropic-beta"), modelNames...)
	if len(filtered) == 0 {
		header.Del("anthropic-beta")
		return
	}
	header.Set("anthropic-beta", strings.Join(filtered, ","))
}
