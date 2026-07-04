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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
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

func TestCacheGetRandomSatisfiedChannelAutoGroupDefaultsToVisibleGroups(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		_ = setting.UpdateGroupDisplayConfigByJSONString(`{"categories":[],"groups":[]}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"default": "默认分组",
		"backup": "备用分组",
		"vip": "vip分组"
	}`))
	require.NoError(t, setting.UpdateGroupDisplayConfigByJSONString(`{
		"categories": [],
		"groups": [
			{"group": "backup", "order": 10},
			{"group": "default", "order": 20},
			{"group": "vip", "order": 30, "user_group": true}
		]
	}`))
	createChannelSelectFixture(t, db, 1501, "backup", 10)
	createChannelSelectFixture(t, db, 1502, "default", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1501, channel.Id)
	require.Equal(t, "backup", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelAgentAutoGroupKeepsAgentGroupContext(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	createChannelSelectFixture(t, db, 1601, "default", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "member")
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		AgentID: 3,
		Groups: map[string]types.AgentGroup{
			"default": {
				GroupName:       "default",
				SystemGroupName: "default",
				Visible:         true,
				Available:       true,
			},
		},
		UserGroups: map[string]types.AgentUserGroup{
			"member": {
				GroupName:     "member",
				VisibleGroups: []string{"default"},
			},
		},
	})

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1601, channel.Id)
	require.Equal(t, "default", selectedGroup)
	require.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
	require.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyAgentSelectedGroup))
}

