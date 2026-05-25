package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelControllerBatchTestDB(t *testing.T) {
	t.Helper()

	originDB := model.DB
	originLogDB := model.LOG_DB
	originUsingSQLite := common.UsingSQLite
	originUsingMySQL := common.UsingMySQL
	originUsingPostgreSQL := common.UsingPostgreSQL
	originRedisEnabled := common.RedisEnabled

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColForTest()
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		model.DB = originDB
		model.LOG_DB = originLogDB
		common.UsingSQLite = originUsingSQLite
		common.UsingMySQL = originUsingMySQL
		common.UsingPostgreSQL = originUsingPostgreSQL
		common.RedisEnabled = originRedisEnabled
		model.InitColForTest()
	})
}

func TestGetAllChannelsFiltersByRatioQuery(t *testing.T) {
	setupChannelControllerBatchTestDB(t)

	ratioOne := 1.0
	ratioFive := 5.0
	require.NoError(t, model.BatchInsertChannels([]model.Channel{
		{Type: 1, Key: "sk-1", Name: "low-ratio", Models: "gpt-4o", Group: "default", ChannelRatio: &ratioOne},
		{Type: 1, Key: "sk-5", Name: "high-ratio", Models: "gpt-4o", Group: "default", ChannelRatio: &ratioFive},
	}))

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/channel?ratio_op=lt&ratio_value=3", nil)

	GetAllChannels(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "low-ratio")
	require.NotContains(t, recorder.Body.String(), "high-ratio")
}

func TestBatchSetChannelGroupsControllerUpdatesAbilities(t *testing.T) {
	setupChannelControllerBatchTestDB(t)

	ratio := 1.0
	channel := model.Channel{
		Type:         1,
		Key:          "sk-test",
		Name:         "controller-grouped-channel",
		Models:       "gpt-4o",
		Group:        "default",
		ChannelRatio: &ratio,
	}
	require.NoError(t, channel.Insert())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/channel/batch/groups", strings.NewReader(fmt.Sprintf(`{"ids":[%d],"groups":["vip","priority"]}`, channel.Id)))
	c.Request.Header.Set("Content-Type", "application/json")

	BatchSetChannelGroups(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	var updated model.Channel
	require.NoError(t, model.DB.First(&updated, channel.Id).Error)
	require.Equal(t, "vip,priority", updated.Group)

	var abilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ? AND "+model.CommonGroupCol()+" IN ?", channel.Id, []string{"vip", "priority"}).Count(&abilityCount).Error)
	require.Equal(t, int64(2), abilityCount)
}

func TestRemoveChannelModelControllerRemovesExactModelAndUpdatesAbilities(t *testing.T) {
	setupChannelControllerBatchTestDB(t)

	channel := model.Channel{
		Type:   1,
		Key:    "sk-test",
		Name:   "controller-model-removal-channel",
		Models: "gpt-4,gpt-4o,gpt-5",
		Group:  "default",
	}
	require.NoError(t, channel.Insert())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	c.Request = httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/channel/%d/models", channel.Id),
		bytes.NewBufferString(`{"model":"gpt-4"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	RemoveChannelModel(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	var refreshed model.Channel
	require.NoError(t, model.DB.First(&refreshed, channel.Id).Error)
	require.Equal(t, "gpt-4o,gpt-5", refreshed.Models)

	var removedAbilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ? AND model = ?", channel.Id, "gpt-4").
		Count(&removedAbilityCount).Error)
	require.Zero(t, removedAbilityCount)

	var keptAbilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ? AND model = ?", channel.Id, "gpt-4o").
		Count(&keptAbilityCount).Error)
	require.EqualValues(t, 1, keptAbilityCount)
}

func TestRemoveChannelModelControllerCanClearLastModel(t *testing.T) {
	setupChannelControllerBatchTestDB(t)

	channel := model.Channel{
		Type:   1,
		Key:    "sk-test",
		Name:   "controller-last-model-removal-channel",
		Models: "gpt-4o",
		Group:  "default",
	}
	require.NoError(t, channel.Insert())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	c.Request = httptest.NewRequest(
		http.MethodDelete,
		fmt.Sprintf("/api/channel/%d/models", channel.Id),
		bytes.NewBufferString(`{"model":"gpt-4o"}`),
	)
	c.Request.Header.Set("Content-Type", "application/json")

	RemoveChannelModel(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), `"success":true`)

	var refreshed model.Channel
	require.NoError(t, model.DB.First(&refreshed, channel.Id).Error)
	require.Empty(t, refreshed.Models)

	var abilityCount int64
	require.NoError(t, model.DB.Model(&model.Ability{}).
		Where("channel_id = ?", channel.Id).
		Count(&abilityCount).Error)
	require.Zero(t, abilityCount)
}
