package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/require"
)

// TestFormatUserLogsStripsQuotaSaturation verifies the admin-only quota
// saturation marker (nested under other.admin_info) is removed for non-admin
// log views, since formatUserLogs strips the whole admin_info object.
func TestFormatUserLogsStripsQuotaSaturation(t *testing.T) {
	other := common.MapToJsonStr(map[string]interface{}{
		"model_price": 0.004,
		"admin_info": map[string]interface{}{
			"quota_saturation": map[string]interface{}{
				"op":      "QuotaFromDecimal",
				"kind":    "overflow",
				"clamped": common.MaxQuota,
			},
		},
	})
	logs := []*Log{{Other: other}}

	formatUserLogs(logs, 0)

	parsed, err := common.StrToMap(logs[0].Other)
	require.NoError(t, err)
	_, hasAdminInfo := parsed["admin_info"]
	require.False(t, hasAdminInfo, "admin_info (and nested quota_saturation) must be stripped for non-admin views")
	// Non-admin billing fields remain visible.
	require.Contains(t, parsed, "model_price")
}

func TestUserLogResponsesHideChannelRouting(t *testing.T) {
	truncateTables(t)
	log := Log{UserId: 1001, TokenId: 101, ChannelId: 72, Type: LogTypeConsume, Quota: 123,
		RequestId: "customer-request", CompletionTokens: 456,
		Other: `{"channel_id":72,"channel_name":"private-supplier","channel_type":999,"admin_info":{"use_channel":[72]},"audit_info":{"key":"private"},"retry_route_event_ids":[99],"reject_reason":"internal diagnostic","stream_status":"debug","cost_quota":9,"task_id":"customer-task","resolution":"1080p","has_video":true}`}
	require.NoError(t, LOG_DB.Create(&log).Error)
	for _, mode := range []string{"page", "cursor", "token"} {
		t.Run(mode, func(t *testing.T) {
			var logs []*Log
			var err error
			switch mode {
			case "page":
				logs, _, err = GetUserLogs(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "", nil)
			case "cursor":
				logs, _, _, err = GetUserLogsByCursor(1001, LogTypeUnknown, 0, 0, "", "", 0, 20, "", "", "", "", nil)
			case "token":
				logs, err = GetLogByTokenId(101)
			}
			require.NoError(t, err)
			require.Len(t, logs, 1)
			body, err := common.Marshal(logs[0])
			require.NoError(t, err)
			var item map[string]any
			require.NoError(t, common.Unmarshal(body, &item))
			require.NotContains(t, item, "channel")
			require.NotContains(t, item, "channel_name")
			require.Equal(t, float64(123), item["quota"])
			require.Equal(t, float64(456), item["completion_tokens"])
			require.Equal(t, "customer-request", item["request_id"])
			require.JSONEq(t, `{"task_id":"customer-task","resolution":"1080p","has_video":true}`, logs[0].Other)
		})
	}
	admin, _, err := GetAllLogs(LogTypeUnknown, 0, 0, "", "", "", 0, 20, 0, "", "", "")
	require.NoError(t, err)
	require.Len(t, admin, 1)
	require.Equal(t, 72, admin[0].ChannelId)
	require.JSONEq(t, log.Other, admin[0].Other, "formatting customer responses must not alter stored administrator diagnostics")
}
