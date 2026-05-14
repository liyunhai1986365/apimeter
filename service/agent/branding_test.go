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

	ApplyBrandingToStatus(status, `{"site_name":"Agent Site","logo":"https://agent.example.com/logo.png"}`)

	assert.Equal(t, "Agent Site", status["system_name"])
	assert.Equal(t, "https://agent.example.com/logo.png", status["logo"])
	assert.Equal(t, gin.H{
		"site_name": "Agent Site",
		"logo":      "https://agent.example.com/logo.png",
	}, status["agent_branding"])
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
