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