func TestCacheGetRandomSatisfiedChannelAgentPolicyAutoKeepsAgentGroupContext(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`[]`))
	createChannelSelectFixture(t, db, 1602, "default", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "member")
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"ordered","groups":["auto"]}`)
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		AgentID: 3,
		Groups: map[string]types.AgentGroup{
			"default": {
				GroupName:       "default",
				SystemGroupName: "default",
				Visible:         true,
				Available:       true,
			},
		},
		UserGroups: map[string]types.AgentUserGroup{
			"member": {
				GroupName:     "member",
				VisibleGroups: []string{"default"},
			},
		},
	})

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1602, channel.Id)
	require.Equal(t, "default", selectedGroup)
	require.Equal(t, "default", common.GetContextKeyString(c, constant.ContextKeyAgentSelectedGroup))
}

func TestCacheGetRandomSatisfiedChannelDoesNotResetRetryAtLastAutoGroup(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	createChannelSelectFixture(t, db, 1201, "default", 10)
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
	require.Equal(t, 1201, channel.Id)
	require.Equal(t, "default", selectedGroup)
	require.Equal(t, common.RetryTimes, retry)
	index, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex)
	require.True(t, ok)
	require.Equal(t, 0, index)
}

func TestCacheGetRandomSatisfiedChannelUsesOrderedTokenGroupPolicy(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
	createChannelSelectFixture(t, db, 1101, "vip", 10)
	createChannelSelectFixture(t, db, 1102, "backup", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"ordered","groups":["vip","backup"]}`)

	retry := common.RetryTimes
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1101, channel.Id)
	require.Equal(t, "vip", selectedGroup)

	retry++
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1102, channel.Id)
	require.Equal(t, "backup", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelUsesAgentSystemGroupPolicy(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	createChannelSelectFixture(t, db, 1111, "vip", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "member")
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"ordered","groups":["vip"]}`)
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		AgentID: 10,
		Groups: map[string]types.AgentGroup{
			"vip": {
				GroupName:       "vip",
				SystemGroupName: "vip",
				Visible:         false,
				Available:       true,
			},
		},
		UserGroups: map[string]types.AgentUserGroup{
			"member": {
				GroupName:     "member",
				VisibleGroups: []string{"vip"},
			},
		},
	})

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "vip",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1111, channel.Id)
	require.Equal(t, "vip", selectedGroup)
	require.Equal(t, "vip", common.GetContextKeyString(c, constant.ContextKeyAgentSelectedGroup))
}

func TestCacheGetRandomSatisfiedChannelUsesRoutingStrategySnapshot(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip","backup"]`))
	createChannelSelectFixture(t, db, 1601, "default", 10)
	createChannelSelectFixture(t, db, 1602, "vip", 10)
	createChannelSelectFixture(t, db, 1603, "backup", 10)
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:  model.RoutingStrategySpeedFirst,
		UserGroup: "default",
		Groups:    `["backup","vip","default"]`,
		Scores:    `{"backup":{"group":"backup","rank":1},"vip":{"group":"vip","rank":2},"default":{"group":"default","rank":3}}`,
		Config:    `{}`,
	}))
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"speed_first"}`)

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1603, channel.Id)
	require.Equal(t, "backup", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelRoutingStrategyAdvancesGroupAfterFailure(t *testing.T) {
	db := openChannelSelectTestDB(t)
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 2
	t.Cleanup(func() {
		common.RetryTimes = originalRetryTimes
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip","backup"]`))
	createChannelSelectFixture(t, db, 1801, "vip", 10)
	createChannelSelectFixture(t, db, 1802, "backup", 10)
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:  model.RoutingStrategySmartAuto,
		UserGroup: "default",
		Groups:    `["vip","backup","default"]`,
		Scores:    `{"vip":{"group":"vip","rank":1},"backup":{"group":"backup","rank":2},"default":{"group":"default","rank":3}}`,
		Config:    `{}`,
	}))
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"smart_auto"}`)

	retry := common.RetryTimes
	retryParam := &RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	}
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(retryParam)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1801, channel.Id)
	require.Equal(t, "vip", selectedGroup)
	autoGroup, exists := common.GetContextKey(c, constant.ContextKeyAutoGroup)
	require.True(t, exists)
	require.Equal(t, "vip", autoGroup)
	require.Equal(t, 0, retryParam.GetRetry())

	retryParam.IncreaseRetry()
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(retryParam)
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1802, channel.Id)
	require.Equal(t, "backup", selectedGroup)
	autoGroup, exists = common.GetContextKey(c, constant.ContextKeyAutoGroup)
	require.True(t, exists)
	require.Equal(t, "backup", autoGroup)
}

func TestCacheGetRandomSatisfiedChannelRoutingStrategyStaysOnFirstGroupWithoutCrossGroupRetry(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","cheap":"低价分组","expensive":"高价分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["cheap","expensive"]`))
	createChannelSelectFixture(t, db, 1811, "cheap", 10)
	createChannelSelectFixture(t, db, 1812, "expensive", 10)
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:  model.RoutingStrategyPriceFirst,
		UserGroup: "default",
		Groups:    `["cheap","expensive"]`,
		Scores:    `{"cheap":{"group":"cheap","rank":1},"expensive":{"group":"expensive","rank":2}}`,
		Config:    `{}`,
	}))
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, false)
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"price_first"}`)

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1811, channel.Id)
	require.Equal(t, "cheap", selectedGroup)

	retry++
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1811, channel.Id)
	require.Equal(t, "cheap", selectedGroup)
	require.Equal(t, "cheap", common.GetContextKeyString(c, constant.ContextKeyAutoGroup))
}

func TestCacheGetRandomSatisfiedChannelRoutingStrategyExcludesTokenGroups(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip","backup"]`))
	createChannelSelectFixture(t, db, 1901, "default", 10)
	createChannelSelectFixture(t, db, 1902, "vip", 10)
	createChannelSelectFixture(t, db, 1903, "backup", 10)
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:  model.RoutingStrategyPriceFirst,
		UserGroup: "default",
		Groups:    `["vip","backup","default"]`,
		Scores:    `{"vip":{"group":"vip","rank":1},"backup":{"group":"backup","rank":2},"default":{"group":"default","rank":3}}`,
		Config:    `{}`,
	}))
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"price_first","excluded_groups":["vip"]}`)

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1903, channel.Id)
	require.Equal(t, "backup", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelRoutingStrategyDoesNotFallbackWhenAllGroupsExcluded(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["vip"]`))
	createChannelSelectFixture(t, db, 2001, "vip", 10)
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:  model.RoutingStrategySmartAuto,
		UserGroup: "default",
		Groups:    `["vip"]`,
		Scores:    `{"vip":{"group":"vip","rank":1}}`,
		Config:    `{}`,
	}))
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"smart_auto","excluded_groups":["vip"]}`)

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.Nil(t, channel)
	require.Equal(t, "auto", selectedGroup)
	_, exists := common.GetContextKey(c, constant.ContextKeyAutoGroup)
	require.False(t, exists)
}

func TestCacheGetRandomSatisfiedChannelFallsBackWhenRoutingStrategySnapshotMissing(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))
	createChannelSelectFixture(t, db, 1701, "default", 10)
	createChannelSelectFixture(t, db, 1702, "vip", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"success_first"}`)

	retry := 0
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "auto",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1701, channel.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelUsesRetryPolicyRecoveryGroups(t *testing.T) {
	db := openChannelSelectTestDB(t)
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","codex-primary":"Codex主分组","codex-backup":"Codex备用分组"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"codex-primary":1,"codex-backup":1}`))
	createChannelSelectFixture(t, db, 1401, "codex-primary", 10)
	createChannelSelectFixture(t, db, 1402, "codex-backup", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	SetRetryPolicyRecovery(c, operation_setting.RetryPolicyDecision{
		Matched:     true,
		ShouldRetry: true,
		Source:      operation_setting.RetryPolicySourceGlobal,
		RuleName:    "codex encrypted recovery",
		RetryGroups: []string{"codex-primary", "codex-backup"},
		MaxRetries:  2,
	})

	retry := 1
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1401, channel.Id)
	require.Equal(t, "codex-primary", selectedGroup)

	retry = 2
	channel, selectedGroup, err = CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1402, channel.Id)
	require.Equal(t, "codex-backup", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelDoesNotResetRetryAtLastPolicyGroup(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		common.MemoryCacheEnabled = false
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","backup":"备用分组"}`))
	createChannelSelectFixture(t, db, 1301, "backup", 10)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(c, constant.ContextKeyTokenCrossGroupRetry, true)
	common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"ordered","groups":["backup"]}`)

	retry := common.RetryTimes
	channel, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "backup",
		ModelName:  "gpt-test",
		Retry:      &retry,
	})
	require.NoError(t, err)
	require.NotNil(t, channel)
	require.Equal(t, 1301, channel.Id)
	require.Equal(t, "backup", selectedGroup)
	require.Equal(t, common.RetryTimes, retry)
	index, ok := common.GetContextKey(c, constant.ContextKeyAutoGroupIndex)
	require.True(t, ok)
	require.Equal(t, 0, index)
}

func TestNormalizeTokenGroupPolicyAutoClearsConcreteGroups(t *testing.T) {
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	group, policy, err := NormalizeTokenGroupPolicy(`{"type":"ordered","groups":["vip","auto","backup"]}`, "default")
	require.NoError(t, err)
	require.Equal(t, "auto", group)
	require.Empty(t, policy)
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
	require.Equal(t, 1001, channel.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelAllowsEquivalentNativeConfigurableProfile(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	channel := model.Channel{
		Id:       1001,
		Type:     constant.ChannelTypeConfigurable,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     "seedance-api-assets",
		Group:    "default",
		Models:   "doubao-seedance-2-0-fast-260128",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "doubao-seedance-2-api-assets",
		},
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "doubao-seedance-2-0-fast-260128",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"doubao-seedance-2-0-fast-260128"}`))
	c.Set("configurable_native_profile_id", "doubao-seedance-2")
	c.Set("configurable_native_profile_ids", []string{"doubao-seedance-2", "doubao-seedance-2-api-assets"})

	selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "doubao-seedance-2-0-fast-260128",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
	require.Equal(t, "default", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelAllowsSeedanceArkTaskAssetsNativeProfile(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})

	weight := uint(100)
	priority := int64(20)
	autoBan := 1
	channel := model.Channel{
		Id:       125,
		Type:     constant.ChannelTypeConfigurable,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     "seedance2 ephone",
		Group:    "Volcengine",
		Models:   "doubao-seedance-2-0-260128",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "seedance2-ark-task-assets",
		},
	})
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "Volcengine",
		Model:     "doubao-seedance-2-0-260128",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"doubao-seedance-2-0-260128"}`))
	c.Set("configurable_native_profile_id", "doubao-seedance-2")
	c.Set("configurable_native_profile_ids", []string{"doubao-seedance-2", "doubao-seedance-2-api-assets", "seedance2-ark-task-assets"})

	selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "Volcengine",
		ModelName:  "doubao-seedance-2-0-260128",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
	require.Equal(t, "Volcengine", selectedGroup)
}

func TestCacheGetRandomSatisfiedChannelAllowsNativeSeedanceVolcEngineChannel(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})

	weight := uint(100)
	priority := int64(10)
	autoBan := 1
	channel := model.Channel{
		Id:       1001,
		Type:     constant.ChannelTypeVolcEngine,
		Key:      "sk-test",
		Status:   common.ChannelStatusEnabled,
		Name:     "volcengine-seedance",
		Group:    "default",
		Models:   "doubao-seedance-2-0-fast-260128",
		Weight:   &weight,
		Priority: &priority,
		AutoBan:  &autoBan,
	}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&model.Ability{
		Group:     "default",
		Model:     "doubao-seedance-2-0-fast-260128",
		ChannelId: channel.Id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
	model.InitChannelCache()

	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"doubao-seedance-2-0-fast-260128"}`))
	c.Set("configurable_native_profile_id", "doubao-seedance-2")
	c.Set("configurable_native_profile_ids", []string{"doubao-seedance-2", "doubao-seedance-2-api-assets"})

	selected, selectedGroup, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        c,
		TokenGroup: "default",
		ModelName:  "doubao-seedance-2-0-fast-260128",
		Retry:      common.GetPointer(0),
	})
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, channel.Id, selected.Id)
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
			NativeModes: []string{string(conversion.RequestModeOpenAIChat)},
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

