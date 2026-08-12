package system_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestDefaultThemeUsesAPIMeterFrontend(t *testing.T) {
	require.Equal(t, "default", GetThemeSettings().Frontend)
	require.Equal(t, "default", common.GetTheme())
}
