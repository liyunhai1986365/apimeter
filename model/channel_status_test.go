package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelStatusTest(t *testing.T) {
	t.Helper()
	truncateTables(t)
	require.NoError(t, DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, DB.Exec("DELETE FROM channels").Error)

	memoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = memoryCacheEnabled
	})
}

func TestUpdateChannelStatusPersistsMultiKeyState(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:   "multi-key-status",
		Key:    "key-a\nkey-b",
		Status: common.ChannelStatusEnabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey:           true,
			MultiKeySize:         2,
			MultiKeyMode:         constant.MultiKeyModePolling,
			MultiKeyPollingIndex: 1,
		},
	}
	require.NoError(t, DB.Create(&channel).Error)

	changed := UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, "provider rejected key")
	require.True(t, changed)

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Equal(t, common.ChannelStatusAutoDisabled, stored.ChannelInfo.MultiKeyStatusList[0])
	assert.Equal(t, "provider rejected key", stored.ChannelInfo.MultiKeyDisabledReason[0])
	assert.NotZero(t, stored.ChannelInfo.MultiKeyDisabledTime[0])
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
}

func TestSaveStatusStateFromSingleKeySnapshotPreservesUnownedColumns(t *testing.T) {
	setupChannelStatusTest(t)

	channel := Channel{
		Name:        "single-key-status",
		Key:         "original-key",
		Status:      common.ChannelStatusEnabled,
		Models:      "original-model",
		Group:       "default",
		UsedQuota:   100,
		ChannelInfo: ChannelInfo{},
	}
	require.NoError(t, DB.Create(&channel).Error)

	stale, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)

	concurrentChannelInfo := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyMode:         constant.MultiKeyModePolling,
		MultiKeyPollingIndex: 1,
	}
	require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
		"key":          "rotated-key",
		"used_quota":   gorm.Expr("used_quota + ?", 250),
		"models":       "concurrent-model",
		"channel_info": concurrentChannelInfo,
	}).Error)

	stale.Status = common.ChannelStatusManuallyDisabled
	stale.SetOtherInfo(map[string]interface{}{
		"status_reason": "manual operation",
		"status_time":   int64(1234),
	})
	require.NoError(t, stale.saveStatusState())

	var stored Channel
	require.NoError(t, DB.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
	assert.Equal(t, "rotated-key", stored.Key)
	assert.Equal(t, int64(350), stored.UsedQuota)
	assert.Equal(t, "concurrent-model", stored.Models)
	assert.Equal(t, concurrentChannelInfo, stored.ChannelInfo)

	otherInfo := stored.GetOtherInfo()
	assert.Equal(t, "manual operation", otherInfo["status_reason"])
	assert.Equal(t, float64(1234), otherInfo["status_time"])
}

func TestUpdateWholeChannelStatusPreservesConcurrentAssetCredentialRotation(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "channel-status-test-only")
	for _, credentials := range []struct {
		name     string
		original AssetCredentials
		rotated  AssetCredentials
	}{
		{"api_key", AssetCredentials{APIKey: "original-asset-key"}, AssetCredentials{APIKey: "rotated-asset-key"}},
		{"aksk", AssetCredentials{AccessKeyID: "original-ak", SecretAccessKey: "original-sk"}, AssetCredentials{AccessKeyID: "rotated-ak", SecretAccessKey: "rotated-sk"}},
	} {
		for _, mode := range []string{"single", "multi"} {
			t.Run(credentials.name+"/"+mode, func(t *testing.T) {
				setupChannelStatusTest(t)
				channel := Channel{
					Name: "asset-credential-rotation", Key: "video-key", Models: "model", Group: "default",
					Status: common.ChannelStatusEnabled, UsedQuota: 100, Setting: common.GetPointer("{}"),
					ChannelInfo: ChannelInfo{IsMultiKey: mode == "multi", MultiKeySize: 1},
				}
				require.NoError(t, channel.SetAssetCredentials(&credentials.original))
				require.NoError(t, channel.Insert())
				rotated := channel
				require.NoError(t, rotated.SetAssetCredentials(&credentials.rotated))

				// Commit another writer's changes after the status flow reads its
				// snapshot, but before it starts its write transaction.
				rotatedDuringSave := false
				const callback = "test:rotate_asset_credentials_before_status_save"
				require.NoError(t, DB.Callback().Update().Before("gorm:begin_transaction").Register(callback, func(tx *gorm.DB) {
					if rotatedDuringSave || tx.Statement.Table != "channels" {
						return
					}
					rotatedDuringSave = true
					require.NoError(t, DB.Model(&Channel{}).Where("id = ?", channel.Id).Updates(map[string]any{
						"asset_secret": rotated.AssetSecret,
						"key":          "rotated-video-key",
						"used_quota":   gorm.Expr("used_quota + ?", 250),
						"setting":      `{"proxy":"http://proxy.example"}`,
					}).Error)
				}))
				t.Cleanup(func() { _ = DB.Callback().Update().Remove(callback) })

				require.True(t, UpdateWholeChannelStatus(channel.Id, common.ChannelStatusAutoDisabled, "monitor failure"))
				require.True(t, rotatedDuringSave)
				stored, err := GetChannelById(channel.Id, true)
				require.NoError(t, err)
				actualCredentials, err := stored.GetAssetCredentials()
				require.NoError(t, err)
				assert.Equal(t, credentials.rotated, *actualCredentials)
				assert.Equal(t, "rotated-video-key", stored.Key)
				assert.Equal(t, int64(350), stored.UsedQuota)
				assert.Equal(t, `{"proxy":"http://proxy.example"}`, *stored.Setting)
				assert.Equal(t, common.ChannelStatusAutoDisabled, stored.Status)
				assert.Equal(t, "monitor failure", stored.GetOtherInfo()["status_reason"])
				assert.False(t, IsChannelEnabledForGroupModel("default", "model", channel.Id))
			})
		}
	}
}

func TestUpdateWholeChannelStatusEnablesMultiKeyChannel(t *testing.T) {
	setupChannelStatusTest(t)
	channel := Channel{
		Name: "enable-whole-channel", Key: "key-a\nkey-b", Models: "model", Group: "default",
		Status: common.ChannelStatusAutoDisabled,
		ChannelInfo: ChannelInfo{
			IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling, MultiKeyPollingIndex: 1,
			MultiKeyStatusList:     map[int]int{0: common.ChannelStatusAutoDisabled, 1: common.ChannelStatusAutoDisabled},
			MultiKeyDisabledReason: map[int]string{0: "key failure", 1: "key failure"},
			MultiKeyDisabledTime:   map[int]int64{0: 123, 1: 123},
		},
	}
	require.NoError(t, channel.Insert())
	require.True(t, UpdateWholeChannelStatus(channel.Id, common.ChannelStatusEnabled, "recovered"))
	stored, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	assert.Empty(t, stored.ChannelInfo.MultiKeyStatusList)
	assert.Empty(t, stored.ChannelInfo.MultiKeyDisabledReason)
	assert.Empty(t, stored.ChannelInfo.MultiKeyDisabledTime)
	assert.Equal(t, 1, stored.ChannelInfo.MultiKeyPollingIndex)
	assert.True(t, IsChannelEnabledForGroupModel("default", "model", channel.Id))
}
