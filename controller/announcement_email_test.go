package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnnouncementEmailControllerTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originRedisEnabled := common.RedisEnabled

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AgentUser{}, &model.Log{}))

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColForTest()
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.RedisEnabled = originRedisEnabled
		model.InitColForTest()
	})
}

func TestSendAnnouncementEmailReturnsBroadcastSummary(t *testing.T) {
	setupAnnouncementEmailControllerTestDB(t)

	require.NoError(t, model.DB.Create(&[]model.User{
		{Id: 1, Username: "first", Password: "password123", Email: "first@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-1"},
		{Id: 2, Username: "empty", Password: "password123", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-2"},
		{Id: 3, Username: "agent", Password: "password123", Email: "agent@example.com", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Group: "default", AffCode: "aff-3"},
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{AgentId: 10, UserId: 3, Status: model.AgentUserStatusEnabled}).Error)

	originSender := service.AnnouncementEmailSenderFunc
	var gotSubject, gotReceiver, gotContent string
	service.AnnouncementEmailSenderFunc = func(subject string, receiver string, content string) error {
		gotSubject = subject
		gotReceiver = receiver
		gotContent = content
		return nil
	}
	t.Cleanup(func() {
		service.AnnouncementEmailSenderFunc = originSender
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 9)
	c.Set("username", "root")
	c.Request = httptest.NewRequest(http.MethodPost, "/api/option/announcements/email", strings.NewReader(`{"title":"模型发布","content":"## 新增模型\n\n新增模型已上线","type":"model_release","audience":"main_site"}`))

	SendAnnouncementEmail(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"success":true,"message":"","data":{"total":1,"sent":1,"failed":0}}`, recorder.Body.String())
	require.Equal(t, "模型发布", gotSubject)
	require.Equal(t, "first@example.com", gotReceiver)
	require.Contains(t, gotContent, "<h2>新增模型</h2>")
	require.Contains(t, gotContent, "新增模型已上线")
	require.Contains(t, gotContent, "announcement-type:model_release")
}

func TestSendAnnouncementEmailRejectsMissingContent(t *testing.T) {
	setupAnnouncementEmailControllerTestDB(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/option/announcements/email", strings.NewReader(`{"title":"模型发布"}`))

	SendAnnouncementEmail(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "common.invalid_params")
	require.Contains(t, recorder.Body.String(), `"success":false`)
}
