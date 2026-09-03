package controller

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const openMosaicSSOCodeTTL = 60 * time.Second

type openMosaicSSOExchangeRequest struct {
	Code        string `json:"code"`
	RedirectURI string `json:"redirect_uri"`
	SiteOrigin  string `json:"site_origin"`
}

type openMosaicEmbeddedAuthorizeRequest struct {
	RedirectURI string `json:"redirect_uri"`
	SiteOrigin  string `json:"site_origin"`
}

func AuthorizeEmbeddedOpenMosaicSSO(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request openMosaicEmbeddedAuthorizeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !isAllowedOpenMosaicRedirectURI(request.RedirectURI) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid OpenMosaic embedded SSO request"})
		return
	}
	siteOrigin, ok := normalizeRequestSiteOrigin(c, request.SiteOrigin)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "site_origin does not match this new-api site"})
		return
	}
	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, true)
	if err != nil || user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "account is unavailable"})
		return
	}
	if _, err := model.EnsureOpenMosaicSSOToken(model.DB, userID); err != nil {
		common.ApiError(c, err)
		return
	}
	code, err := model.CreateOpenMosaicSSOAuthorizationCodeForSite(model.DB, userID, request.RedirectURI, siteOrigin, openMosaicSSOCodeTTL)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"code": code, "site_origin": siteOrigin, "expires_in": int(openMosaicSSOCodeTTL.Seconds())}})
}

func AuthorizeOpenMosaicSSO(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	state := strings.TrimSpace(c.Query("state"))
	if c.Query("site_origin") != "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "embedded SSO must use the embedded-authorize endpoint"})
		return
	}
	if state == "" || !isAllowedOpenMosaicRedirectURI(redirectURI) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid OpenMosaic SSO request"})
		return
	}

	userID := c.GetInt("id")
	user, err := model.GetUserById(userID, true)
	if err != nil || user.Status != common.UserStatusEnabled {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "message": "Modelsell account is unavailable"})
		return
	}
	if _, err := model.EnsureOpenMosaicSSOToken(model.DB, userID); err != nil {
		common.ApiError(c, err)
		return
	}
	code, err := model.CreateOpenMosaicSSOAuthorizationCode(model.DB, userID, redirectURI, openMosaicSSOCodeTTL)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	callback, err := url.Parse(redirectURI)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid redirect_uri"})
		return
	}
	query := callback.Query()
	query.Set("code", code)
	query.Set("state", state)
	callback.RawQuery = query.Encode()
	c.Redirect(http.StatusFound, callback.String())
}

func ExchangeOpenMosaicSSO(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var request openMosaicSSOExchangeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}
	clientSecret := strings.TrimSpace(strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer "))
	if !validOpenMosaicClientSecret(clientSecret) || !isAllowedOpenMosaicRedirectURI(request.RedirectURI) {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": "invalid client credentials"})
		return
	}
	claim, err := model.ConsumeOpenMosaicSSOAuthorizationCodeForSite(model.DB, request.Code, request.RedirectURI, request.SiteOrigin, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "authorization code is invalid or expired"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"sub":            claim.UserID,
			"username":       claim.Username,
			"display_name":   claim.DisplayName,
			"email":          claim.Email,
			"email_verified": claim.EmailVerified,
			"api_key":        claim.APIKey,
		},
	})
}

func normalizeRequestSiteOrigin(c *gin.Context, candidate string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(candidate))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	requestOrigin := strings.TrimSpace(c.GetHeader("Origin"))
	normalized := parsed.Scheme + "://" + parsed.Host
	if requestOrigin == "" || requestOrigin != normalized {
		return "", false
	}
	if !strings.EqualFold(parsed.Host, c.Request.Host) {
		return "", false
	}
	return normalized, true
}

func isAllowedOpenMosaicRedirectURI(candidate string) bool {
	candidate = strings.TrimSpace(candidate)
	configured := strings.Split(os.Getenv("OPENMOSAIC_SSO_REDIRECT_URIS"), ",")
	for _, allowed := range configured {
		if candidate != "" && candidate == strings.TrimSpace(allowed) {
			return true
		}
	}
	return false
}

func validOpenMosaicClientSecret(candidate string) bool {
	expected := strings.TrimSpace(os.Getenv("OPENMOSAIC_SSO_CLIENT_SECRET"))
	candidate = strings.TrimSpace(candidate)
	if expected == "" || candidate == "" || len(expected) != len(candidate) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(candidate)) == 1
}
