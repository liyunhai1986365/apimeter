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
	"github.com/QuantumNous/new-api/service"
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

func TestApplyTaskOtherRatiosSkipsFixedPerRequestPrice(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota:    1000,
			UsePrice: true,
			OtherRatios: map[string]float64{
				"seconds": 5,
				"size":    2,
			},
		},
	}

	applyTaskOtherRatios(info, "sora-2")

	require.Equal(t, 1000, info.PriceData.Quota)
}

func TestApplyTaskOtherRatiosKeepsUsageBasedTaskMultiplier(t *testing.T) {
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

func TestApplyTaskOtherRatiosKeepsLegacyFixedPricePatch(t *testing.T) {
	originalPatches := constant.TaskPricePatches
	constant.TaskPricePatches = []string{"legacy-fixed-video"}
	t.Cleanup(func() {
		constant.TaskPricePatches = originalPatches
	})

	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 1000,
			OtherRatios: map[string]float64{
				"seconds": 5,
			},
		},
	}

	applyTaskOtherRatios(info, "legacy-fixed-video")

	require.Equal(t, 1000, info.PriceData.Quota)
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

func TestVideoGenerationsFetchRealtimeConfigurableChannelSettlesTerminalTask(t *testing.T) {
	setupRelayTaskTestDB(t)

	const userID = 7
	const tokenID = 99
	const channelID = 9102
	const preConsumed = 1250000
	const actualQuota = 100000
	const initialUserQuota = 1000000
	const initialTokenQuota = 2000000

	expr := `tier("base", c * 0.2)`
	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "seedance-user",
		Quota:    initialUserQuota,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:          tokenID,
		UserId:      userID,
		Key:         "sk-user-token",
		Name:        "seedance-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: initialTokenQuota,
	}).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v3/contents/generations/tasks/upstream-seedance-task", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"upstream-seedance-task",
			"status":"completed",
			"model":"doubao-seedance-2-0-260128",
			"content":{"video_url":"https://cdn.example/seedance.mp4"},
			"usage":{"completion_tokens":1000000,"total_tokens":1000000}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      channelID,
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
		UserId:    userID,
		ChannelId: channel.Id,
		TokenId:   tokenID,
		TokenName: "seedance-token",
		Platform:  constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeConfigurable)),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		Quota:     preConsumed,
		Group:     "default",
		Properties: model.Properties{
			OriginModelName:   "doubao-seedance-2-0-260128",
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-seedance-task",
			BillingSource:  service.BillingSourceWallet,
			TokenId:        tokenID,
			BillingContext: &model.TaskBillingContext{
				GroupRatio:      1,
				OriginModelName: "doubao-seedance-2-0-260128",
				TieredBillingSnapshot: &billingexpr.BillingSnapshot{
					BillingMode:               "tiered_expr",
					ModelName:                 "doubao-seedance-2-0-260128",
					ExprString:                expr,
					ExprHash:                  billingexpr.ExprHashString(expr),
					GroupRatio:                1,
					EstimatedPromptTokens:     0,
					EstimatedCompletionTokens: 0,
					EstimatedQuotaAfterGroup:  preConsumed,
					QuotaPerUnit:              common.QuotaPerUnit,
					ExprVersion:               billingexpr.ExprVersion(expr),
				},
			},
		},
		Data: []byte(`{"status":"running"}`),
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/task_public", nil)
	c.Set("id", userID)
	c.Set("task_id", "task_public")

	body, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.Equal(t, "SUCCESS", gjson.GetBytes(body, "data.status").String())

	reloaded, exists, err := model.GetByTaskId(userID, "task_public")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.NotZero(t, reloaded.FinishTime)
	require.Equal(t, actualQuota, reloaded.Quota)

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.Equal(t, initialUserQuota+(preConsumed-actualQuota), user.Quota)

	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	require.Equal(t, initialTokenQuota+(preConsumed-actualQuota), token.RemainQuota)

	var log model.Log
	require.NoError(t, model.LOG_DB.Order("id desc").First(&log).Error)
	require.Equal(t, model.LogTypeRefund, log.Type)
	require.Equal(t, preConsumed-actualQuota, log.Quota)
	require.Equal(t, "task_public", gjson.Get(log.Other, "task_id").String())
	require.Equal(t, float64(preConsumed), gjson.Get(log.Other, "pre_consumed_quota").Float())
	require.Equal(t, float64(actualQuota), gjson.Get(log.Other, "actual_quota").Float())
}

