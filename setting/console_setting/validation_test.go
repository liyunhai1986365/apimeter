package console_setting

import (
	"testing"
)

func TestValidateAnnouncementsAcceptsNewAnnouncementTypesAndTitle(t *testing.T) {
	raw := `[
		{"title":"产品通知","content":"新增控制台功能","publishDate":"2026-07-09T05:00:00.000Z","type":"product_update","audience":"all"},
		{"title":"系统维护","content":"计划维护窗口","publishDate":"2026-07-09T05:00:00.000Z","type":"system_maintenance"},
		{"title":"模型发布","content":"新增模型已上线","publishDate":"2026-07-09T05:00:00.000Z","type":"model_release","audience":"main_site"},
		{"title":"价格调整","content":"套餐价格更新","publishDate":"2026-07-09T05:00:00.000Z","type":"pricing_update"},
		{"title":"故障通知","content":"服务异常恢复","publishDate":"2026-07-09T05:00:00.000Z","type":"incident"},
		{"title":"普通公告","content":"平台公告","publishDate":"2026-07-09T05:00:00.000Z","type":"general"}
	]`

	if err := ValidateConsoleSettings(raw, "Announcements"); err != nil {
		t.Fatalf("expected new announcement types to be valid, got %v", err)
	}
}

func TestValidateAnnouncementsRejectsInvalidAudience(t *testing.T) {
	raw := `[{"title":"范围错误","content":"公告内容","publishDate":"2026-07-09T05:00:00.000Z","type":"general","audience":"agent_only"}]`
	if err := ValidateConsoleSettings(raw, "Announcements"); err == nil {
		t.Fatal("expected invalid announcement audience to be rejected")
	}
}

func TestFilterAnnouncementsForAgentSite(t *testing.T) {
	announcements := []map[string]interface{}{
		{"id": float64(1), "title": "legacy"},
		{"id": float64(2), "title": "all", "audience": AnnouncementAudienceAll},
		{"id": float64(3), "title": "main", "audience": AnnouncementAudienceMainSite},
	}

	filtered := FilterAnnouncementsForAgentSite(announcements)
	if len(filtered) != 2 || filtered[0]["title"] != "legacy" || filtered[1]["title"] != "all" {
		t.Fatalf("unexpected filtered announcements: %#v", filtered)
	}
}

func TestValidateAnnouncementsRejectsLegacyTypeAndMissingTitle(t *testing.T) {
	legacy := `[{"title":"旧类型","content":"旧公告","publishDate":"2026-07-09T05:00:00.000Z","type":"model_update"}]`
	if err := ValidateConsoleSettings(legacy, "Announcements"); err == nil {
		t.Fatal("expected legacy model_update announcement type to be rejected")
	}

	missingTitle := `[{"content":"没有标题","publishDate":"2026-07-09T05:00:00.000Z","type":"general"}]`
	if err := ValidateConsoleSettings(missingTitle, "Announcements"); err == nil {
		t.Fatal("expected announcement without title to be rejected")
	}
}
