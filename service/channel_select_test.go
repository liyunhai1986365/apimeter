package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openChannelSelectTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalMemoryCacheEnabled := common.MemoryCacheEnabled
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.MemoryCacheEnabled = true
	model.InitColForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))
	model.DB = db

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.MemoryCacheEnabled = originalMemoryCacheEnabled
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func createChannelSelectFixture(
	t *testing.T,
	db *gorm.DB,
	id int,
	group string,
	priority int64,
) {
	t.Helper()

	weight := uint(100)
	autoBan := 1
	channel := model.Channel{
		Id:       id,
		Type:     1,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     group,
		Group:    group,
		Models:   "gpt-test",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     "gpt-test",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func createImageChannelSelectFixture(
	t *testing.T,
	db *gorm.DB,
	id int,
	group string,
	priority int64,
	setting dto.ChannelSettings,
) {
	t.Helper()

	weight := uint(100)
	autoBan := 1
	channel := model.Channel{
		Id:       id,
		Type:     1,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     group,
		Group:    group,
		Models:   "gpt-image-2",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	channel.SetSetting(setting)
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     group,
		Model:     "gpt-image-2",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func TestCacheGetRandomSatisfiedChannelAutoGroupAndCrossGroupRetry(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	createChannelSelectFixture(t, db, 1001, "default", 10)
	createChannelSelectFixture(t, db, 1002, "vip", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)

	retry := common.RetryTimes
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1001, channel.Id)
	require.Equal(t, "default", selectedGroup)

	retry++
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1002, channel.Id)
	require.Equal(t, "vip", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelFiltersImageChatProtocolSupport(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})

	createImageChannelSelectFixture(t, db, 1001, "default", 20, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	createImageChannelSelectFixture(t, db, 1002, "default", 10, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{
				string(conversion.RequestModeOpenAIChat),
				string(conversion.RequestModeOpenAIImageGenerations),
			},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-image-2"}`))

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "gpt-image-2",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1002, channel.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestBuildProtocolChannelFilterUsesConversionRequirement(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-image-2"}`))

	filter := BuildProtocolChannelFilter(&RetryParam{
		Ctx:       c,
		ModelName: "gpt-image-2",
	})
	require.NotNil(t, filter)

	unsupported := model.Channel{}
	unsupported.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	require.False(t, filter(&unsupported))

	supported := model.Channel{}
	supported.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{
				string(conversion.RequestModeOpenAIChat),
				string(conversion.RequestModeOpenAIImageGenerations),
			},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	require.True(t, filter(&supported))
}

func TestCacheGetRandomSatisfiedChannelRejectsWhenAllImageChatChannelsUnsupported(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})

	createImageChannelSelectFixture(t, db, 1001, "default", 20, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	createImageChannelSelectFixture(t, db, 1002, "default", 10, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIChat)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-image-2"}`))

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "gpt-image-2",
		Retry:      common.GetPointer(0),
	})
	require.Error(t, err)
	require.Nil(t, channel)
	require.Equal(t, "default", selectedGroup)
	require.Contains(t, err.Error(), string(conversion.RequestModeOpenAIChat))
	require.Contains(t, err.Error(), string(conversion.ConversionOpenAIChatToImageGenerations))
}

func TestCacheGetRandomSatisfiedChannelFiltersImageChatProtocolSupportWithoutMemoryCache(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})
	common.MemoryCacheEnabled = false

	createImageChannelSelectFixture(t, db, 1001, "default", 20, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})
	createImageChannelSelectFixture(t, db, 1002, "default", 10, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{
				string(conversion.RequestModeOpenAIChat),
				string(conversion.RequestModeOpenAIImageGenerations),
			},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})

	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-image-2"}`))

	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "gpt-image-2",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1002, channel.Id)
	require.Equal(t, "default", selectedGroup)
}