func TestVideoGenerationsFetchRealtimeConfigurableFailurePersistsReasonAndRefundsOnce(t *testing.T) {
	setupRelayTaskTestDB(t)

	const userID = 8
	const tokenID = 100
	const channelID = 9103
	const preConsumed = 1000000
	const initialUserQuota = 2000000
	const initialTokenQuota = 3000000
	const failureReason = "The request failed because the output audio may be related to copyright restrictions. Request id: request-123"

	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "seedance-failure-user",
		Quota:    initialUserQuota,
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		Id:          tokenID,
		UserId:      userID,
		Key:         "sk-user-token",
		Name:        "seedance-failure-token",
		Status:      common.TokenStatusEnabled,
		RemainQuota: initialTokenQuota,
	}).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/video/tasks/upstream-seedance-task", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"task":{
				"id":"upstream-seedance-task",
				"status":"failed",
				"error":"The request failed because the output audio may be related to copyright restrictions",
				"metadata":{
					"error":{
						"code":"OutputAudioSensitiveContentDetected.PolicyViolation",
						"message":"The request failed because the output audio may be related to copyright restrictions. Request id: request-123"
					}
				}
			}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      channelID,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "sk-seedance",
		BaseURL: common.GetPointer(upstream.URL),
		Status:  common.ChannelStatusEnabled,
		Name:    "seedance-service-inference",
		Models:  "dreamina-seedance-2-0-ep",
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "seedance2-service-inference",
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:    "task_failure",
		UserId:    userID,
		ChannelId: channel.Id,
		TokenId:   tokenID,
		TokenName: "seedance-failure-token",
		Platform:  constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeConfigurable)),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		Quota:     preConsumed,
		Group:     "default",
		Properties: model.Properties{
			OriginModelName:   "dreamina-seedance-2-0-ep",
			UpstreamModelName: "dreamina-seedance-2-0-ep",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-seedance-task",
			BillingSource:  service.BillingSourceWallet,
			TokenId:        tokenID,
			BillingContext: &model.TaskBillingContext{
				OriginModelName: "dreamina-seedance-2-0-ep",
			},
		},
		Data: []byte(`{"task":{"status":"running"}}`),
	}).Error)

	fetch := func() []byte {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/task_failure", nil)
		c.Set("id", userID)
		c.Set("task_id", "task_failure")
		body, taskErr := videoFetchByIDRespBodyBuilder(c)
		require.Nil(t, taskErr)
		return body
	}

	body := fetch()
	require.Equal(t, string(model.TaskStatusFailure), gjson.GetBytes(body, "data.status").String())
	require.Equal(t, failureReason, gjson.GetBytes(body, "data.fail_reason").String())

	reloaded, exists, err := model.GetByTaskId(userID, "task_failure")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, failureReason, reloaded.FailReason)
	require.NotZero(t, reloaded.FinishTime)

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.Equal(t, initialUserQuota+preConsumed, user.Quota)

	var token model.Token
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	require.Equal(t, initialTokenQuota+preConsumed, token.RemainQuota)

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.Equal(t, preConsumed, logs[0].Quota)
	require.Equal(t, "task_failure", gjson.Get(logs[0].Other, "task_id").String())
	require.Equal(t, failureReason, gjson.Get(logs[0].Other, "reason").String())

	_ = fetch()
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeRefund).Find(&logs).Error)
	require.Len(t, logs, 1)
	require.NoError(t, model.DB.First(&user, userID).Error)
	require.Equal(t, initialUserQuota+preConsumed, user.Quota)
	require.NoError(t, model.DB.First(&token, tokenID).Error)
	require.Equal(t, initialTokenQuota+preConsumed, token.RemainQuota)
}

func setupRelayTaskTestDB(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}, &model.User{}, &model.Token{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}
