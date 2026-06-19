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
			{Group: " vip ", CategoryID: "partner", Order: 20},
			{Group: "default", CategoryID: "official", Order: 10},
			{Group: "vip", CategoryID: "official", Order: 30},
			{Group: "orphan", CategoryID: "missing", Order: 5},
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
	if config.Groups[2].Group != "orphan" || config.Groups[2].CategoryID != "" {
		t.Fatalf("expected unknown category to be cleared, got %+v", config.Groups[2])
	}
}
