package operation_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestParseHTTPStatusCodeRanges_CommaSeparated(t *testing.T) {
	ranges, err := ParseHTTPStatusCodeRanges("401,403,500-599")
	require.NoError(t, err)
	require.Equal(t, []StatusCodeRange{
		{Start: 401, End: 401},
		{Start: 403, End: 403},
		{Start: 500, End: 599},
	}, ranges)
}

func TestParseHTTPStatusCodeRanges_MergeAndNormalize(t *testing.T) {
	ranges, err := ParseHTTPStatusCodeRanges("500-505,504,401,403,402")
	require.NoError(t, err)
	require.Equal(t, []StatusCodeRange{
		{Start: 401, End: 403},
		{Start: 500, End: 505},
	}, ranges)
}

func TestParseHTTPStatusCodeRanges_Invalid(t *testing.T) {
	_, err := ParseHTTPStatusCodeRanges("99,600,foo,500-400,500-")
	require.Error(t, err)
}

func TestParseHTTPStatusCodeRanges_NoComma_IsInvalid(t *testing.T) {
	_, err := ParseHTTPStatusCodeRanges("401 403")
	require.Error(t, err)
}

func TestShouldDisableByStatusCode(t *testing.T) {
	orig := AutomaticDisableStatusCodeRanges
	t.Cleanup(func() { AutomaticDisableStatusCodeRanges = orig })

	AutomaticDisableStatusCodeRanges = []StatusCodeRange{
		{Start: 401, End: 403},
		{Start: 500, End: 599},
	}

	require.True(t, ShouldDisableByStatusCode(401))
	require.True(t, ShouldDisableByStatusCode(403))
	require.False(t, ShouldDisableByStatusCode(404))
	require.True(t, ShouldDisableByStatusCode(500))
	require.False(t, ShouldDisableByStatusCode(200))
}

func TestShouldRetryByStatusCode(t *testing.T) {
	orig := AutomaticRetryStatusCodeRanges
	t.Cleanup(func() { AutomaticRetryStatusCodeRanges = orig })

	AutomaticRetryStatusCodeRanges = []StatusCodeRange{
		{Start: 429, End: 429},
		{Start: 500, End: 599},
	}

	require.True(t, ShouldRetryByStatusCode(429))
	require.True(t, ShouldRetryByStatusCode(500))
	require.False(t, ShouldRetryByStatusCode(504))
	require.False(t, ShouldRetryByStatusCode(524))
	require.False(t, ShouldRetryByStatusCode(400))
	require.False(t, ShouldRetryByStatusCode(200))
}

func TestShouldRetryByStatusCode_DefaultMatchesLegacyBehavior(t *testing.T) {
	require.False(t, ShouldRetryByStatusCode(200))
	require.False(t, ShouldRetryByStatusCode(400))
	require.True(t, ShouldRetryByStatusCode(401))
	require.False(t, ShouldRetryByStatusCode(408))
	require.True(t, ShouldRetryByStatusCode(429))
	require.True(t, ShouldRetryByStatusCode(500))
	require.False(t, ShouldRetryByStatusCode(504))
	require.False(t, ShouldRetryByStatusCode(524))
	require.True(t, ShouldRetryByStatusCode(599))
}

func TestIsAlwaysSkipRetryStatusCode(t *testing.T) {
	require.True(t, IsAlwaysSkipRetryStatusCode(504))
	require.True(t, IsAlwaysSkipRetryStatusCode(524))
	require.False(t, IsAlwaysSkipRetryStatusCode(500))
}

func TestRetryPolicyChannelRulesOverrideGlobalRules(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	AutomaticRetryPolicyRules = []RetryPolicyRule{
		{
			Name:   "global retry on overloaded",
			Action: RetryPolicyActionRetry,
			Conditions: RetryPolicyConditions{
				MessageContains: []string{"overloaded"},
			},
		},
	}

	decision := ShouldRetryByPolicy(RetryPolicyInput{
		ModelName:    "gpt-image-2",
		ChannelID:    29,
		ChannelType:  1,
		ErrorType:    types.ErrorTypeOpenAIError,
		ErrorCode:    types.ErrorCode("temporary_error"),
		StatusCode:   500,
		ErrorMessage: "upstream overloaded",
		ChannelRules: []RetryPolicyRule{
			{
				Name:   "channel skips gpt-image-2",
				Action: RetryPolicyActionSkipRetry,
				Conditions: RetryPolicyConditions{
					Models: []string{"gpt-image-2"},
				},
			},
		},
	})

	require.True(t, decision.Matched)
	require.Equal(t, RetryPolicySourceChannel, decision.Source)
	require.False(t, decision.ShouldRetry)
}

