package controller

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBuildModelAttemptsUsesGlobalWhenUserHasNoFallbackSetting(t *testing.T) {
	original := *model_setting.GetModelFallbackSetting()
	t.Cleanup(func() { *model_setting.GetModelFallbackSetting() = original })

	*model_setting.GetModelFallbackSetting() = model_setting.ModelFallbackSetting{
		Enabled:           true,
		AllowUserOverride: true,
		Rules: []model_setting.ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "qwen-3.6", Enabled: true},
		},
	}

	attempts := buildModelAttempts(&relaycommon.RelayInfo{OriginModelName: "gpt-5.5"})

	require.Equal(t, []string{"gpt-5.5", "qwen-3.6"}, attempts)
}

func TestBuildModelAttemptsUsesUserCustomFallback(t *testing.T) {
	original := *model_setting.GetModelFallbackSetting()
	t.Cleanup(func() { *model_setting.GetModelFallbackSetting() = original })

	*model_setting.GetModelFallbackSetting() = model_setting.ModelFallbackSetting{
		Enabled:           true,
		AllowUserOverride: true,
		Rules: []model_setting.ModelFallbackRule{
			{PrimaryModel: "gpt-5.5", FallbackModel: "global-fallback", Enabled: true},
		},
	}

	attempts := buildModelAttempts(&relaycommon.RelayInfo{
		OriginModelName: "gpt-5.5",
		UserSetting: relaycommonUserSetting(map[string]any{
			"mode":    "custom",
			"enabled": true,
			"rules": []any{
				map[string]any{"primary_model": "gpt-5.5", "fallback_model": "user-fallback", "enabled": true},
			},
		}),
	})

	require.Equal(t, []string{"gpt-5.5", "user-fallback"}, attempts)
}

func relaycommonUserSetting(modelFallback any) dto.UserSetting {
	return dto.UserSetting{ModelFallback: modelFallback}
}

func TestUpdateUserSettingPreservesModelFallbackWhenRequestOmitsIt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))

	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	model.DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
	})

	existing := dto.UserSetting{
		NotifyType:            dto.NotifyTypeEmail,
		QuotaWarningThreshold: 100,
		Language:              "zh",
		ModelFallback: map[string]any{
			"mode":    "custom",
			"enabled": true,
		},
	}
	settingBytes, err := json.Marshal(existing)
	require.NoError(t, err)
	user := model.User{
		Id:       1,
		Username: "test-user",
		Password: "x",
		Setting:  string(settingBytes),
	}
	require.NoError(t, db.Create(&user).Error)

	// This exercises the merge expectation around the controller path:
	// notification-only updates must start from existingSettings instead of an empty struct.
	loaded, err := model.GetUserById(1, true)
	require.NoError(t, err)
	merged := loaded.GetSetting()
	merged.NotifyType = dto.NotifyTypeWebhook
	merged.QuotaWarningThreshold = 50
	merged.WebhookUrl = "https://example.com/hook"
	merged.WebhookSecret = "secret"
	loaded.SetSetting(merged)
	require.NoError(t, loaded.Update(false))

	updated, err := model.GetUserById(1, true)
	require.NoError(t, err)
	updatedSetting := updated.GetSetting()
	require.Equal(t, dto.NotifyTypeWebhook, updatedSetting.NotifyType)
	require.Equal(t, "zh", updatedSetting.Language)
	require.NotNil(t, updatedSetting.ModelFallback)
}
