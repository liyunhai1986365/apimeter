package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func channelConsistencyFixture(t *testing.T) *model.Channel {
	t.Helper()
	t.Setenv("CRYPTO_SECRET", "review-only-not-real")
	openConfigurableResourceTestDB(t)
	ch := &model.Channel{Name: "review", Type: constant.ChannelTypeConfigurable, Key: "video-a\nvideo-b", Models: "model", Group: "default", Status: common.ChannelStatusEnabled,
		BaseURL: common.GetPointer("https://video.invalid"), ChannelInfo: model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModeRandom}}
	ch.SetSetting(relaydto.ChannelSettings{Protocol: &relaydto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas", ProjectName: "project-before", AssetLibrary: &relaydto.AssetLibrarySettings{Backend: "tgxmaas", AuthMode: "api_key", BaseURL: "https://old-assets.invalid"}}})
	require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "old-asset-test-key"}))
	require.NoError(t, ch.Insert())
	return ch
}

func rotateAssetForConsistencyTest(t *testing.T, ch *model.Channel) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	c.Set("id", 1)
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(fmt.Sprintf(`{"id":%d,"asset_credentials":{"api_key":"rotated-test-key"}}`, ch.Id)))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(c)
	return w
}

func TestChannelPollingSnapshotPreservesDisabledKey(t *testing.T) {
	ch := channelConsistencyFixture(t)
	ch.ChannelInfo.MultiKeyMode = constant.MultiKeyModePolling
	require.NoError(t, model.DB.Model(ch).Update("channel_info", ch.ChannelInfo).Error)
	stale, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	// A request has selected its channel but has not selected a key yet.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	c.Set("id", 1)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"channel_id":%d,"action":"disable_key","key_index":0}`, ch.Id)))
	c.Request.Header.Set("Content-Type", "application/json")
	ManageMultiKeys(c)
	require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
	before, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	require.Equal(t, common.ChannelStatusManuallyDisabled, before.ChannelInfo.MultiKeyStatusList[0])
	key, _, apiErr := stale.GetNextEnabledKey()
	require.Nil(t, apiErr)
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	t.Logf("after successful disable and resumed request: selected_disabled_key=%t persisted_statuses=%v", key == "video-a", stored.ChannelInfo.MultiKeyStatusList)
	require.Equal(t, common.ChannelStatusManuallyDisabled, stored.ChannelInfo.MultiKeyStatusList[0], "key polling must not restore a disabled credential")
	require.Equal(t, "video-b", key)
}

func TestChannelAssetRotationRollsBackOnReadbackFailure(t *testing.T) {
	ch := channelConsistencyFixture(t)
	var before int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", ch.Id).Count(&before).Error)
	require.EqualValues(t, 1, before)
	updated, fired := false, false
	const afterWrite = "review:after_asset_update"
	const readFailure = "review:readback_failure"
	require.NoError(t, model.DB.Callback().Update().After("gorm:commit_or_rollback_transaction").Register(afterWrite, func(tx *gorm.DB) {
		if tx.Statement.Table == "channels" {
			updated = true
		}
	}))
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(readFailure, func(tx *gorm.DB) {
		if updated && !fired && tx.Statement.Table == "channels" {
			fired = true
			tx.AddError(errors.New("injected channel reload failure"))
		}
	}))
	defer model.DB.Callback().Update().Remove(afterWrite)
	defer model.DB.Callback().Query().Remove(readFailure)
	w := rotateAssetForConsistencyTest(t, ch)
	require.True(t, fired)
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	creds, err := stored.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, "old-asset-test-key", creds.APIKey)
	require.False(t, gjson.Get(w.Body.String(), "success").Bool())
	var after int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", ch.Id).Count(&after).Error)
	t.Logf("rotation result success=%t; channel_models=%s channel_status=%d; abilities_before=%d after=%d", gjson.Get(w.Body.String(), "success").Bool(), stored.Models, stored.Status, before, after)
	require.Equal(t, before, after, "asset rotation must not erase video routing abilities on readback failure")
}

func TestChannelAssetRotationDoesNotRebuildAbilities(t *testing.T) {
	ch := channelConsistencyFixture(t)
	const callback = "review:ability_write_failure"
	fired := false
	require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			fired = true
			tx.AddError(errors.New("injected ability write failure"))
		}
	}))
	defer model.DB.Callback().Delete().Remove(callback)
	w := rotateAssetForConsistencyTest(t, ch)
	require.False(t, fired)
	require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	creds, err := stored.GetAssetCredentials()
	require.NoError(t, err)
	t.Logf("API reported failure; new_asset_credential_committed=%t", creds.APIKey == "rotated-test-key")
	require.Equal(t, "rotated-test-key", creds.APIKey)
}

