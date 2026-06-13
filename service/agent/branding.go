package agent

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type Branding struct {
	SiteName         string `json:"site_name"`
	Logo             string `json:"logo"`
	HomePageContent  string `json:"home_page_content"`
	HeaderNavModules string `json:"header_nav_modules"`
}

func ParseBranding(raw string) (Branding, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Branding{}, false
	}
	var branding Branding
	if err := common.UnmarshalJsonStr(raw, &branding); err != nil {
		return Branding{}, false
	}
	branding.SiteName = strings.TrimSpace(branding.SiteName)
	branding.Logo = strings.TrimSpace(branding.Logo)
	branding.HomePageContent = strings.TrimSpace(branding.HomePageContent)
	branding.HeaderNavModules = strings.TrimSpace(branding.HeaderNavModules)
	return branding, branding.SiteName != "" || branding.Logo != "" || branding.HomePageContent != "" || branding.HeaderNavModules != ""
}

func ApplyBrandingToStatus(status gin.H, raw string) {
	branding, ok := ParseBranding(raw)
	if !ok {
		return
	}
	if branding.SiteName == "" && branding.Logo == "" && branding.HeaderNavModules == "" {
		return
	}
	if branding.SiteName != "" {
		status["system_name"] = branding.SiteName
	}
	if branding.Logo != "" {
		status["logo"] = branding.Logo
	}
	if branding.HeaderNavModules != "" {
		status["HeaderNavModules"] = branding.HeaderNavModules
	}
	status["agent_branding"] = gin.H{
		"site_name":          branding.SiteName,
		"logo":               branding.Logo,
		"header_nav_modules": branding.HeaderNavModules,
	}
}

func HomePageContentFromBranding(raw string) (string, bool) {
	branding, ok := ParseBranding(raw)
	if !ok || branding.HomePageContent == "" {
		return "", false
	}
	return branding.HomePageContent, true
}
