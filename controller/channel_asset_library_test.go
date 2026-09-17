package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAddMultiKeyChannelAssetLibraryValidation(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "multi-key-assets-test-only")
	for _, tc := range []struct {
		name, backend, auth string
		credentials         *model.AssetCredentials
		wantError           string
	}{
		{name: "omitted"},
		{name: "disabled", backend: "disabled", auth: "channel_key"},
		{name: "inherit", backend: "inherit", auth: "channel_key"},
		{name: "shared_key", backend: "tgxmaas", auth: "channel_key", wantError: "dedicated asset credentials"},
		{name: "dedicated_api_key", backend: "tgxmaas", auth: "api_key", credentials: &model.AssetCredentials{APIKey: "asset-key"}},
		{name: "dedicated_aksk", backend: "volcengine-assets", auth: "aksk", credentials: &model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "asset-sk"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			openConfigurableResourceTestDB(t)
			protocol := &dto.ChannelProtocolSettings{ProfileID: "kling-video", ProjectName: "mock-project"}
			if tc.backend != "" {
				protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: tc.backend, AuthMode: tc.auth}
			}
			ch := &model.Channel{
				Name: tc.name, Type: constant.ChannelTypeConfigurable, Key: "video-a\nvideo-b",
				Models: "kling-v1", Group: "default", BaseURL: common.GetPointer("https://video.example"),
			}
			ch.SetSetting(dto.ChannelSettings{Protocol: protocol})
			payload, err := common.Marshal(AddChannelRequest{
				Mode: "multi_to_single", MultiKeyMode: constant.MultiKeyModePolling,
				Channel: ch, AssetCredentials: tc.credentials,
			})
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/channel", bytes.NewReader(payload))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set("id", 1)
			c.Set("role", common.RoleRootUser)
			AddChannel(c)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			if tc.wantError != "" {
				require.False(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
				require.Contains(t, w.Body.String(), tc.wantError)
				var count int64
				require.NoError(t, model.DB.Model(&model.Channel{}).Count(&count).Error)
				require.Zero(t, count)
				return
			}
			require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
			var stored model.Channel
			require.NoError(t, model.DB.Where("name = ?", tc.name).First(&stored).Error)
			require.True(t, stored.ChannelInfo.IsMultiKey)
			require.Equal(t, 2, stored.ChannelInfo.MultiKeySize)
			require.Equal(t, constant.MultiKeyModePolling, stored.ChannelInfo.MultiKeyMode)
			require.Equal(t, ch.Key, stored.Key)
			if tc.credentials != nil {
				credentials, err := stored.GetAssetCredentials()
				require.NoError(t, err)
				require.Equal(t, tc.credentials, credentials)
			}
		})
	}
}
