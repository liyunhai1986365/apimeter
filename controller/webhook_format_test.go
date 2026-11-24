package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWebhookFormatUserSettingsPersistAndPreserveChoice(t *testing.T) {
	setupGlobalWebhookControllerTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	user := model.User{Id: 51, Username: "webhook-user", Password: "x"}
	user.SetSetting(dto.UserSetting{NotifyType: dto.NotifyTypeWebhook, Language: "zh"})
	require.NoError(t, model.DB.Create(&user).Error)

	for _, test := range []struct {
		name    string
		format  *string
		want    string
		success bool
	}{
		{"select_wecom", webhookFormatPointer(dto.WebhookFormatWeCom), dto.WebhookFormatWeCom, true},
		{"older_client_omits_format", nil, dto.WebhookFormatWeCom, true},
		{"switch_to_generic", webhookFormatPointer(dto.WebhookFormatGeneric), dto.WebhookFormatGeneric, true},
		{"reject_unknown_format", webhookFormatPointer("invalid"), dto.WebhookFormatGeneric, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			body, err := common.Marshal(UpdateUserSettingRequest{
				QuotaWarningType: dto.NotifyTypeWebhook,
				WebhookUrl:       "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test-key",
				WebhookFormat:    test.format,
			})
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", user.Id)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			UpdateUserSetting(c)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
			require.Equal(t, test.success, result.Success, w.Body.String())
			var updated model.User
			require.NoError(t, model.DB.First(&updated, user.Id).Error)
			require.Equal(t, test.want, updated.GetSetting().WebhookFormat)
			require.Equal(t, "zh", updated.GetSetting().Language)
		})
	}
}

func webhookFormatPointer(value string) *string { return &value }

func TestPersonalNotificationSettingsRequirePublicDestination(t *testing.T) {
	setupGlobalWebhookControllerTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	user := model.User{Username: "notification-target", Password: "x"}
	require.NoError(t, model.DB.Create(&user).Error)
	for _, tc := range []struct {
		name    string
		setting UpdateUserSettingRequest
		want    bool
	}{
		{"private webhook", UpdateUserSettingRequest{QuotaWarningType: dto.NotifyTypeWebhook, WebhookUrl: "http://127.0.0.1/hook"}, false},
		{"private bark", UpdateUserSettingRequest{QuotaWarningType: dto.NotifyTypeBark, BarkUrl: "http://localhost/{{title}}/{{content}}"}, false},
		{"private gotify", UpdateUserSettingRequest{QuotaWarningType: dto.NotifyTypeGotify, GotifyUrl: "http://10.0.0.1", GotifyToken: "test"}, false},
		{"public bark template", UpdateUserSettingRequest{QuotaWarningType: dto.NotifyTypeBark, BarkUrl: "https://example.org/notify/{{title}}/{{content}}"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := common.Marshal(tc.setting)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", user.Id)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			UpdateUserSetting(c)
			var result struct{ Success bool }
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
			require.Equal(t, tc.want, result.Success, w.Body.String())
		})
	}
}

func TestWebhookFormatGlobalOptionPersistsAndValidatesChoice(t *testing.T) {
	setupGlobalWebhookControllerTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	setting := operation_setting.GetWebhookSetting()
	original := *setting
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		*setting = original
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
	})

	for _, format := range []string{dto.WebhookFormatWeCom, dto.WebhookFormatGeneric, "invalid"} {
		t.Run(format, func(t *testing.T) {
			body, err := common.Marshal(OptionUpdateRequest{Key: "webhook_setting.format", Value: format})
			require.NoError(t, err)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
			UpdateOption(c)
			var result struct {
				Success bool `json:"success"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
			want := format
			if format == "invalid" {
				require.False(t, result.Success)
				want = dto.WebhookFormatGeneric
			} else {
				require.True(t, result.Success, w.Body.String())
			}
			require.Equal(t, want, setting.Format)
			var option model.Option
			require.NoError(t, model.DB.Where("key = ?", "webhook_setting.format").First(&option).Error)
			require.Equal(t, want, option.Value)
		})
	}
}
