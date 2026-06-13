package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
