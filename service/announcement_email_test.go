package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
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
	require.NoError(t, db.AutoMigrate(&model.User{}))

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
