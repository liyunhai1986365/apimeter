package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTokenAuthAllowsSameKeyOnEveryDomainOfItsAgent(t *testing.T) {
	db := openTokenAuthTestDB(t)
	seedTokenAuthAutoGroupFixture(t, db)
	require.NoError(t, db.AutoMigrate(&model.Agent{}, &model.AgentDomain{}, &model.AgentUser{}, &model.AgentGroupRatio{}, &model.AgentUserGroupConfig{}))
	agentservice.InvalidateAllDomainResolutions()
	t.Cleanup(agentservice.InvalidateAllDomainResolutions)
	agent := &model.Agent{Name: "Agent", Slug: "agent", Status: model.AgentStatusEnabled, DefaultMarkup: 1}
	other := &model.Agent{Name: "Other", Slug: "other", Status: model.AgentStatusEnabled, DefaultMarkup: 1}
	require.NoError(t, db.Create(agent).Error)
	require.NoError(t, db.Create(other).Error)
	domains, err := agentservice.CreateDomains(agent.Id, []string{"site.example.com", "api.example.com"})
	require.NoError(t, err)
	_, err = agentservice.CreateDomain(other.Id, "other.example.com")
	require.NoError(t, err)
	require.NoError(t, model.BindUserToAgent(agent.Id, 1, model.AgentUserSourceDomain))

	router := gin.New()
	router.Use(AgentResolver())
	router.POST("/v1/chat/completions", TokenAuth(), func(c *gin.Context) {
		ctx, _ := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
		require.NotNil(t, ctx)
		require.Equal(t, agent.Id, ctx.AgentID)
		require.Equal(t, 1, c.GetInt("id"))
		c.Status(http.StatusNoContent)
	})
	request := func(host string, status int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "https://"+host+"/v1/chat/completions", nil)
		req.Header.Set("Authorization", "Bearer sk-autotokenkey")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		require.Equal(t, status, recorder.Code, recorder.Body.String())
	}
	for _, domain := range domains {
		request(domain.Domain, http.StatusNoContent)
	}
	request("API.Example.com:443", http.StatusNoContent)
	request("other.example.com", http.StatusForbidden)
	require.NoError(t, db.Model(&model.AgentUser{}).Where("user_id = ?", 1).Update("status", model.AgentUserStatusDisabled).Error)
	for _, domain := range domains {
		request(domain.Domain, http.StatusForbidden)
	}
}