func TestChannelAssetRotationRejectsConcurrentBackendSwitch(t *testing.T) {
	ch := channelConsistencyFixture(t)
	replacement := *ch
	cfg := ch.GetSetting()
	cfg.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: "volcengine-assets", AuthMode: "aksk", BaseURL: "https://official.invalid"}
	replacement.SetSetting(cfg)
	require.NoError(t, replacement.SetAssetCredentials(&model.AssetCredentials{AccessKeyID: "test-ak", SecretAccessKey: "test-sk"}))
	fired := false
	reads := 0
	const callback = "review:backend_switch_before_rotate"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "channels" {
			return
		}
		reads++
		if reads != 2 {
			return
		}
		fired = true
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{"setting": replacement.Setting, "asset_secret": replacement.AssetSecret}).Error)
	}))
	defer model.DB.Callback().Query().Remove(callback)
	w := rotateAssetForConsistencyTest(t, ch)
	require.True(t, fired)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	require.False(t, gjson.Get(w.Body.String(), "success").Bool())
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	creds, err := stored.GetAssetCredentials()
	require.NoError(t, err)
	t.Logf("asset-only rotation success: backend=%s auth=%s has_api_key=%t has_ak=%t has_sk=%t", stored.GetSetting().Protocol.AssetLibrary.Backend, stored.GetSetting().Protocol.AssetLibrary.AuthMode, creds.APIKey != "", creds.AccessKeyID != "", creds.SecretAccessKey != "")
	require.Equal(t, "test-ak", creds.AccessKeyID)
	require.Equal(t, "test-sk", creds.SecretAccessKey)
	require.Empty(t, creds.APIKey)
}

func TestChannelCacheRejectsOlderRefresh(t *testing.T) {
	ch := channelConsistencyFixture(t)
	common.MemoryCacheEnabled = true
	defer func() { common.MemoryCacheEnabled = false }()
	model.InitChannelCache()
	fired := false
	const callback = "review:cache_read_snapshot"
	require.NoError(t, model.DB.Callback().Query().After("gorm:after_query").Register(callback, func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "channels" {
			return
		}
		if _, ok := tx.Statement.Dest.(*[]*model.Channel); !ok {
			return
		}
		fired = true
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", "new-video-test-key").Error)
		model.InitChannelCache()
	}))
	defer model.DB.Callback().Query().Remove(callback)
	model.InitChannelCache()
	require.True(t, fired)
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	cached, err := model.CacheGetChannel(ch.Id)
	require.NoError(t, err)
	t.Logf("after newer cache refresh completed then older refresh resumed: db_has_new_key=%t cache_has_old_key=%t", stored.Key == "new-video-test-key", cached.Key == ch.Key)
	require.Equal(t, stored.Key, cached.Key, "an older refresh must not roll back a completed credential update in cache")
}

func TestChannelRetryAppendPreservesConcurrentAssetBackendSwitch(t *testing.T) {
	ch := channelConsistencyFixture(t)
	replacement := *ch
	cfg := ch.GetSetting()
	cfg.Protocol.ProjectName = "project-after"
	cfg.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: "volcengine-assets", AuthMode: "aksk", BaseURL: "https://new-assets.invalid"}
	replacement.SetSetting(cfg)
	require.NoError(t, replacement.SetAssetCredentials(&model.AssetCredentials{AccessKeyID: "new-test-ak", SecretAccessKey: "new-test-sk"}))
	fired := false
	reads := 0
	const callback = "review:asset_backend_switch"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "channels" {
			return
		}
		reads++
		if reads != 2 {
			return
		}
		fired = true
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Updates(map[string]any{"setting": replacement.Setting, "asset_secret": replacement.AssetSecret}).Error)
	}))
	defer model.DB.Callback().Query().Remove(callback)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	c.Set("id", 1)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprint(ch.Id)}}
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"rule":{"name":"test","action":"skip_retry","status_codes":"400"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	AppendChannelRetryPolicyRule(c)
	require.True(t, fired)
	require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	creds, err := stored.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, "new-test-ak", creds.AccessKeyID)
	require.Len(t, stored.GetSetting().RetryPolicyRules, 1)
	t.Logf("after backend switch and retry-rule append: backend=%s auth=%s project=%s; new AKSK ciphertext survived", stored.GetSetting().Protocol.AssetLibrary.Backend, stored.GetSetting().Protocol.AssetLibrary.AuthMode, stored.GetSetting().Protocol.ProjectName)
	require.Equal(t, "volcengine-assets", stored.GetSetting().Protocol.AssetLibrary.Backend, "retry append must preserve concurrently saved asset backend")
}

