package openai

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type chatStreamUsage struct {
	fields map[string]json.RawMessage
	counts chatTokenCounts
	cached int // Some providers report cache hits outside usage.
}

// nil means unknown. Never put an estimate into a running snapshot.
type chatTokenCounts struct {
	prompt, completion, total *int
}

func (n chatTokenCounts) merge(previous chatTokenCounts) chatTokenCounts {
	switch {
	case n.prompt == nil && n.completion == nil && n.total == nil:
		return previous
	case n.prompt == nil && n.completion == nil:
		// Only the total changed. The prompt of a chat request is normally
		// fixed; use a known completion instead if there is no usable prompt.
		if previous.prompt != nil && *previous.prompt <= *n.total {
			n.prompt = previous.prompt
		} else if previous.completion != nil && *previous.completion <= *n.total {
			n.completion = previous.completion
		}
	case n.total == nil:
		// A new component supersedes the old total. Prefer the other
		// component; use an earlier total only if it is all we have.
		if n.prompt == nil {
			n.prompt = previous.prompt
		}
		if n.completion == nil {
			n.completion = previous.completion
		}
		if n.prompt == nil || n.completion == nil {
			if t := previous.total; t != nil && (n.prompt == nil || *t >= *n.prompt) && (n.completion == nil || *t >= *n.completion) {
				n.total = t
			}
		}
	}
	// Any pair from the new snapshot determines the third, even if the
	// previous snapshot explicitly reported a different value for it.
	switch {
	case n.total == nil && n.prompt != nil && n.completion != nil:
		total := *n.prompt + *n.completion
		n.total = &total
	case n.prompt == nil && n.total != nil && n.completion != nil && *n.total >= *n.completion:
		prompt := *n.total - *n.completion
		n.prompt = &prompt
	case n.completion == nil && n.total != nil && n.prompt != nil && *n.total >= *n.prompt:
		completion := *n.total - *n.prompt
		n.completion = &completion
	}
	return n
}

// Keep the latest reported value of each field, not a sum of snapshots.
// Raw values distinguish absent/null from zero and preserve provider extensions.
func (s *chatStreamUsage) observe(raw gjson.Result) error {
	var reported dto.Usage
	if err := common.UnmarshalJsonStr(raw.Raw, &reported); err != nil {
		return err
	}
	var incoming map[string]json.RawMessage
	if err := common.UnmarshalJsonStr(raw.Raw, &incoming); err != nil {
		return err
	}
	take := func(key string, value int) *int {
		raw := incoming[key]
		delete(incoming, key)
		if len(raw) == 0 || string(raw) == "null" {
			return nil
		}
		return &value
	}
	n := chatTokenCounts{take("prompt_tokens", reported.PromptTokens), take("completion_tokens", reported.CompletionTokens), take("total_tokens", reported.TotalTokens)}
	// A wholly zero trailer must not erase a positive cumulative report.
	if n.prompt != nil && n.completion != nil && n.total != nil && *n.prompt == 0 && *n.completion == 0 && *n.total == 0 {
		for _, v := range []*int{s.counts.prompt, s.counts.completion, s.counts.total} {
			if v != nil && *v > 0 {
				return nil
			}
		}
	}
	s.counts = n.merge(s.counts)
	if s.fields == nil {
		s.fields = make(map[string]json.RawMessage)
	}
	for key, value := range incoming {
		if string(value) == "null" {
			continue
		}
		if old := s.fields[key]; strings.HasSuffix(key, "_details") && len(old) > 0 && old[0] == '{' && value[0] == '{' {
			var err error
			value, err = mergeUsageDetails(old, value)
			if err != nil {
				return err
			}
		}
		s.fields[key] = value
	}
	return nil
}

func mergeUsageDetails(old, incoming json.RawMessage) (json.RawMessage, error) {
	var before, after map[string]json.RawMessage
	if err := common.Unmarshal(old, &before); err != nil {
		return nil, err
	}
	if err := common.Unmarshal(incoming, &after); err != nil {
		return nil, err
	}
	for key, value := range after {
		if string(value) == "null" {
			continue
		}
		prior := before[key]
		if key == "cache_write_tokens_details" && len(prior) > 0 && prior[0] == '{' && value[0] == '{' {
			merged, err := mergeUsageDetails(prior, value)
			if err != nil {
				return nil, err
			}
			value = merged
		}
		before[key] = value
	}
	return common.Marshal(before)
}

func (s *chatStreamUsage) observeCache(channel int, frame string) {
	var count gjson.Result
	switch channel {
	case constant.ChannelTypeMoonshot:
		count = gjson.Get(frame, "choices.#(usage.cached_tokens>0).usage.cached_tokens")
	case constant.ChannelTypeOpenAI:
		count = gjson.Get(frame, "timings.cache_n")
	}
	if count.Type == gjson.Number {
		s.cached = int(count.Int())
	}
}

func (s *chatStreamUsage) finish(c *gin.Context, info *relaycommon.RelayInfo, text string, tools int) (*dto.Usage, json.RawMessage, error) {
	for k, v := range map[string]*int{"prompt_tokens": s.counts.prompt, "completion_tokens": s.counts.completion, "total_tokens": s.counts.total} {
		if v != nil {
			s.fields[k] = strconv.AppendInt(nil, int64(*v), 10)
		}
	}
	body, _ := common.Marshal(map[string]any{"usage": s.fields})
	var usage dto.Usage
	raw := gjson.GetBytes(body, "usage")
	if err := common.UnmarshalJsonStr(raw.Raw, &usage); err != nil {
		return nil, nil, err
	}
	prompt := func() int {
		common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		return info.GetEstimatePromptTokens()
	}
	completion := func() int {
		common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
		return service.EstimateTokenByModel(info.UpstreamModelName, text) + tools*7
	}
	body, err := fillMissingChatUsage(body, &usage, prompt, completion)
	if err != nil {
		return nil, nil, err
	}
	applyUsagePostProcessing(info, &usage, body)
	if usage.PromptTokensDetails.CachedTokens == 0 {
		usage.PromptTokensDetails.CachedTokens = s.cached
	}
	return &usage, json.RawMessage(gjson.GetBytes(body, "usage").Raw), nil
}

// Never discard a tool, finish reason, or vendor event along with its usage.
func isUsageOnlyFrame(frame string) bool {
	payload := false
	gjson.Parse(frame).ForEach(func(k, v gjson.Result) bool {
		switch k.String() {
		case "id", "object", "created", "model", "system_fingerprint", "usage":
		case "choices":
			payload = v.Get("#").Int() > 0
		default:
			payload = true
		}
		return !payload
	})
	return !payload
}