func TestBuildProtocolChannelFilterAllowsGeminiImageNativeOrImageCanonical(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(`{
		"contents":[{"parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["TEXT","IMAGE"]}
	}`))

	filter := BuildProtocolChannelFilter(&RetryParam{
		Ctx:       c,
		ModelName: "gemini-3.1-flash-image-preview",
	})
	require.NotNil(t, filter)

	geminiOnly := model.Channel{}
	geminiOnly.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeGeminiGenerateContent)},
		},
	})
	require.True(t, filter(&geminiOnly))

	imageCapable := model.Channel{}
	imageCapable.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeOpenAIImageGenerations)},
		},
	})
	require.True(t, filter(&imageCapable))
}

func TestBuildProtocolChannelFilterAllowsNativeGeminiImageGenerateContent(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(`{
		"contents":[{"parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["TEXT","IMAGE"]}
	}`))

	filter := BuildProtocolChannelFilter(&RetryParam{
		Ctx:       c,
		ModelName: "gemini-3.1-flash-image-preview",
	})
	require.NotNil(t, filter)

	geminiNative := model.Channel{}
	geminiNative.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeGeminiGenerateContent)},
		},
	})
	require.True(t, filter(&geminiNative))
}

func TestCacheGetRandomSatisfiedChannelRejectsWhenAllImageChatChannelsCannotRunCanonicalImage(t *testing.T) {
	db := openChannelSelectTestDB(t)
	t.Cleanup(func() {
		common.MemoryCacheEnabled = false
	})

	createImageChannelSelectFixture(t, db, 1001, "default", 20, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeOpenAIChat)},
		},
	})
	createImageChannelSelectFixture(t, db, 1002, "default", 10, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeOpenAIChat)},
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
	require.Contains(t, err.Error(), string(conversion.RequestModeOpenAIImageGenerations))
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
	require.Equal(t, 1001, channel.Id)
	require.Equal(t, "default", selectedGroup)
}
