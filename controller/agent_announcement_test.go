package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentAnnouncementControllerTestDB(t *testing.T) {
	t.Helper()
	originDB := model.DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.AgentAnnouncement{}))
	model.DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.DB = originDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		model.InitColForTest()
	})
}

func TestAgentAnnouncementCRUDUsesCurrentAgentScope(t *testing.T) {
	setupAgentAnnouncementControllerTestDB(t)
	publishAt := time.Now().Unix()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(currentAgentIDKey, 7)
	c.Set("id", 101)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/agent/announcements", strings.NewReader(`{"title":" Agent notice ","content":" Hello users ","type":"general","extra":" Note ","publish_at":`+strconvFormatInt(publishAt)+`,"enabled":true}`))

	AgentCreateAnnouncement(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	require.Equal(t, true, body["success"])
	announcement := body["data"].(map[string]interface{})
	require.Equal(t, "Agent notice", announcement["title"])
	require.Equal(t, float64(7), announcement["agent_id"])

	other := &model.AgentAnnouncement{
		AgentId:   8,
		Title:     "Other agent",
		Content:   "Private",
		Type:      "general",
		PublishAt: publishAt,
		Enabled:   true,
	}
	require.NoError(t, model.DB.Create(other).Error)

	updateRecorder := httptest.NewRecorder()
	updateContext, _ := gin.CreateTestContext(updateRecorder)
	updateContext.Set(currentAgentIDKey, 7)
	updateContext.Set("id", 101)
	updateContext.Params = gin.Params{{Key: "announcement_id", Value: strconvFormatInt(int64(other.Id))}}
	updateContext.Request = httptest.NewRequest(http.MethodPut, "/api/agent/announcements/other", strings.NewReader(`{"title":"Changed","content":"Changed","type":"general","publish_at":`+strconvFormatInt(publishAt)+`,"enabled":true}`))

	AgentUpdateAnnouncement(updateContext)

	require.Contains(t, updateRecorder.Body.String(), "agent announcement not found")
	require.NoError(t, model.DB.First(other, other.Id).Error)
	require.Equal(t, "Other agent", other.Title)
}

func TestGetStatusShowsOnlyPublishedAnnouncementForCurrentAgent(t *testing.T) {
	setupAgentAnnouncementControllerTestDB(t)
	consoleSetting := console_setting.GetConsoleSetting()
	oldEnabled := consoleSetting.AnnouncementsEnabled
	oldAnnouncements := consoleSetting.Announcements
	consoleSetting.AnnouncementsEnabled = false
	consoleSetting.Announcements = ""
	t.Cleanup(func() {
		consoleSetting.AnnouncementsEnabled = oldEnabled
		consoleSetting.Announcements = oldAnnouncements
	})
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&[]model.AgentAnnouncement{
		{AgentId: 7, Title: "Agent seven", Content: "Visible", Type: "general", PublishAt: now - 1, Enabled: true},
		{AgentId: 7, Title: "Scheduled", Content: "Future", Type: "general", PublishAt: now + 3600, Enabled: true},
		{AgentId: 8, Title: "Agent eight", Content: "Private", Type: "general", PublishAt: now - 1, Enabled: true},
	}).Error)

	router := gin.New()
	router.GET("/api/status", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{AgentID: 7, Domain: "agent.example.com"})
		GetStatus(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	data := body["data"].(map[string]interface{})
	require.Equal(t, true, data["announcements_enabled"])
	announcements := data["announcements"].([]interface{})
	require.Len(t, announcements, 1)
	require.Equal(t, "Agent seven", announcements[0].(map[string]interface{})["title"])
	require.Equal(t, "agent", announcements[0].(map[string]interface{})["source"])
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}
