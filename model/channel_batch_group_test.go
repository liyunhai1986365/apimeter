package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelBatchGroupTestDB(t *testing.T) {
	t.Helper()

	originDB := DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}))

	DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	InitColForTest()

	t.Cleanup(func() {
		DB = originDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		InitColForTest()
	})
}

func TestApplyChannelRatioFilterSupportsComparisons(t *testing.T) {
	setupChannelBatchGroupTestDB(t)

	ratioOne := 1.0
	ratioThree := 3.0
	ratioFive := 5.0
	channels := []Channel{
		{Type: 1, Key: "sk-1", Name: "ratio-one", Models: "gpt-4o", Group: "default", ChannelRatio: &ratioOne},
		{Type: 1, Key: "sk-3", Name: "ratio-three", Models: "gpt-4o", Group: "default", ChannelRatio: &ratioThree},
		{Type: 1, Key: "sk-5", Name: "ratio-five", Models: "gpt-4o", Group: "default", ChannelRatio: &ratioFive},
	}
	require.NoError(t, BatchInsertChannels(channels))

	var lessThanThree []Channel
	err := ApplyChannelRatioFilter(DB.Model(&Channel{}), &ChannelRatioFilter{
		Operator: "<",
		Value:    3,
	}).Order("id ASC").Find(&lessThanThree).Error
	require.NoError(t, err)
	require.Len(t, lessThanThree, 1)
	require.Equal(t, "ratio-one", lessThanThree[0].Name)

	var equalFive []Channel
	err = ApplyChannelRatioFilter(DB.Model(&Channel{}), &ChannelRatioFilter{
		Operator: "eq",
		Value:    5,
	}).Find(&equalFive).Error
	require.NoError(t, err)
	require.Len(t, equalFive, 1)
	require.Equal(t, "ratio-five", equalFive[0].Name)
}

func TestBatchSetChannelGroupsUpdatesAbilities(t *testing.T) {
	setupChannelBatchGroupTestDB(t)

	ratio := 1.0
	channel := Channel{
		Type:         1,
		Key:          "sk-test",
		Name:         "grouped-channel",
		Models:       "gpt-4o,gpt-4.1",
		Group:        "default",
		ChannelRatio: &ratio,
	}
	require.NoError(t, channel.Insert())

	require.NoError(t, BatchSetChannelGroups([]int{channel.Id}, "vip,priority"))

	var updated Channel
	require.NoError(t, DB.First(&updated, channel.Id).Error)
	require.Equal(t, "vip,priority", updated.Group)

	var abilities []Ability
	require.NoError(t, DB.Where("channel_id = ?", channel.Id).Order("`group` ASC, model ASC").Find(&abilities).Error)
	require.Len(t, abilities, 4)
	require.ElementsMatch(t, []string{"vip:gpt-4.1", "vip:gpt-4o", "priority:gpt-4.1", "priority:gpt-4o"}, []string{
		abilities[0].Group + ":" + abilities[0].Model,
		abilities[1].Group + ":" + abilities[1].Model,
		abilities[2].Group + ":" + abilities[2].Model,
		abilities[3].Group + ":" + abilities[3].Model,
	})
}
