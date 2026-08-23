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

func TestGetStatusIncludesGoogleAnalyticsMeasurementID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldMeasurementID := common.OptionMap["GoogleAnalyticsId"]
	common.OptionMap["GoogleAnalyticsId"] = "G-6B94BX72EW"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap["GoogleAnalyticsId"] = oldMeasurementID
		common.OptionMapRWMutex.Unlock()
	})

	router := gin.New()
	router.GET("/api/status", GetStatus)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]interface{})
	require.Equal(t, "G-6B94BX72EW", data["google_analytics_id"])
}

func TestGetStatusIncludesFooterCompanyName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldValue := common.OptionMap[common.FooterCompanyNameOptionKey]
	common.OptionMap[common.FooterCompanyNameOptionKey] = "Example Technology Ltd."
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap[common.FooterCompanyNameOptionKey] = oldValue
		common.OptionMapRWMutex.Unlock()
	})

	router := gin.New()
	router.GET("/api/status", GetStatus)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]interface{})
	require.Equal(t, "Example Technology Ltd.", data["footer_company_name"])
}

func TestGetStatusIncludesMainlandChinaPresentationFlag(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldValue := common.OptionMap[common.MainlandChinaPresentationOptionKey]
	common.OptionMap[common.MainlandChinaPresentationOptionKey] = "true"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap[common.MainlandChinaPresentationOptionKey] = oldValue
		common.OptionMapRWMutex.Unlock()
	})

	router := gin.New()
	router.GET("/api/status", GetStatus)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]interface{})
	require.Equal(t, true, data["mainland_china_presentation_enabled"])
}

func TestGetHomePageContentUsesAgentBrandingContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	oldHomePageContent := common.OptionMap["HomePageContent"]
	common.OptionMap["HomePageContent"] = "# Main Home"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap["HomePageContent"] = oldHomePageContent
		common.OptionMapRWMutex.Unlock()
	})

	router := gin.New()
	router.GET("/api/home_page_content", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
			AgentID:  1,
			Domain:   "agent.example.com",
			Branding: `{"home_page_content":"# Agent Home"}`,
		})
		GetHomePageContent(c)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/home_page_content", nil)
	req.Host = "agent.example.com"
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, "# Agent Home", body["data"])
}