func TestRetryPolicyGlobalRulesApplyWhenChannelRulesMiss(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	AutomaticRetryPolicyRules = []RetryPolicyRule{
		{
			Name:   "global retry private ip",
			Action: RetryPolicyActionRetry,
			Conditions: RetryPolicyConditions{
				MessageContains: []string{"private ip"},
			},
		},
	}

	decision := ShouldRetryByPolicy(RetryPolicyInput{
		ModelName:    "gpt-image-2",
		StatusCode:   400,
		ErrorMessage: "download image failed: private ip rejected",
		ChannelRules: []RetryPolicyRule{
			{
				Name:   "channel only skips claude",
				Action: RetryPolicyActionSkipRetry,
				Conditions: RetryPolicyConditions{
					Models: []string{"claude-3-5-sonnet"},
				},
			},
		},
	})

	require.True(t, decision.Matched)
	require.Equal(t, RetryPolicySourceGlobal, decision.Source)
	require.True(t, decision.ShouldRetry)
}

func TestRetryPolicyDecisionCarriesRecoveryGroupsAndMaxRetries(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	AutomaticRetryPolicyRules = []RetryPolicyRule{
		{
			Name:        "codex encrypted recovery",
			Action:      RetryPolicyActionRetry,
			StatusCodes: "400",
			MessageContains: []string{
				"encrypted content",
				"could not be verified",
			},
			RetryGroups: []string{"codex-primary", "codex-backup"},
			MaxRetries:  2,
		},
	}

	decision := ShouldRetryByPolicy(RetryPolicyInput{
		StatusCode:   400,
		ErrorMessage: "status_code=400, The encrypted content gAAA could not be verified",
	})

	require.True(t, decision.Matched)
	require.True(t, decision.ShouldRetry)
	require.Equal(t, RetryPolicySourceGlobal, decision.Source)
	require.Equal(t, "codex encrypted recovery", decision.RuleName)
	require.Equal(t, []string{"codex-primary", "codex-backup"}, decision.RetryGroups)
	require.Equal(t, 2, decision.MaxRetries)
}

func TestRetryPolicyDecisionCarriesFailoverTargets(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	AutomaticRetryPolicyRules = []RetryPolicyRule{
		{
			Name:        "gpt failover",
			Action:      RetryPolicyActionFailover,
			Priority:    10,
			StatusCodes: "500",
			Targets: RetryPolicyTargets{
				Groups:      []string{"backup"},
				ChannelIDs:  []int{28},
				ChannelTags: []string{"stable"},
			},
			Strategy: RetryPolicyStrategy{
				MaxRetries:           2,
				ExcludeFailedChannel: true,
			},
		},
	}

	decision := ShouldRetryByPolicy(RetryPolicyInput{
		ModelName:    "gpt-5",
		ChannelID:    12,
		StatusCode:   500,
		ErrorMessage: "upstream overloaded",
	})

	require.True(t, decision.Matched)
	require.True(t, decision.ShouldRetry)
	require.Equal(t, RetryPolicyActionFailover, decision.Action)
	require.Equal(t, []string{"backup"}, decision.Targets.Groups)
	require.Equal(t, []int{28}, decision.Targets.ChannelIDs)
	require.Equal(t, []string{"stable"}, decision.Targets.ChannelTags)
	require.Equal(t, 2, decision.MaxRetries)
	require.Equal(t, 12, decision.ExcludeChannelID)
}

func TestRetryPolicyFailoverExcludesFailedChannelByDefault(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	AutomaticRetryPolicyRules = []RetryPolicyRule{
		{
			Name:        "default failover",
			Action:      RetryPolicyActionFailover,
			StatusCodes: "500",
			Targets: RetryPolicyTargets{
				Groups: []string{"backup"},
			},
		},
	}

	decision := ShouldRetryByPolicy(RetryPolicyInput{
		ChannelID:    12,
		StatusCode:   500,
		ErrorMessage: "upstream overloaded",
	})

	require.True(t, decision.Matched)
	require.Equal(t, RetryPolicyActionFailover, decision.Action)
	require.Equal(t, 12, decision.ExcludeChannelID)
}