func TestChannelAssetRotationPreservesConcurrentVideoKeyDisable(t *testing.T) {
	ch := channelConsistencyFixture(t)
	fired := false
	reads := 0
	const callback = "review:video_key_disable"
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "channels" {
			return
		}
		reads++
		if reads != 2 {
			return
		}
		fired = true
		ci := ch.ChannelInfo
		ci.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled}
		require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("channel_info", ci).Error)
	}))
	defer model.DB.Callback().Query().Remove(callback)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("role", common.RoleRootUser)
	c.Set("id", 1)
	c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(fmt.Sprintf(`{"id":%d,"asset_credentials":{"api_key":"new-asset-test-key"}}`, ch.Id)))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateChannel(c)
	require.True(t, fired)
	require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
	stored, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	creds, err := stored.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, "new-asset-test-key", creds.APIKey)
	selected, _, apiErr := stored.GetNextEnabledKeyMatching(func(key string, _ int) bool { return key == "video-a" })
	require.NotNil(t, apiErr)
	require.Empty(t, selected)
	t.Logf("after asset-only rotation and video-key disable: video statuses=%v; the disabled key can be selected again", stored.ChannelInfo.MultiKeyStatusList)
	require.Equal(t, common.ChannelStatusManuallyDisabled, stored.ChannelInfo.MultiKeyStatusList[0], "rotating assets must not reenable a disabled video key")
}

func TestChannelRoutingUpdateIsAtomic(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("ability_failure_%t", fail), func(t *testing.T) {
			ch := channelConsistencyFixture(t)
			if fail {
				const callback = "test:channel_ability_failure"
				require.NoError(t, model.DB.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table == "abilities" {
						tx.AddError(errors.New("injected ability write failure"))
					}
				}))
				defer model.DB.Callback().Delete().Remove(callback)
			}
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("role", common.RoleRootUser)
			c.Set("id", 1)
			c.Request = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(fmt.Sprintf(`{"id":%d,"models":"model,new-model","asset_credentials":{"api_key":"new-test-key"}}`, ch.Id)))
			c.Request.Header.Set("Content-Type", "application/json")
			UpdateChannel(c)
			require.Equal(t, !fail, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
			stored, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			credentials, err := stored.GetAssetCredentials()
			require.NoError(t, err)
			var abilities []model.Ability
			require.NoError(t, model.DB.Where("channel_id = ?", ch.Id).Find(&abilities).Error)
			if fail {
				require.Equal(t, "old-asset-test-key", credentials.APIKey)
				require.Equal(t, "model", stored.Models)
				require.Len(t, abilities, 1)
			} else {
				require.Equal(t, "new-test-key", credentials.APIKey)
				require.Equal(t, "model,new-model", stored.Models)
				require.Len(t, abilities, 2)
			}
		})
	}
}

func TestChannelPollingAcrossStorageModes(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache_%t", cached), func(t *testing.T) {
			ch := channelConsistencyFixture(t)
			ch.ChannelInfo.MultiKeyMode = constant.MultiKeyModePolling
			require.NoError(t, model.DB.Model(ch).Update("channel_info", ch.ChannelInfo).Error)
			common.MemoryCacheEnabled = cached
			defer func() { common.MemoryCacheEnabled = false }()
			model.InitChannelCache()
			stale, err := model.CacheGetChannel(ch.Id)
			require.NoError(t, err)
			for _, want := range []string{"video-a", "video-b", "video-a"} {
				key, _, apiErr := stale.GetNextEnabledKey()
				require.Nil(t, apiErr)
				require.Equal(t, want, key)
			}
			require.True(t, model.UpdateChannelStatus(ch.Id, "video-b", common.ChannelStatusAutoDisabled, "test rejection"))
			key, _, apiErr := stale.GetNextEnabledKey()
			require.Nil(t, apiErr)
			require.Equal(t, "video-a", key)
			latest, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			require.Equal(t, common.ChannelStatusAutoDisabled, latest.ChannelInfo.MultiKeyStatusList[1])
			// Changed key lists cannot be combined with a previously selected channel.
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("key", "replacement-key").Error)
			model.InitChannelCache()
			key, _, apiErr = stale.GetNextEnabledKey()
			require.NotNil(t, apiErr)
			require.Empty(t, key)
		})
	}
}

func TestChannelCacheReadFailureKeepsPreviousSnapshot(t *testing.T) {
	for _, table := range []string{"channels", "abilities"} {
		t.Run(table, func(t *testing.T) {
			ch := channelConsistencyFixture(t)
			common.MemoryCacheEnabled = true
			defer func() { common.MemoryCacheEnabled = false }()
			model.InitChannelCache()
			const callback = "test:channel_cache_query_failure"
			require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(errors.New("injected cache read failure"))
				}
			}))
			defer model.DB.Callback().Query().Remove(callback)
			model.InitChannelCache()
			cached, err := model.CacheGetChannel(ch.Id)
			require.NoError(t, err)
			require.Equal(t, ch.Key, cached.Key)
			selected, err := model.GetRandomSatisfiedChannel("default", "model", 0)
			require.NoError(t, err)
			require.NotNil(t, selected)
			require.Equal(t, ch.Id, selected.Id)
		})
	}
}
