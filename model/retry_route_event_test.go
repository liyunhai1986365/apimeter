package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestRetryRouteEventQueryFiltersAndStats(t *testing.T) {
	truncateTables(t)

	err := RecordRetryRouteEvent(&RetryRouteEvent{
		RequestId:       "req-a",
		AttemptIndex:    0,
		UserId:          1001,
		Username:        "alice",
		TokenId:         2001,
		TokenName:       "prod-key",
		OriginalModel:   "gpt-4o",
		RuleSource:      "global",
		RuleName:        "429 backup",
		Action:          "failover",
		Matched:         true,
		Routed:          true,
		FinalSuccess:    true,
		FinalStatus:     "success",
		SourceChannelId: 11,
		TargetChannelId: 22,
		SourceGroup:     "default",
		TargetGroup:     "backup",
		StatusCode:      429,
		ErrorCode:       "rate_limit",
		ErrorType:       "upstream_error",
		ErrorMessage:    "rate limited",
		LogId:           101,
		RequestHash:     "hash-a",
	})
	require.NoError(t, err)
	require.NoError(t, RecordRetryRouteEvent(&RetryRouteEvent{
		RequestId:       "req-b",
		AttemptIndex:    0,
		UserId:          1002,
		OriginalModel:   "claude-sonnet-4",
		RuleSource:      "global",
		RuleName:        "invalid key skip",
		Action:          "skip_retry",
		Matched:         true,
		Routed:          false,
		FinalSuccess:    false,
		FinalStatus:     "failed",
		SourceChannelId: 33,
		StatusCode:      401,
		ErrorCode:       "invalid_key",
		ErrorType:       "channel_error",
		ErrorMessage:    "invalid key",
		LogId:           102,
		RequestHash:     "hash-b",
	}))

	events, total, err := GetRetryRouteEvents(RetryRouteEventQuery{
		RequestId: "req-a",
		Action:    "failover",
		Page:      1,
		PageSize:  20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Len(t, events, 1)
	require.Equal(t, "429 backup", events[0].RuleName)
	require.Equal(t, 22, events[0].TargetChannelId)

	stats, err := GetRetryRouteEventStats(RetryRouteEventQuery{})
	require.NoError(t, err)
	require.Equal(t, int64(2), stats.Total)
	require.Equal(t, int64(2), stats.Matched)
	require.Equal(t, int64(1), stats.Routed)
	require.Equal(t, int64(1), stats.FinalSuccess)
	require.InDelta(t, 0.5, stats.RecoverySuccessRate, 0.001)
	require.Equal(t, int64(1), stats.ActionBreakdown["failover"])
	require.Equal(t, int64(1), stats.ActionBreakdown["skip_retry"])

	events, total, err = GetRetryRouteEvents(RetryRouteEventQuery{
		RequestHash: "hash-a",
		LogId:       101,
		Page:        1,
		PageSize:    20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
	require.Equal(t, "req-a", events[0].RequestId)
}

func TestRetryRouteRuleStatsAggregatesRecoveryByRule(t *testing.T) {
	truncateTables(t)

	require.NoError(t, RecordRetryRouteEvent(&RetryRouteEvent{
		RequestId:       "req-rule-1",
		RuleSource:      "global",
		RuleName:        "global failover",
		Action:          "failover",
		OriginalModel:   "gpt-4o",
		StatusCode:      500,
		ErrorCode:       "bad_response_status_code",
		Matched:         true,
		Routed:          true,
		FinalSuccess:    true,
		FinalStatus:     "success",
		UseTimeMs:       120,
		LogId:           201,
		RequestHash:     "hash-same",
		SourceChannelId: 10,
		TargetChannelId: 20,
	}))
	require.NoError(t, RecordRetryRouteEvent(&RetryRouteEvent{
		RequestId:       "req-rule-2",
		RuleSource:      "global",
		RuleName:        "global failover",
		Action:          "failover",
		OriginalModel:   "gpt-4o",
		StatusCode:      500,
		ErrorCode:       "bad_response_status_code",
		Matched:         true,
		Routed:          true,
		FinalSuccess:    false,
		FinalStatus:     "failed",
		UseTimeMs:       80,
		LogId:           202,
		RequestHash:     "hash-same",
		SourceChannelId: 10,
		TargetChannelId: 21,
	}))
	require.NoError(t, RecordRetryRouteEvent(&RetryRouteEvent{
		RequestId:     "req-rule-3",
		RuleSource:    "channel",
		RuleName:      "skip client",
		Action:        "skip_retry",
		OriginalModel: "gpt-4o",
		StatusCode:    400,
		ErrorCode:     "invalid_request",
		Matched:       true,
		FinalStatus:   "failed",
	}))

	stats, err := GetRetryRouteRuleStats(RetryRouteEventQuery{RequestHash: "hash-same"})
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "global", stats[0].RuleSource)
	require.Equal(t, "global failover", stats[0].RuleName)
	require.Equal(t, "failover", stats[0].Action)
	require.Equal(t, int64(2), stats[0].Total)
	require.Equal(t, int64(2), stats[0].Routed)
	require.Equal(t, int64(1), stats[0].FinalSuccess)
	require.Equal(t, int64(1), stats[0].FinalFailed)
	require.Equal(t, 0.5, stats[0].RecoverySuccessRate)
	require.Equal(t, int64(100), stats[0].AvgUseTimeMs)
}

func TestRetryRouteEventFinalStatusUpdate(t *testing.T) {
	truncateTables(t)

	event := &RetryRouteEvent{RequestId: "req-final", RuleName: "backup", Action: "failover", Matched: true}
	require.NoError(t, RecordRetryRouteEvent(event))
	require.NotZero(t, event.Id)

	require.NoError(t, MarkRetryRouteEventsFinal("req-final", true, "success"))

	events, _, err := GetRetryRouteEvents(RetryRouteEventQuery{RequestId: "req-final", Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.True(t, events[0].FinalSuccess)
	require.Equal(t, "success", events[0].FinalStatus)
	require.NotZero(t, events[0].UpdatedAt)
}

func TestRetryRouteEventExtraUsesCommonJSON(t *testing.T) {
	payload := map[string]interface{}{"retry_groups": []string{"backup"}}
	raw, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, string(raw), "retry_groups")
}
