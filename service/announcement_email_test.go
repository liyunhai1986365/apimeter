package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnnouncementEmailTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Agent{}, &model.AgentUser{}, &model.AgentDomain{}))

	model.DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()

	t.Cleanup(func() {
		model.DB = originDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		model.InitColForTest()
	})
}

func TestBroadcastAnnouncementEmailSendsToEnabledUsersWithEmail(t *testing.T) {
	setupAnnouncementEmailTestDB(t)

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "first", Password: "password123", Email: "first@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-1"},
		{Id: 2, Username: "duplicate", Password: "password123", Email: " first@example.com ", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-2"},
		{Id: 3, Username: "second", Password: "password123", Email: "second@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-3"},
		{Id: 4, Username: "disabled", Password: "password123", Email: "disabled@example.com", Status: common.UserStatusDisabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-4"},
		{Id: 5, Username: "empty", Password: "password123", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-5"},
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{AgentId: 10, UserId: 3, Status: model.AgentUserStatusEnabled}).Error)

	var receivers []string
	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:   "模型发布公告",
		Content: "## 新增模型\n\n- gpt-new\n\n[查看详情](https://example.com/models)",
		Type:    "model_release",
		Send: func(subject string, receiver string, content string) error {
			receivers = append(receivers, receiver)
			require.Equal(t, "模型发布公告", subject)
			require.Contains(t, content, "<h2>新增模型</h2>")
			require.Contains(t, content, "<li>gpt-new</li>")
			require.Contains(t, content, `<a href="https://example.com/models">查看详情</a>`)
			require.Contains(t, content, "announcement-type:model_release")
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, summary.Total)
	require.Equal(t, 2, summary.Sent)
	require.Equal(t, 0, summary.Failed)
	require.Equal(t, []string{"first@example.com", "second@example.com"}, receivers)
}

func TestBroadcastAnnouncementEmailCountsSendFailures(t *testing.T) {
	setupAnnouncementEmailTestDB(t)

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "first", Password: "password123", Email: "first@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-1"},
		{Id: 2, Username: "second", Password: "password123", Email: "second@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-2"},
	}).Error)

	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:   "系统维护",
		Content: "维护通知",
		Send: func(_ string, receiver string, _ string) error {
			if receiver == "second@example.com" {
				return errors.New("smtp failed")
			}
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, summary.Total)
	require.Equal(t, 1, summary.Sent)
	require.Equal(t, 1, summary.Failed)
	require.Len(t, summary.Errors, 1)
	require.Contains(t, summary.Errors[0], "second@example.com")
}

func TestBroadcastAnnouncementEmailOnlySendsToSelectedUserGroups(t *testing.T) {
	setupAnnouncementEmailTestDB(t)

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "default", Password: "password123", Email: "default@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-default"},
		{Id: 2, Username: "vip", Password: "password123", Email: "vip@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "vip", AffCode: "aff-vip"},
		{Id: 3, Username: "premium", Password: "password123", Email: "premium@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "premium", AffCode: "aff-premium"},
	}).Error)

	var receivers []string
	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:        "分组公告",
		Content:      "仅指定分组接收",
		TargetGroups: []string{" vip ", "premium", "vip"},
		Send: func(_ string, receiver string, _ string) error {
			receivers = append(receivers, receiver)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, summary.Total)
	require.Equal(t, []string{"premium@example.com", "vip@example.com"}, receivers)
}

func TestBroadcastAnnouncementEmailMainSiteAudienceExcludesAgentUsers(t *testing.T) {
	setupAnnouncementEmailTestDB(t)

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "main", Password: "password123", Email: "main@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-main"},
		{Id: 2, Username: "agent-enabled", Password: "password123", Email: "agent-enabled@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-agent-enabled"},
		{Id: 3, Username: "agent-disabled", Password: "password123", Email: "agent-disabled@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-agent-disabled"},
		{Id: 4, Username: "duplicate-agent-email", Password: "password123", Email: "agent-enabled@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-duplicate-agent-email"},
	}).Error)
	require.NoError(t, model.DB.Create(&[]model.AgentUser{
		{AgentId: 10, UserId: 2, Status: model.AgentUserStatusEnabled},
		{AgentId: 10, UserId: 3, Status: model.AgentUserStatusDisabled},
	}).Error)

	var receivers []string
	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:    "主站公告",
		Content:  "仅主站用户接收",
		Audience: console_setting.AnnouncementAudienceMainSite,
		Send: func(_ string, receiver string, _ string) error {
			receivers = append(receivers, receiver)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 1, summary.Total)
	require.Equal(t, []string{"main@example.com"}, receivers)
}

