package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelRatioPersistsThroughCreateAndUpdate(t *testing.T) {
	originDB := DB
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))
	DB = db
	t.Cleanup(func() { DB = originDB })

	ratio := 1.35
	channel := Channel{
		Type:         1,
		Key:          "sk-test",
		Name:         "cost-aware-channel",
		Models:       "gpt-4o",
		Group:        "default",
		ChannelRatio: &ratio,
	}
	require.NoError(t, channel.Insert())

	persisted, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, persisted.ChannelRatio)
	require.Equal(t, 1.35, *persisted.ChannelRatio)

	updatedRatio := 0.85
	persisted.ChannelRatio = &updatedRatio
	require.NoError(t, persisted.Update())

	updated, err := GetChannelById(channel.Id, true)
	require.NoError(t, err)
	require.NotNil(t, updated.ChannelRatio)
	require.Equal(t, 0.85, *updated.ChannelRatio)
}
