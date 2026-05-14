package agent

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

type Branding struct {
	SiteName string `json:"site_name"`
	Logo     string `json:"logo"`
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
	return branding, branding.SiteName != "" || branding.Logo != ""
}

func ApplyBrandingToStatus(status gin.H, raw string) {
	branding, ok := ParseBranding(raw)
	if !ok {
		return
	}
	if branding.SiteName != "" {
		status["system_name"] = branding.SiteName
	}
	if branding.Logo != "" {
		status["logo"] = branding.Logo
	}
	status["agent_branding"] = gin.H{
		"site_name": branding.SiteName,
		"logo":      branding.Logo,
	}
}
