package agent

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestApplyBrandingToStatusOverridesSystemNameAndLogo(t *testing.T) {
	status := gin.H{
		"system_name": "Main Site",
		"logo":        "/logo.png",
	}

	ApplyBrandingToStatus(status, `{"site_name":"Agent Site","logo":"https://agent.example.com/logo.png","header_nav_modules":"{\"home\":false}"}`)

	assert.Equal(t, "Agent Site", status["system_name"])
	assert.Equal(t, "https://agent.example.com/logo.png", status["logo"])
	assert.Equal(t, `{"home":false}`, status["HeaderNavModules"])
	assert.Equal(t, gin.H{
		"site_name":          "Agent Site",
		"logo":               "https://agent.example.com/logo.png",
		"header_nav_modules": `{"home":false}`,
	}, status["agent_branding"])
}

func TestParseBrandingReturnsHomePageContent(t *testing.T) {
	branding, ok := ParseBranding(`{"site_name":"Agent Site","logo":"https://agent.example.com/logo.png","home_page_content":"# Agent Home","header_nav_modules":"{\"pricing\":{\"enabled\":false}}"}`)

	assert.True(t, ok)
	assert.Equal(t, "Agent Site", branding.SiteName)
	assert.Equal(t, "https://agent.example.com/logo.png", branding.Logo)
	assert.Equal(t, "# Agent Home", branding.HomePageContent)
	assert.Equal(t, `{"pricing":{"enabled":false}}`, branding.HeaderNavModules)
}

func TestApplyBrandingToStatusKeepsDefaultsForInvalidOrBlankBranding(t *testing.T) {
	status := gin.H{
		"system_name": "Main Site",
		"logo":        "/logo.png",
	}

	ApplyBrandingToStatus(status, `{"site_name":"","logo":""}`)
	ApplyBrandingToStatus(status, `not-json`)

	assert.Equal(t, "Main Site", status["system_name"])
	assert.Equal(t, "/logo.png", status["logo"])
	_, exists := status["agent_branding"]
	assert.False(t, exists)
}
