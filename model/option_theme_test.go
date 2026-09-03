package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleConfigUpdateSyncsFrontendThemeToRuntime(t *testing.T) {
	originalRuntimeTheme := common.GetTheme()
	originalConfiguredTheme := system_setting.GetThemeSettings().Frontend
	t.Cleanup(func() {
		require.True(t, handleConfigUpdate("theme.frontend", originalConfiguredTheme))
		common.SetTheme(originalRuntimeTheme)
	})

	common.SetTheme("classic")

	require.True(t, handleConfigUpdate("theme.frontend", "default"))
	assert.Equal(t, "default", system_setting.GetThemeSettings().Frontend)
	assert.Equal(t, "default", common.GetTheme())
}
