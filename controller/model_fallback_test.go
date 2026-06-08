package controller

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
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

func TestUserImageStorageSettingPersistsAndRedactsSecret(t *testing.T) {
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

	user := model.User{
		Id:       1,
		Username: "image-storage-user",
		Password: "x",
	}
	require.NoError(t, db.Create(&user).Error)

	loaded, err := model.GetUserById(1, true)
	require.NoError(t, err)
	settings := loaded.GetSetting()
	settings.ImageStorage = dto.UserImageStorageSetting{
		Enabled:         true,
		Type:            dto.UserImageStorageTypeAliyunOSS,
		Endpoint:        "https://oss-cn.example.com",
		Bucket:          "bucket",
		AccessKeyID:     "access-key",
		AccessKeySecret: "access-secret",
		ObjectPrefix:    "prefix",
		PublicBaseURL:   "https://cdn.example.com",
	}
	loaded.SetSetting(settings)
	require.NoError(t, loaded.Update(false))

	updated, err := model.GetUserById(1, true)
	require.NoError(t, err)
	updatedSetting := updated.GetSetting()
	require.Equal(t, "access-secret", updatedSetting.ImageStorage.AccessKeySecret)

	redacted := updatedSetting.ImageStorage.Redacted()
	require.Equal(t, "access-key", redacted.AccessKeyID)
	require.Empty(t, redacted.AccessKeySecret)
}

func TestTestUserImageStorageUsesSavedSecretWhenRequestOmitsIt(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var uploadedBody string
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()

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

	settings := dto.UserSetting{
		ImageStorage: dto.UserImageStorageSetting{
			Enabled:         true,
			Type:            dto.UserImageStorageTypeAliyunOSS,
			Endpoint:        ossServer.URL,
			Bucket:          "bucket",
			AccessKeyID:     "access-key",
			AccessKeySecret: "saved-secret",
			ObjectPrefix:    "prefix",
		},
	}
	settingBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	user := model.User{
		Id:       1,
		Username: "image-storage-test-user",
		Password: "x",
		Setting:  string(settingBytes),
	}
	require.NoError(t, db.Create(&user).Error)

	body := []byte(`{
		"enabled": true,
		"type": "aliyun_oss",
		"endpoint": "` + ossServer.URL + `",
		"bucket": "bucket",
		"access_key_id": "access-key",
		"object_prefix": "prefix"
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/setting/image-storage/test", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)

	TestUserImageStorage(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Equal(t, "test", uploadedBody)
}

func TestTestUserImageStorageSupportsCloudflareR2(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var uploadedBody string
	var uploadedAuthorization string
	r2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uploadedAuthorization = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer r2Server.Close()

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

	user := model.User{
		Id:       1,
		Username: "r2-storage-test-user",
		Password: "x",
	}
	require.NoError(t, db.Create(&user).Error)

	body := []byte(`{
		"enabled": true,
		"type": "cloudflare_r2",
		"endpoint": "` + r2Server.URL + `",
		"bucket": "bucket",
		"access_key_id": "access-key",
		"access_key_secret": "secret",
		"object_prefix": "prefix"
	}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/user/setting/image-storage/test", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)

	TestUserImageStorage(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)
	require.Equal(t, "test", uploadedBody)
	require.Contains(t, uploadedAuthorization, "AWS4-HMAC-SHA256")
}
