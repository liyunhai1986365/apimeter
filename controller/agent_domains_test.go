package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAgentDomainCreationSupportsBatchAndLegacyRequests(t *testing.T) {
	agent := setupAgentViewContextTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AgentDomain{}))
	router := gin.New()
	router.POST("/api/agents/:id/domains", AgentCreateDomain)
	router.POST("/api/agents/:id/domains/batch", AgentCreateDomains)
	router.POST("/api/agent/domains/batch", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{AgentID: agent.Id})
	}, AgentCreateDomains)

	request := func(path, body string) []byte {
		t.Helper()
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, req)
		require.Equal(t, http.StatusOK, recorder.Code)
		return recorder.Body.Bytes()
	}
	var single struct {
		Success bool              `json:"success"`
		Data    model.AgentDomain `json:"data"`
	}
	require.NoError(t, common.Unmarshal(request(fmt.Sprintf("/api/agents/%d/domains", agent.Id), `{"domain":"old.example.com"}`), &single))
	require.True(t, single.Success)
	require.Equal(t, "old.example.com", single.Data.Domain)
	for i, path := range []string{fmt.Sprintf("/api/agents/%d/domains/batch", agent.Id), "/api/agent/domains/batch"} {
		var batch struct {
			Success bool                `json:"success"`
			Data    []model.AgentDomain `json:"data"`
		}
		body := fmt.Sprintf(`{"agent_id":999,"domains":["site%d.example.com","api%d.example.com"]}`, i, i)
		require.NoError(t, common.Unmarshal(request(path, body), &batch))
		require.True(t, batch.Success)
		require.Len(t, batch.Data, 2)
		for _, domain := range batch.Data {
			require.Equal(t, agent.Id, domain.AgentId)
			require.Equal(t, model.AgentDomainStatusActive, domain.Status)
		}
	}
	var failure struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(request("/api/agent/domains/batch", `{"domains":["new.example.com","old.example.com"]}`), &failure))
	require.False(t, failure.Success)
	domains, total, err := model.ListAgentDomains(agent.Id, 0, 2)
	require.NoError(t, err)
	require.EqualValues(t, 5, total)
	require.Len(t, domains, 2)
}
