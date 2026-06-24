package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestApplyTaskOtherRatiosAppliesSeedanceTieredExprSecondsPreConsumeQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
		},
		PriceData: types.PriceData{
			Quota: 250000,
			OtherRatios: map[string]float64{
				"seconds": 5,
			},
		},
	}

	applyTaskOtherRatios(info, "doubao-seedance-2-0-260128")

	require.Equal(t, 1250000, info.PriceData.Quota)
}

func TestApplyTaskOtherRatiosKeepsPerCallTaskMultiplier(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 1000,
			OtherRatios: map[string]float64{
				"seconds": 5,
				"size":    2,
			},
		},
	}

	applyTaskOtherRatios(info, "sora-2")

	require.Equal(t, 10000, info.PriceData.Quota)
}

func TestVideoGenerationsFetchRealtimeConfigurableChannelAndReturnsOpenAIShape(t *testing.T) {
	setupRelayTaskTestDB(t)

	var requestedPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer sk-seedance", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"upstream-seedance-task",
			"status":"completed",
			"model":"doubao-seedance-2-0-260128",
			"content":{"video_url":"https://cdn.example/seedance.mp4"},
			"usage":{"completion_tokens":40594,"total_tokens":40594}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      9101,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "sk-seedance",
		BaseURL: common.GetPointer(upstream.URL),
		Status:  common.ChannelStatusEnabled,
		Name:    "seedance",
		Models:  "doubao-seedance-2-0-260128",
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "doubao-seedance-2",
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: channel.Id,
		Platform:  constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeConfigurable)),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		Properties: model.Properties{
			OriginModelName:   "doubao-seedance-2-0-260128",
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-seedance-task",
		},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/task_public", nil)
	c.Set("id", 7)
	c.Set("task_id", "task_public")

	body, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.Equal(t, "/api/v3/contents/generations/tasks/upstream-seedance-task", requestedPath)
	require.Equal(t, "success", gjson.GetBytes(body, "code").String())
	require.Equal(t, "task_public", gjson.GetBytes(body, "data.task_id").String())
	require.Equal(t, "SUCCESS", gjson.GetBytes(body, "data.status").String())
	require.Equal(t, "https://cdn.example/seedance.mp4", gjson.GetBytes(body, "data.result_url").String())

	reloaded, exists, err := model.GetByTaskId(7, "task_public")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.Equal(t, "https://cdn.example/seedance.mp4", reloaded.GetResultURL())
}

func setupRelayTaskTestDB(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}
