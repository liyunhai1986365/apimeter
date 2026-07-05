package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetRetryRouteEventsReturnsFilteredPage(t *testing.T) {
	openRelayRetryEventTestDB(t)
	require.NoError(t, model.RecordRetryRouteEvent(&model.RetryRouteEvent{
		RequestId:       "req-api-1",
		Username:        "alice",
		OriginalModel:   "gpt-4o",
		RuleName:        "global failover 500",
		Action:          operation_setting.RetryPolicyActionFailover,
		SourceChannelId: 10,
		TargetChannelId: 20,
		StatusCode:      http.StatusInternalServerError,
		ErrorCode:       "bad_response_status_code",
		FinalStatus:     "success",
		Matched:         true,
		Routed:          true,
		FinalSuccess:    true,
	}))
	require.NoError(t, model.RecordRetryRouteEvent(&model.RetryRouteEvent{
		RequestId:     "req-api-2",
		Username:      "bob",
		OriginalModel: "claude-sonnet",
		RuleName:      "skip client error",
		Action:        operation_setting.RetryPolicyActionSkipRetry,
		StatusCode:    http.StatusBadRequest,
		FinalStatus:   "failed",
		Matched:       true,
	}))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/retry-route/events?action=failover&model_name=gpt&page_size=10", nil)

	GetRetryRouteEvents(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"total":1`)
	require.Contains(t, w.Body.String(), `"request_id":"req-api-1"`)
	require.NotContains(t, w.Body.String(), `"request_id":"req-api-2"`)
}

func TestGetRetryRouteEventStatsReturnsBreakdowns(t *testing.T) {
	openRelayRetryEventTestDB(t)
	require.NoError(t, model.RecordRetryRouteEvent(&model.RetryRouteEvent{
		RequestId:       "req-stat-1",
		OriginalModel:   "gpt-4o",
		RuleName:        "global failover 500",
		Action:          operation_setting.RetryPolicyActionFailover,
		SourceChannelId: 10,
		TargetChannelId: 20,
		StatusCode:      http.StatusInternalServerError,
		ErrorCode:       "bad_response_status_code",
		Matched:         true,
		Routed:          true,
		FinalSuccess:    true,
	}))
	require.NoError(t, model.RecordRetryRouteEvent(&model.RetryRouteEvent{
		RequestId:     "req-stat-2",
		OriginalModel: "gpt-4o",
		RuleName:      "skip client error",
		Action:        operation_setting.RetryPolicyActionSkipRetry,
		StatusCode:    http.StatusBadRequest,
		ErrorCode:     "invalid_request",
		Matched:       true,
	}))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/retry-route/events/stat?model_name=gpt-4o", nil)

	GetRetryRouteEventStats(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"total":2`)
	require.Contains(t, w.Body.String(), `"matched":2`)
	require.Contains(t, w.Body.String(), `"routed":1`)
	require.Contains(t, w.Body.String(), `"failover":1`)
	require.Contains(t, w.Body.String(), `"skip_retry":1`)
}

func TestTestRetryRouteRulesUsesSubmittedRules(t *testing.T) {
	orig := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = orig })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{Name: "global skip", Action: operation_setting.RetryPolicyActionSkipRetry, StatusCodes: "500"},
	}

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{
		"input": {
			"model_name": "gpt-4o",
			"status_code": 500,
			"error_message": "temporary upstream failure"
		},
		"rules": [
			{
				"name": "adhoc failover",
				"action": "failover",
				"status_codes": "500",
				"targets": {"groups": ["backup"]}
			}
		]
	}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/retry-route/rules/test", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	TestRetryRouteRules(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"rule_name":"adhoc failover"`)
	require.Equal(t, "global skip", operation_setting.AutomaticRetryPolicyRules[0].Name)
}

func TestTestRetryRouteRulesRejectsInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/retry-route/rules/test", strings.NewReader(`{"rules":[{"action":"failover"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")

	TestRetryRouteRules(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":false`)
	require.Contains(t, w.Body.String(), "failover rules must configure at least one channel target")
}

func TestParseRetryRouteEventQueryTrimsQuotedInts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/retry-route/events?source_channel_id=%2210%22&status_code='500'", nil)

	query := parseRetryRouteEventQuery(c, common.GetPageQuery(c))

	require.Equal(t, 10, query.SourceChannelId)
	require.Equal(t, 500, query.StatusCode)
}
