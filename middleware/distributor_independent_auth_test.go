package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestIndependentAuthContextPreservesMetadataAndClearsVideoKey(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	channel := &model.Channel{
		Id: 20, Name: "asset-channel", Type: constant.ChannelTypeConfigurable,
		Key: "disabled-video-key", BaseURL: common.GetPointer("https://video.example"),
		ChannelRatio: common.GetPointer(1.5),
		ChannelInfo: model.ChannelInfo{
			IsMultiKey: true, MultiKeySize: 1, MultiKeyMode: constant.MultiKeyModePolling,
			MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled},
		},
	}
	common.SetContextKey(c, constant.ContextKeyChannelKey, "previous-video-key")
	common.SetContextKey(c, constant.ContextKeyChannelIsMultiKey, true)
	common.SetContextKey(c, constant.ContextKeyChannelMultiKeyIndex, 9)
	require.Nil(t, SetupContextForSelectedChannelWithIndependentAuth(c, channel, "model"))
	require.Equal(t, channel.Id, c.GetInt(string(constant.ContextKeyChannelId)))
	require.Equal(t, channel.Name, common.GetContextKeyString(c, constant.ContextKeyChannelName))
	require.Equal(t, channel.GetBaseURL(), common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl))
	require.Equal(t, 1.5, c.GetFloat64(string(constant.ContextKeyChannelRatio)))
	selected, ok := c.Get("relay_selected_channel")
	require.True(t, ok)
	require.Same(t, channel, selected)
	require.Empty(t, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
	require.False(t, common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey))
	require.Zero(t, c.GetInt(string(constant.ContextKeyChannelMultiKeyIndex)))

	// Ordinary single-key requests retain their credential selection.
	channel.ChannelInfo = model.ChannelInfo{}
	channel.Key = "enabled-video-key"
	require.Nil(t, SetupContextForSelectedChannel(c, channel, "model"))
	require.Equal(t, channel.Key, common.GetContextKeyString(c, constant.ContextKeyChannelKey))
}
