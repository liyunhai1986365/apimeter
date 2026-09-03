package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserEmailControllerTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originRedisEnabled := common.RedisEnabled

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Log{}, &model.Agent{}, &model.AgentUser{}, &model.AgentDomain{}))

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

func TestSendAdminUserEmailSendsToTargetUserEmail(t *testing.T) {
	setupUserEmailControllerTestDB(t)

	require.NoError(t, model.DB.Create(&model.User{
		Id:          7,
		Username:    "target-user",
		Password:    "password123",
		DisplayName: "Target",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Email:       "target@example.com",
		Group:       "default",
	}).Error)

	var gotSubject, gotReceiver, gotContent string
	originSender := adminUserEmailSender
	adminUserEmailSender = func(subject string, receiver string, content string) error {
		gotSubject = subject
		gotReceiver = receiver
		gotContent = content
		return nil
	}
	t.Cleanup(func() {
		adminUserEmailSender = originSender
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleAdminUser)
	c.Set("id", 1)
	c.Set("username", "admin")
	c.Params = gin.Params{{Key: "id", Value: "7"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/7/email", strings.NewReader(`{"subject":" Hello ","content":"<p>Custom notice</p>"}`))

	SendAdminUserEmail(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"success":true,"message":""}`, recorder.Body.String())
	require.Equal(t, "Hello", gotSubject)
	require.Equal(t, "target@example.com", gotReceiver)
	require.Equal(t, "<p>Custom notice</p>", gotContent)
}

func TestSendAdminUserEmailRejectsUserWithoutEmail(t *testing.T) {
	setupUserEmailControllerTestDB(t)

	require.NoError(t, model.DB.Create(&model.User{
		Id:          8,
		Username:    "no-email",
		Password:    "password123",
		DisplayName: "No Email",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
	}).Error)

	called := false
	originSender := adminUserEmailSender
	adminUserEmailSender = func(subject string, receiver string, content string) error {
		called = true
		return nil
	}
	t.Cleanup(func() {
		adminUserEmailSender = originSender
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleAdminUser)
	c.Params = gin.Params{{Key: "id", Value: "8"}}
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/8/email", strings.NewReader(`{"subject":"Notice","content":"Body"}`))

	SendAdminUserEmail(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "用户未绑定邮箱")
	require.False(t, called)
}

func TestSendAdminUserEmailRewritesMainSiteLinksForAgentUser(t *testing.T) {
	setupUserEmailControllerTestDB(t)
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	require.NoError(t, model.DB.Create(&model.User{
		Id: 9, Username: "agent-user", Password: "password123", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Email: "agent@example.com", Group: "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Agent{Id: 3, Name: "Agent", Slug: "admin-email-agent", Status: model.AgentStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{AgentId: 3, UserId: 9, Status: model.AgentUserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{AgentId: 3, Domain: "fallback.example.com", Status: model.AgentDomainStatusActive}).Error)

	var gotContent string
	originSender := adminUserEmailSender
	adminUserEmailSender = func(_, _ string, content string) error {
		gotContent = content
		return nil
	}
	t.Cleanup(func() { adminUserEmailSender = originSender })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("role", common.RoleAdminUser)
	c.Set("id", 1)
	c.Params = gin.Params{{Key: "id", Value: "9"}}
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{AgentID: 3, Domain: "request.example.com"})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/9/email", strings.NewReader(`{"subject":"Notice","content":"<a href='https://modelsell.com/wallet'>Wallet</a> <a href='https://external.example'>External</a>"}`))
	c.Request.Host = "request.example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	SendAdminUserEmail(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, gotContent, "https://request.example.com/wallet")
	require.Contains(t, gotContent, "https://external.example")
}
