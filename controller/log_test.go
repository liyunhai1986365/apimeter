package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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

func TestGetLogsSelfStatUsesNetLedgerForCurrentUser(t *testing.T) {
	db := setupUseDataControllerTestDB(t)
	require.NoError(t, db.Create(&[]model.Log{
		{Id: 301, UserId: 1001, Username: "old-name", Type: model.LogTypeConsume, CreatedAt: 100, Quota: 1000},
		{Id: 302, UserId: 1001, Username: "new-name", Type: model.LogTypeRefund, CreatedAt: 101, Quota: 700},
		{Id: 303, UserId: 2002, Username: "new-name", Type: model.LogTypeConsume, CreatedAt: 102, Quota: 9000},
	}).Error)

	ctx, recorder := newAuthenticatedContext(t, http.MethodGet, "/api/log/self/stat", nil, 1001)
	GetLogsSelfStat(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Quota int `json:"quota"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 300, response.Data.Quota)
}
