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
