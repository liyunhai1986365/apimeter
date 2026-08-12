package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupOpenMosaicSSOControllerTest(t *testing.T) string {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.OpenMosaicSSOTokenBinding{}, &model.OpenMosaicSSOAuthorizationCode{}))

	redirectURI := "https://openmosaic.example/auth/apimeter/callback"
	t.Setenv("OPENMOSAIC_SSO_CLIENT_SECRET", "controller-shared-secret")
	t.Setenv("OPENMOSAIC_SSO_REDIRECT_URIS", redirectURI)
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return redirectURI
}

func TestOpenMosaicSSOExchangeRequiresBearerSecretAndConsumesCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redirectURI := setupOpenMosaicSSOControllerTest(t)
	user := model.User{
		Username:        "controller-sso",
		DisplayName:     "Controller SSO",
		Email:           "verified@example.com",
		EmailVerifiedAt: time.Now().Unix(),
		Status:          common.UserStatusEnabled,
		Role:            common.RoleCommonUser,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	_, err := model.EnsureOpenMosaicSSOToken(model.DB, user.Id)
	require.NoError(t, err)
	code, err := model.CreateOpenMosaicSSOAuthorizationCode(model.DB, user.Id, redirectURI, time.Minute)
	require.NoError(t, err)

	call := func(secret string) *httptest.ResponseRecorder {
		body := []byte(`{"code":"` + code + `","redirect_uri":"` + redirectURI + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/integrations/openmosaic/exchange", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if secret != "" {
			req.Header.Set("Authorization", "Bearer "+secret)
		}
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = req
		ExchangeOpenMosaicSSO(ctx)
		return response
	}

	require.Equal(t, http.StatusUnauthorized, call("wrong-secret").Code)
	first := call("controller-shared-secret")
	require.Equal(t, http.StatusOK, first.Code)
	require.Contains(t, first.Body.String(), `"email_verified":true`)
	require.Contains(t, first.Body.String(), `"api_key":"`)
	require.Equal(t, http.StatusBadRequest, call("controller-shared-secret").Code)
}

func TestOpenMosaicSSORedirectAllowlistUsesExactMatch(t *testing.T) {
	t.Setenv("OPENMOSAIC_SSO_REDIRECT_URIS", "https://openmosaic.example/auth/apimeter/callback")
	require.True(t, isAllowedOpenMosaicRedirectURI("https://openmosaic.example/auth/apimeter/callback"))
	require.False(t, isAllowedOpenMosaicRedirectURI("https://openmosaic.example/auth/apimeter/callback/extra"))
	require.False(t, isAllowedOpenMosaicRedirectURI("https://openmosaic.example.evil.test/auth/apimeter/callback"))
}

func TestEmbeddedOpenMosaicSSOAuthorizesSameOriginSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redirectURI := setupOpenMosaicSSOControllerTest(t)
	user := model.User{Username: "embedded-sso", Email: "embedded@example.com", EmailVerifiedAt: time.Now().Unix(), Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, model.DB.Create(&user).Error)

	body := []byte(`{"redirect_uri":"` + redirectURI + `","site_origin":"https://proxy.example"}`)
	req := httptest.NewRequest(http.MethodPost, "https://proxy.example/api/integrations/openmosaic/embedded-authorize", bytes.NewReader(body))
	req.Host = "proxy.example"
	req.Header.Set("Origin", "https://proxy.example")
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = req
	ctx.Set("id", user.Id)
	AuthorizeEmbeddedOpenMosaicSSO(ctx)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"site_origin":"https://proxy.example"`)
	var code model.OpenMosaicSSOAuthorizationCode
	require.NoError(t, model.DB.First(&code).Error)
	require.Equal(t, "https://proxy.example", code.SiteOrigin)
}

func TestTopLevelOpenMosaicSSORejectsProxySiteBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	redirectURI := setupOpenMosaicSSOControllerTest(t)
	req := httptest.NewRequest(http.MethodGet, "/api/integrations/openmosaic/authorize?redirect_uri="+url.QueryEscape(redirectURI)+"&state=test&site_origin=https%3A%2F%2Fproxy.example", nil)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = req
	AuthorizeOpenMosaicSSO(ctx)
	require.Equal(t, http.StatusBadRequest, response.Code)
}
