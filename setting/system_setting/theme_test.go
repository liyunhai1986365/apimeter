package system_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestFrontendThemeDefaultsToNewFrontend(t *testing.T) {
	assert.Equal(t, "default", GetThemeSettings().Frontend)
	assert.Equal(t, "default", common.GetTheme())
}
