package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openChannelRouteAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Channel{}, &model.Ability{}, &model.AuthzRole{}, &model.CasbinRule{}))
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColForTest()
	require.NoError(t, authz.Init(db))
	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.RedisEnabled = originRedisEnabled
		model.InitColForTest()
	})
	return db
}

func TestChannelUpdateRequiresRootRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openChannelRouteAuthTestDB(t)

	accessToken := "admin-access-token"
	require.NoError(t, db.Create(&model.User{
		Id:          10,
		Username:    "delegated-admin",
		Password:    "password",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleAdminUser,
		Group:       "default",
		AccessToken: &accessToken,
	}).Error)
	originalBaseURL := "https://safe-upstream.example.com"
	require.NoError(t, db.Create(&model.Channel{
		Id:          20,
		Type:        constant.ChannelTypeOpenAI,
		Key:         "sk-real-upstream-key",
		Status:      common.ChannelStatusEnabled,
		Name:        "openai-main",
		BaseURL:     &originalBaseURL,
		Models:      "gpt-test",
		Group:       "default",
		CreatedTime: common.GetTimestamp(),
	}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("channel-route-auth-test"))))
	SetApiRouter(engine)

	body := `{
		"id":20,
		"type":1,
		"key":"sk-real-upstream-key",
		"name":"openai-main",
		"base_url":"https://attacker.example.com",
		"models":"gpt-test",
		"group":"default"
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "10")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.False(t, response.Success, recorder.Body.String())
	require.NotEmpty(t, response.Message)

	var channel model.Channel
	require.NoError(t, db.First(&channel, 20).Error)
	require.NotNil(t, channel.BaseURL)
	require.Equal(t, originalBaseURL, *channel.BaseURL)
}

func TestChannelUpdateAllowsRootRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := openChannelRouteAuthTestDB(t)

	accessToken := "root-access-token"
	require.NoError(t, db.Create(&model.User{
		Id:          11,
		Username:    "root-admin",
		Password:    "password",
		Status:      common.UserStatusEnabled,
		Role:        common.RoleRootUser,
		Group:       "default",
		AccessToken: &accessToken,
	}).Error)
	originalBaseURL := "https://safe-upstream.example.com"
	nextBaseURL := "https://new-safe-upstream.example.com"
	require.NoError(t, db.Create(&model.Channel{
		Id:          21,
		Type:        constant.ChannelTypeOpenAI,
		Key:         "sk-real-upstream-key",
		Status:      common.ChannelStatusEnabled,
		Name:        "openai-main",
		BaseURL:     &originalBaseURL,
		Models:      "gpt-test",
		Group:       "default",
		CreatedTime: common.GetTimestamp(),
	}).Error)

	engine := gin.New()
	engine.Use(sessions.Sessions("session", cookie.NewStore([]byte("channel-route-auth-test"))))
	SetApiRouter(engine)

	body := `{
		"id":21,
		"type":1,
		"key":"sk-real-upstream-key",
		"name":"openai-main",
		"base_url":"https://new-safe-upstream.example.com",
		"models":"gpt-test",
		"group":"default"
	}`
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/channel/", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("New-Api-User", "11")

	engine.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success, recorder.Body.String())
	require.Empty(t, response.Message)

	var channel model.Channel
	require.NoError(t, db.First(&channel, 21).Error)
	require.NotNil(t, channel.BaseURL)
	require.Equal(t, nextBaseURL, *channel.BaseURL)
}
