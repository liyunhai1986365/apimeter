package setting

import (
	"testing"
)

func TestNormalizeGroupDisplayConfig(t *testing.T) {
	config := NormalizeGroupDisplayConfig(GroupDisplayConfig{
		Categories: []GroupDisplayCategory{
			{ID: " partner ", Name: "Partner", Order: 20},
			{ID: "official", Name: "", Order: 10},
			{ID: "partner", Name: "Duplicate", Order: 30},
			{ID: " ", Name: "Blank", Order: 1},
		},
		Groups: []GroupDisplayGroup{
			{Group: " vip ", CategoryID: "partner", Order: 20, UserGroup: true},
			{Group: "default", CategoryID: "official", Order: 10},
			{Group: "vip", CategoryID: "official", Order: 30},
			{Group: "orphan", CategoryID: "missing", Order: 5, UserGroup: true},
			{Group: "", CategoryID: "partner", Order: 1},
		},
	})

	if len(config.Categories) != 2 {
		t.Fatalf("expected 2 categories, got %d", len(config.Categories))
	}
	if config.Categories[0].ID != "official" || config.Categories[0].Name != "official" {
		t.Fatalf("expected first category to be official with fallback name, got %+v", config.Categories[0])
	}
	if config.Categories[1].ID != "partner" || config.Categories[1].Name != "Partner" {
		t.Fatalf("expected second category to be partner, got %+v", config.Categories[1])
	}

	if len(config.Groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(config.Groups))
	}
	if config.Groups[0].Group != "default" {
		t.Fatalf("expected default first by category order, got %+v", config.Groups)
	}
	if config.Groups[1].Group != "vip" || config.Groups[1].CategoryID != "partner" {
		t.Fatalf("expected vip to keep first category assignment, got %+v", config.Groups[1])
	}
	if !config.Groups[1].UserGroup {
		t.Fatalf("expected vip to keep user group flag, got %+v", config.Groups[1])
	}
	if config.Groups[2].Group != "orphan" || config.Groups[2].CategoryID != "" {
		t.Fatalf("expected unknown category to be cleared, got %+v", config.Groups[2])
	}
	if !config.Groups[2].UserGroup {
		t.Fatalf("expected orphan to keep user group flag, got %+v", config.Groups[2])
	}
}

func TestGetUserGroupNamesFromDisplayConfig(t *testing.T) {
	t.Cleanup(func() {
		if err := UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})

	if err := UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "default", "order": 10, "user_group": true},
			{"group": "token-only", "order": 20},
			{"group": "vip", "order": 30, "user_group": true}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	groups, configured := GetUserGroupNamesFromDisplayConfig()
	if !configured {
		t.Fatalf("expected display config with groups to be treated as configured")
	}
	if len(groups) != 2 {
		t.Fatalf("expected 2 user groups, got %d: %+v", len(groups), groups)
	}
	if groups[0] != "default" || groups[1] != "vip" {
		t.Fatalf("expected ordered user groups [default vip], got %+v", groups)
	}
}

func TestGetUserGroupNamesFromDisplayConfigReturnsConfiguredEmptyList(t *testing.T) {
	t.Cleanup(func() {
		if err := UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})

	if err := UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "default", "order": 10},
			{"group": "token-only", "order": 20}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	groups, configured := GetUserGroupNamesFromDisplayConfig()
	if !configured {
		t.Fatalf("expected display config with groups to be treated as configured")
	}
	if len(groups) != 0 {
		t.Fatalf("expected no user groups when none are flagged, got %+v", groups)
	}
}

func TestGetTokenGroupNamesFromDisplayConfigExcludesUserGroups(t *testing.T) {
	t.Cleanup(func() {
		if err := UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`); err != nil {
			t.Fatalf("failed to reset group display config: %v", err)
		}
	})

	if err := UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "backup", "order": 10},
			{"group": "default", "order": 20},
			{"group": "vip", "order": 30, "user_group": true}
		]
	}`); err != nil {
		t.Fatalf("failed to update group display config: %v", err)
	}

	groups, configured := GetTokenGroupNamesFromDisplayConfig()
	if !configured {
		t.Fatalf("expected display config with groups to be treated as configured")
	}
	if len(groups) != 2 || groups[0] != "backup" || groups[1] != "default" {
		t.Fatalf("expected ordered token groups [backup default], got %+v", groups)
	}
}