func TestBroadcastAnnouncementEmailUsesEachAgentUsersSiteDomain(t *testing.T) {
	setupAnnouncementEmailTestDB(t)
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "main", Password: "password123", Email: "main@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "agent-link-main"},
		{Id: 2, Username: "agent", Password: "password123", Email: "agent@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "agent-link-user"},
	}).Error)
	require.NoError(t, model.DB.Create(&model.Agent{Id: 10, Name: "Agent", Slug: "announcement-link-agent", Status: model.AgentStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{AgentId: 10, UserId: 2, Status: model.AgentUserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{AgentId: 10, Domain: "agent.example.com", Status: model.AgentDomainStatusActive}).Error)

	contents := make(map[string]string)
	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:   "链接公告",
		Content: "[查看详情](https://modelsell.com/pricing) [外部文档](https://docs.example.com/guide)",
		Send: func(_ string, receiver string, content string) error {
			contents[receiver] = content
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, summary.Sent)
	require.Contains(t, contents["main@example.com"], `href="https://modelsell.com/pricing"`)
	require.Contains(t, contents["agent@example.com"], `href="https://agent.example.com/pricing"`)
	require.Contains(t, contents["agent@example.com"], `href="https://docs.example.com/guide"`)
}

func TestBroadcastAnnouncementEmailRejectsLegacyAnnouncementType(t *testing.T) {
	setupAnnouncementEmailTestDB(t)

	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:   "旧类型",
		Content: "旧公告",
		Type:    "model_update",
		Send: func(_ string, _ string, _ string) error {
			t.Fatal("sender should not be called for invalid announcement type")
			return nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid announcement type")
	require.Equal(t, BroadcastAnnouncementEmailSummary{}, summary)
}

func TestBroadcastAnnouncementEmailRejectsInvalidAudience(t *testing.T) {
	setupAnnouncementEmailTestDB(t)

	summary, err := BroadcastAnnouncementEmail(BroadcastAnnouncementEmailRequest{
		Title:    "范围错误",
		Content:  "公告内容",
		Audience: "agent_only",
		Send: func(_ string, _ string, _ string) error {
			t.Fatal("sender should not be called for invalid announcement audience")
			return nil
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid announcement audience")
	require.Equal(t, BroadcastAnnouncementEmailSummary{}, summary)
}

func TestBroadcastAgentAnnouncementEmailOnlySendsToEnabledUsersInAgent(t *testing.T) {
	setupAnnouncementEmailTestDB(t)
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "first", Password: "password123", Email: "first@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "agent-first"},
		{Id: 2, Username: "duplicate", Password: "password123", Email: " FIRST@example.com ", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "agent-duplicate"},
		{Id: 3, Username: "second", Password: "password123", Email: "second@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "agent-second"},
		{Id: 4, Username: "other-agent", Password: "password123", Email: "other@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "other-agent"},
		{Id: 5, Username: "disabled-membership", Password: "password123", Email: "membership-disabled@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "membership-disabled"},
		{Id: 6, Username: "disabled-user", Password: "password123", Email: "user-disabled@example.com", Status: common.UserStatusDisabled, Role: common.RoleCommonUser, Group: "default", AffCode: "user-disabled"},
	}).Error)
	require.NoError(t, model.DB.Create(&[]model.AgentUser{
		{AgentId: 10, UserId: 1, Status: model.AgentUserStatusEnabled},
		{AgentId: 10, UserId: 2, Status: model.AgentUserStatusEnabled},
		{AgentId: 10, UserId: 3, Status: model.AgentUserStatusEnabled},
		{AgentId: 11, UserId: 4, Status: model.AgentUserStatusEnabled},
		{AgentId: 10, UserId: 5, Status: model.AgentUserStatusDisabled},
		{AgentId: 10, UserId: 6, Status: model.AgentUserStatusEnabled},
	}).Error)

	var receivers []string
	summary, err := BroadcastAgentAnnouncementEmail(BroadcastAgentAnnouncementEmailRequest{
		AgentID:   10,
		AgentName: "Agent <One>",
		Title:     "代理公告",
		Content:   "## 新内容\n\n[查看价格](https://modelsell.com/pricing) [外部链接](https://external.example/docs)",
		Type:      "general",
		SiteURL:   "https://agent.example.com",
		Send: func(subject string, receiver string, content string) error {
			receivers = append(receivers, receiver)
			require.Equal(t, "代理公告", subject)
			require.Contains(t, content, "announcement-source:agent:10")
			require.Contains(t, content, "Agent &lt;One&gt;")
			require.Contains(t, content, "<h2>新内容</h2>")
			require.Contains(t, content, `href="https://agent.example.com/pricing"`)
			require.Contains(t, content, `href="https://external.example/docs"`)
			return nil
		},
	})

	require.NoError(t, err)
	require.Equal(t, 2, summary.Total)
	require.Equal(t, 2, summary.Sent)
	require.Equal(t, []string{"first@example.com", "second@example.com"}, receivers)
}
