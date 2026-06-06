package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelUpdatePersistsRetryEnabledFalse(t *testing.T) {
	originalDB := DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		DB = originalDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		InitColForTest()
	})

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	InitColForTest()
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}))

	channel := Channel{
		Name:         "retry-switch",
		Type:         1,
		Key:          "sk-test",
		Status:       common.ChannelStatusEnabled,
		Models:       "gpt-4",
		Group:        "default",
		RetryEnabled: common.GetPointer(true),
	}
	require.NoError(t, channel.Insert())

	channel.RetryEnabled = common.GetPointer(false)
	require.NoError(t, channel.Update())

	updated, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, updated.RetryEnabled)
	require.False(t, updated.IsRetryEnabled())
}
