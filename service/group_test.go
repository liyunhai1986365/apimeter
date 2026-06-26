package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

func TestGetUserUsableGroupsExcludesCurrentUserGroupWhenItIsUserGroupOnly(t *testing.T) {
	t.Cleanup(func() {
		if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`); err != nil {
			t.Fatalf("failed to reset user usable groups: %v", err)
		}
		if err := setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
		if err := ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":1,"svip":1}`); err != nil {
			t.Fatalf("failed to reset group ratios: %v", err)
		}
	})

	if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`); err != nil {
		t.Fatalf("failed to update user usable groups: %v", err)
	}
	if err := setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "default", "order": 10},
			{"group": "vip", "order": 20, "user_group": true}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	groups := GetUserUsableGroups("vip")
	if _, ok := groups["vip"]; ok {
		t.Fatalf("expected user-group-only vip to be excluded from token groups, got %+v", groups)
	}
	if _, ok := groups["default"]; !ok {
		t.Fatalf("expected default token group to remain available, got %+v", groups)
	}
}

func TestGetUserAutoGroupDefaultsToVisibleTokenGroups(t *testing.T) {
	t.Cleanup(func() {
		if err := setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`); err != nil {
			t.Fatalf("failed to reset user usable groups: %v", err)
		}
		if err := setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
		if err := setting.UpdateAutoGroupsByJsonString(`["default"]`); err != nil {
			t.Fatalf("failed to reset auto groups: %v", err)
		}
	})

	if err := setting.UpdateUserUsableGroupsByJSONString(`{
		"default": "默认分组",
		"vip": "vip分组",
		"backup": "备用分组"
	}`); err != nil {
		t.Fatalf("failed to update user usable groups: %v", err)
	}
	if err := setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "backup", "order": 10},
			{"group": "default", "order": 20},
			{"group": "vip", "order": 30, "user_group": true}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}
	if err := setting.UpdateAutoGroupsByJsonString(`[]`); err != nil {
		t.Fatalf("failed to clear auto groups: %v", err)
	}

	groups := GetUserAutoGroup("default")
	if len(groups) != 2 || groups[0] != "backup" || groups[1] != "default" {
		t.Fatalf("expected default auto groups to follow visible token groups, got %+v", groups)
	}
}
