package service

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openRoutingStrategyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}, &model.PerfMetric{}))
	model.DB = db

	t.Cleanup(func() {
		model.DB = originalDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestRefreshRoutingStrategySnapshotsKeepsManualOverride(t *testing.T) {
	_ = openRoutingStrategyTestDB(t)
	originalGroupRatio := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		_ = setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组"}`)
		_ = setting.UpdateAutoGroupsByJsonString(`["default"]`)
		_ = ratio_setting.UpdateGroupRatioByJSONString(originalGroupRatio)
	})

	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组","vip":"vip分组","backup":"备用分组"}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip","backup"]`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.8,"backup":0.6}`))
	require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{
		Strategy:       model.RoutingStrategyPriceFirst,
		UserGroup:      "default",
		Groups:         `["vip"]`,
		Scores:         `{}`,
		Config:         `{}`,
		ManualOverride: true,
	}))

	result, err := RefreshRoutingStrategySnapshots(context.Background())
	require.NoError(t, err)
	require.Greater(t, result.UpdatedSnapshots, 0)

	snapshot, err := model.GetRoutingStrategySnapshot(model.RoutingStrategyPriceFirst, "default")
	require.NoError(t, err)
	require.Equal(t, `["vip"]`, snapshot.Groups)
	require.True(t, snapshot.ManualOverride)
}