func TestRetryPolicySkipRetryWinsOverFailoverWithinSameSource(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	AutomaticRetryPolicyRules = []RetryPolicyRule{
		{
			Name:        "broad failover",
			Action:      RetryPolicyActionFailover,
			Priority:    100,
			StatusCodes: "400-599",
			Targets: RetryPolicyTargets{
				Groups: []string{"backup"},
			},
		},
		{
			Name:        "invalid request must not retry",
			Action:      RetryPolicyActionSkipRetry,
			Priority:    1,
			StatusCodes: "400",
			ErrorCodes:  []types.ErrorCode{types.ErrorCodeInvalidRequest},
		},
	}

	decision := ShouldRetryByPolicy(RetryPolicyInput{
		ModelName:    "gpt-5",
		StatusCode:   400,
		ErrorCode:    types.ErrorCodeInvalidRequest,
		ErrorMessage: "invalid request",
	})

	require.True(t, decision.Matched)
	require.Equal(t, RetryPolicyActionSkipRetry, decision.Action)
	require.False(t, decision.ShouldRetry)
	require.Equal(t, "invalid request must not retry", decision.RuleName)
}

func TestAutomaticRetryPolicyRulesFromStringParsesFailoverRule(t *testing.T) {
	orig := AutomaticRetryPolicyRules
	t.Cleanup(func() { AutomaticRetryPolicyRules = orig })

	err := AutomaticRetryPolicyRulesFromString(`[
		{
			"name": "json failover",
			"enabled": true,
			"priority": 50,
			"action": "failover",
			"status_codes": "500-504",
			"targets": {
				"groups": ["backup"],
				"channel_ids": [28],
				"channel_tags": ["stable"]
			},
			"strategy": {
				"max_retries": 2,
				"exclude_failed_channel": true
			}
		}
	]`)
	require.NoError(t, err)
	require.Len(t, AutomaticRetryPolicyRules, 1)

	rule := AutomaticRetryPolicyRules[0]
	require.Equal(t, RetryPolicyActionFailover, rule.Action)
	require.Equal(t, 50, rule.Priority)
	require.Equal(t, []string{"backup"}, rule.Targets.Groups)
	require.Equal(t, []int{28}, rule.Targets.ChannelIDs)
	require.Equal(t, []string{"stable"}, rule.Targets.ChannelTags)
	require.Equal(t, 2, rule.Strategy.MaxRetries)
	require.True(t, rule.Strategy.ExcludeFailedChannel)
}

func TestValidateRetryPolicyRuleRejectsFailoverWithOnlyTargetModel(t *testing.T) {
	err := ValidateRetryPolicyRule(RetryPolicyRule{
		Name:        "model-only failover",
		Action:      RetryPolicyActionFailover,
		StatusCodes: "500",
		Targets: RetryPolicyTargets{
			Model: "gpt-5-mini",
		},
	})

	require.ErrorContains(t, err, "failover rules must configure at least one channel target")
}

func TestRetryPolicyRuleMatchesExtendedConditions(t *testing.T) {
	stream := false
	rule := RetryPolicyRule{
		Name:   "workspace route",
		Action: RetryPolicyActionFailover,
		Conditions: RetryPolicyConditions{
			Groups:       []string{"vip"},
			RequestPaths: []string{"/v1/chat/completions"},
			Stream:       &stream,
			TokenIDs:     []int{12},
			WorkspaceIDs: []int{34},
			StatusCodes:  "429",
		},
		Targets: RetryPolicyTargets{Groups: []string{"backup"}},
	}

	decision, ok := matchRetryPolicyRules(RetryPolicyInput{
		ModelName:   "gpt-4o",
		Group:       "vip",
		RequestPath: "/v1/chat/completions",
		IsStream:    false,
		TokenID:     12,
		WorkspaceID: 34,
		StatusCode:  429,
	}, []RetryPolicyRule{rule}, RetryPolicySourceGlobal)

	require.True(t, ok)
	require.True(t, decision.ShouldRetry)
	require.Equal(t, "workspace route", decision.RuleName)
}

func TestRetryPolicyRuleRejectsExtendedConditionMismatch(t *testing.T) {
	stream := true
	rule := RetryPolicyRule{
		Name:   "stream only",
		Action: RetryPolicyActionRetry,
		Conditions: RetryPolicyConditions{
			Stream: &stream,
		},
		MaxRetries: 1,
	}

	_, ok := matchRetryPolicyRules(RetryPolicyInput{IsStream: false}, []RetryPolicyRule{rule}, RetryPolicySourceGlobal)

	require.False(t, ok)
}
