package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAgentTLSAskControllerTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	t.Setenv("AGENT_TLS_ASK_SECRET", "")
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap["AgentTLSAskSecret"] = ""
	common.OptionMapRWMutex.Unlock()

	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(
		&model.Agent{},
		&model.AgentDomain{},
	))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestAgentDomainTLSAskAllowsActiveAgentDomain(t *testing.T) {
	setupAgentTLSAskControllerTestDB(t)
	require.NoError(t, model.DB.Create(&model.Agent{
		Id:            1,
		OwnerUserId:   10,
		Name:          "Agent One",
		Slug:          "agent-one",
		Status:        model.AgentStatusEnabled,
		PriceMode:     model.AgentPriceModeMultiplier,
		DefaultMarkup: 1,
	}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{
		AgentId: 1,
		Domain:  "api.customer.com",
		Status:  model.AgentDomainStatusActive,
	}).Error)

	router := gin.New()
	router.GET("/api/agent/domains/tls-ask", AgentDomainTLSAsk)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/domains/tls-ask?domain=api.customer.com", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusOK, resp.Code)
}

func TestAgentDomainTLSAskRejectsUnknownDomain(t *testing.T) {
	setupAgentTLSAskControllerTestDB(t)

	router := gin.New()
	router.GET("/api/agent/domains/tls-ask", AgentDomainTLSAsk)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/domains/tls-ask?domain=missing.customer.com", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	require.Equal(t, http.StatusForbidden, resp.Code)
}
