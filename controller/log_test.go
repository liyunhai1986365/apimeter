package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestParseLogQueryIntAcceptsQuotedValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "plain", value: "11", want: 11},
		{name: "double quoted", value: `"11"`, want: 11},
		{name: "single quoted", value: `'11'`, want: 11},
		{name: "spaced quoted", value: ` "11" `, want: 11},
		{name: "invalid", value: `"abc"`, want: 0},
		{name: "empty", value: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseLogQueryInt(tt.value); got != tt.want {
				t.Fatalf("parseLogQueryInt(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestBuildLogsStatDataOnlyIncludesCostForRoot(t *testing.T) {
	stat := model.Stat{
		Quota:       3000,
		Rpm:         12,
		Tpm:         34,
		BaseQuota:   2000,
		CostQuota:   1200,
		ProfitQuota: 1800,
	}

	adminData := buildLogsStatData(stat, common.RoleAdminUser)
	if _, ok := adminData["base_quota"]; ok {
		t.Fatalf("ordinary admin stat data should not include base_quota: %#v", adminData)
	}
	if _, ok := adminData["cost_quota"]; ok {
		t.Fatalf("ordinary admin stat data should not include cost_quota: %#v", adminData)
	}
	if _, ok := adminData["profit_quota"]; ok {
		t.Fatalf("ordinary admin stat data should not include profit_quota: %#v", adminData)
	}

	rootData := buildLogsStatData(stat, common.RoleRootUser)
	if got := rootData["base_quota"]; got != stat.BaseQuota {
		t.Fatalf("root base_quota = %v, want %d", got, stat.BaseQuota)
	}
	if got := rootData["cost_quota"]; got != stat.CostQuota {
		t.Fatalf("root cost_quota = %v, want %d", got, stat.CostQuota)
	}
	if got := rootData["profit_quota"]; got != stat.ProfitQuota {
		t.Fatalf("root profit_quota = %v, want %d", got, stat.ProfitQuota)
	}
}

func TestGetErrorRequestLogReturnsRequestEvidence(t *testing.T) {
	openRelayRetryEventTestDB(t)
	require.NoError(t, model.RecordErrorRequestLog(&model.ErrorRequestLog{
		LogId:          42,
		RequestId:      "req-error",
		RequestMethod:  http.MethodPost,
		RequestPath:    "/v1/chat/completions",
		RequestBody:    `{"model":"gpt-4o"}`,
		RequestHash:    "hash-error",
		RequestHeaders: `{"Authorization":"[REDACTED]"}`,
		StatusCode:     http.StatusBadRequest,
		ErrorCode:      "invalid_request",
	}))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "log_id", Value: "42"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log/error-request/42", nil)

	GetErrorRequestLog(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":true`)
	require.Contains(t, w.Body.String(), `"request_id":"req-error"`)
	require.Contains(t, w.Body.String(), `"request_hash":"hash-error"`)
	require.Contains(t, w.Body.String(), `"request_body":"{\"model\":\"gpt-4o\"}"`)
}

func TestGetErrorRequestLogRejectsInvalidLogID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "log_id", Value: "abc"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/log/error-request/abc", nil)

	GetErrorRequestLog(c)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"success":false`)
}
