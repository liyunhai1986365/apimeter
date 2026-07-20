package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentViewContextTestDB(t *testing.T) *model.Agent {
	t.Helper()
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:agent-view-context-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	require.NoError(t, model.DB.AutoMigrate(&model.Agent{}, &model.AgentLedger{}, &model.AgentWithdrawal{}))
	agent := &model.Agent{
		OwnerUserId:   1001,
		Name:          "Selected Agent",
		Slug:          "selected-agent",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}
	require.NoError(t, model.DB.Create(agent).Error)
	t.Cleanup(func() { model.DB = originalDB })
	return agent
}

func newAgentViewContextTestRouter(role int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("agent-view-context-test"))))
	router.Use(func(c *gin.Context) {
		c.Set("id", 999)
		c.Set("role", role)
		c.Next()
	})
	router.POST("/switch", SwitchAgentViewContext)
	router.GET("/current", func(c *gin.Context) {
		id, ok := currentAgentID(c)
		c.JSON(http.StatusOK, gin.H{"id": id, "ok": ok})
	})
	router.GET("/agents", AdminListAgents)
	return router
}

func TestAgentViewContextSwitchScopesFollowingRequests(t *testing.T) {
	agent := setupAgentViewContextTestDB(t)
	router := newAgentViewContextTestRouter(common.RoleAdminUser)
	body, err := common.Marshal(agentViewRequest{AgentId: agent.Id})
	require.NoError(t, err)

	switchRecorder := httptest.NewRecorder()
	switchRequest := httptest.NewRequest(http.MethodPost, "/switch", bytes.NewReader(body))
	switchRequest.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(switchRecorder, switchRequest)
	require.Equal(t, http.StatusOK, switchRecorder.Code)
	require.NotEmpty(t, switchRecorder.Result().Cookies())

	currentRecorder := httptest.NewRecorder()
	currentRequest := httptest.NewRequest(http.MethodGet, "/current", nil)
	for _, sessionCookie := range switchRecorder.Result().Cookies() {
		currentRequest.AddCookie(sessionCookie)
	}
	router.ServeHTTP(currentRecorder, currentRequest)
	require.Equal(t, http.StatusOK, currentRecorder.Code)
	require.Contains(t, currentRecorder.Body.String(), fmt.Sprintf("\"id\":%d", agent.Id))
}

func TestAgentViewContextRejectsOrdinaryUsers(t *testing.T) {
	agent := setupAgentViewContextTestDB(t)
	router := newAgentViewContextTestRouter(common.RoleCommonUser)
	body, err := common.Marshal(agentViewRequest{AgentId: agent.Id})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/switch", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestAdminListAgentsIncludesAvailableBalance(t *testing.T) {
	agent := setupAgentViewContextTestDB(t)
	require.NoError(t, model.DB.Create(&model.AgentLedger{
		AgentId:      agent.Id,
		Type:         model.AgentLedgerTypeAdjustment,
		ProfitQuota:  1000,
		BalanceAfter: 1000,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentWithdrawal{
		AgentId:     agent.Id,
		AmountQuota: 250,
		Status:      model.AgentWithdrawalStatusPending,
	}).Error)
	router := newAgentViewContextTestRouter(common.RoleAdminUser)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/agents?p=1&page_size=20", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"available_quota":750`)
	require.Contains(t, recorder.Body.String(), `"pending_withdrawal_quota":250`)
}
