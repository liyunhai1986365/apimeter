package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMonitorSettingAutoEnableDefaultsArePositive(t *testing.T) {
	setting := GetMonitorSetting()

	require.Greater(t, setting.ChannelAutoEnableCheckMinutes, 0)
	require.GreaterOrEqual(t, setting.ChannelAutoEnableCooldownMinutes, 0)
}
