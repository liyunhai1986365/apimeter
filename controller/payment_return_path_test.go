package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPaymentReturnPathForRequestUsesAgentDomain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.SetTheme("default")
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() {
		common.SetTheme("classic")
		system_setting.ServerAddress = oldServerAddress
	})

	router := gin.New()
	router.GET("/return", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
			AgentID: 1,
			Domain:  "agent.example.com",
		})

		c.String(http.StatusOK, paymentReturnPathForRequest(c, "/console/log"))
	})

	req := httptest.NewRequest(http.MethodGet, "/return", nil)
	req.Host = "agent.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "https://agent.example.com/usage-logs", w.Body.String())
}

func TestPaymentReturnPathForRequestFallsBackToServerAddress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.SetTheme("default")
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() {
		common.SetTheme("classic")
		system_setting.ServerAddress = oldServerAddress
	})

	router := gin.New()
	router.GET("/return", func(c *gin.Context) {
		c.String(http.StatusOK, paymentReturnPathForRequest(c, "/console/topup?pay=success"))
	})

	req := httptest.NewRequest(http.MethodGet, "/return", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "https://modelsell.com/wallet?pay=success", w.Body.String())
}
