package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetStatusUsesAgentDomainForDisplayedServerAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldApiInfoEnabled := console_setting.GetConsoleSetting().ApiInfoEnabled
	oldApiInfo := console_setting.GetConsoleSetting().ApiInfo
	console_setting.GetConsoleSetting().ApiInfoEnabled = true
	console_setting.GetConsoleSetting().ApiInfo = `[{"url":"https://main.example.com/v1/chat/completions","route":"main","description":"main api","color":"blue"}]`
	t.Cleanup(func() {
		console_setting.GetConsoleSetting().ApiInfoEnabled = oldApiInfoEnabled
		console_setting.GetConsoleSetting().ApiInfo = oldApiInfo
	})

	router := gin.New()
	router.GET("/api/status", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
			AgentID: 1,
			Domain:  "agent.example.com",
		})
		GetStatus(c)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Host = "agent.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]interface{})
	require.Equal(t, "https://agent.example.com", data["server_address"])

	apiInfo := data["api_info"].([]interface{})
	firstApiInfo := apiInfo[0].(map[string]interface{})
	require.Equal(t, "https://agent.example.com/v1/chat/completions", firstApiInfo["url"])
}
