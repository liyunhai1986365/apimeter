package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestSeedanceNativeFetchDirectVolcEngineChannelUsesOfficialShape(t *testing.T) {
	for _, channelType := range []int{constant.ChannelTypeVolcEngine, constant.ChannelTypeDoubaoVideo} {
		t.Run(fmt.Sprint(channelType), func(t *testing.T) {
			testSeedanceNativeFetchDirectChannel(t, channelType)
		})
	}
}

func testSeedanceNativeFetchDirectChannel(t *testing.T, channelType int) {
	setupRelayTaskTestDB(t)

	var fetchCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v3/contents/generations/tasks/cgt-upstream", r.URL.Path)
		require.Equal(t, "Bearer sk-volcengine", r.Header.Get("Authorization"))
		if fetchCount.Add(1) > 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"cgt-upstream",
			"model":"doubao-seedance-2-0-260128",
			"status":"succeeded",
			"content":{"video_url":"https://cdn.example/direct.mp4"},
			"created_at":1789026244,
			"updated_at":1789026397,
			"duration":5,
			"resolution":"720p",
			"ratio":"16:9",
			"framespersecond":24,
			"usage":{"completion_tokens":108900,"total_tokens":108900}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      9105,
		Type:    channelType,
		Key:     "sk-volcengine",
		BaseURL: common.GetPointer(upstream.URL),
		Status:  common.ChannelStatusEnabled,
		Name:    "volcengine",
		Models:  "doubao-seedance-2-0-260128",
		Group:   "default",
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Task{
		CreatedAt: 1789026244,
		UpdatedAt: 1789026397,
		TaskID:    "task_direct",
		UserId:    7,
		ChannelId: channel.Id,
		Platform:  constant.TaskPlatform(fmt.Sprint(channelType)),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "cgt-upstream",
		},
		Properties: model.Properties{
			OriginModelName:   "doubao-seedance-2-0-260128",
			UpstreamModelName: "doubao-seedance-2-0-260128",
		},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_direct", nil)
	c.Set("id", 7)
	c.Set("task_id", "task_direct")

	body, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.Equal(t, "cgt-upstream", gjson.GetBytes(body, "id").String())
	require.Equal(t, "succeeded", gjson.GetBytes(body, "status").String())
	require.Equal(t, "https://cdn.example/direct.mp4", gjson.GetBytes(body, "content.video_url").String())
	require.False(t, gjson.GetBytes(body, "task").Exists())
	require.False(t, gjson.GetBytes(body, "outputs").Exists())
	require.False(t, gjson.GetBytes(body, "metadata").Exists())
	require.False(t, gjson.GetBytes(body, "error").Exists())

	// A later upstream outage must use the persisted official task result.
	fallback, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.JSONEq(t, string(body), string(fallback))
	require.EqualValues(t, 2, fetchCount.Load())

	// The generic endpoint keeps its own response contract and stored fetch.
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_direct", nil)
	generic, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.Equal(t, "video", gjson.GetBytes(generic, "object").String())
	require.Equal(t, "completed", gjson.GetBytes(generic, "status").String())
	require.EqualValues(t, 2, fetchCount.Load())
}

func TestSeedanceNativeFetchReturnsServiceInferenceFieldsAtRoot(t *testing.T) {
	for _, profileID := range []string{"doubao-seedance-max-service-inference", "seedance2-service-inference"} {
		t.Run(profileID, func(t *testing.T) {
			testSeedanceNativeFetchReturnsServiceInferenceFieldsAtRoot(t, profileID)
		})
	}
}

func testSeedanceNativeFetchReturnsServiceInferenceFieldsAtRoot(t *testing.T, profileID string) {
	upstreamModel := "dreamina-seedance-2-0-fast-hc"
	if profileID == "doubao-seedance-max-service-inference" {
		upstreamModel = "doubao-seedance-2-0-mini-260615-max"
	}
	setupRelayTaskTestDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v2/video/tasks/mvt-upstream", r.URL.Path)
		require.Equal(t, "Bearer sk-seedance", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"task":{
				"id":"mvt-upstream",
				"status":"completed",
				"model":"` + upstreamModel + `",
				"duration_seconds":4,
				"outputs":["https://cdn.example/seedance.mp4"],
				"error":null,
				"created_at":"2026-08-24T09:47:19.113Z",
				"completed_at":"2026-08-24T09:48:41.934Z",
				"usage":{"completion_tokens":40594,"total_tokens":40594},
				"metadata":{
					"updated_at":1787564921,
					"seed":78256,
					"resolution":"480p",
					"ratio":"16:9",
					"duration":4,
					"framespersecond":24,
					"generate_audio":true,
					"draft":false
				}
			}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      9104,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "sk-seedance",
		BaseURL: common.GetPointer(upstream.URL),
		Status:  common.ChannelStatusEnabled,
		Name:    "seedance-service-inference",
		Models:  upstreamModel,
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: profileID,
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Task{
		CreatedAt:  1787564840,
		UpdatedAt:  1787564856,
		TaskID:     "task_public",
		UserId:     7,
		ChannelId:  channel.Id,
		Platform:   constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeConfigurable)),
		Status:     model.TaskStatusInProgress,
		Progress:   "30%",
		SubmitTime: 1787564840,
		Properties: model.Properties{
			OriginModelName:   upstreamModel,
			UpstreamModelName: upstreamModel,
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "mvt-upstream",
		},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_public", nil)
	c.Set("id", 7)
	c.Set("task_id", "task_public")
	c.Set("configurable_native_profile_id", profileID)

	body, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.False(t, gjson.GetBytes(body, "task").Exists())
	require.Equal(t, "mvt-upstream", gjson.GetBytes(body, "id").String())
	require.Equal(t, "succeeded", gjson.GetBytes(body, "status").String())
	require.Equal(t, upstreamModel, gjson.GetBytes(body, "model").String())
	require.Equal(t, "https://cdn.example/seedance.mp4", gjson.GetBytes(body, "content.video_url").String())
	require.Equal(t, int64(40594), gjson.GetBytes(body, "usage.total_tokens").Int())
	require.Equal(t, "480p", gjson.GetBytes(body, "resolution").String())
	require.True(t, gjson.GetBytes(body, "generate_audio").Bool())
	require.True(t, gjson.GetBytes(body, "draft").Exists())
	require.False(t, gjson.GetBytes(body, "draft").Bool())
	require.False(t, gjson.GetBytes(body, "metadata").Exists())
	require.False(t, gjson.GetBytes(body, "code").Exists())
	require.False(t, gjson.GetBytes(body, "data").Exists())
	require.False(t, gjson.GetBytes(body, "user_id").Exists())
	require.False(t, gjson.GetBytes(body, "channel_id").Exists())
	require.False(t, gjson.GetBytes(body, "quota").Exists())
}

func TestWan3NativeFetchUsesBuiltInAliChannelAndPreservesDashScopeShape(t *testing.T) {
	setupRelayTaskTestDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/api/v1/tasks/upstream-wan3", r.URL.Path)
		require.Equal(t, "Bearer sk-wan3", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"request_id":"req-wan3",
			"output":{"task_id":"upstream-wan3","task_status":"SUCCEEDED","video_url":"https://cdn.example/wan3.mp4","orig_prompt":"cat"},
			"usage":{"video_count":1,"input_video_duration":1.5,"output_video_duration":4.5,"fps":24,"SR":1080,"ratio":"16:9"},
			"future_field":{"kept":true}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      9110,
		Type:    constant.ChannelTypeAli,
		Key:     "sk-wan3",
		BaseURL: common.GetPointer(upstream.URL),
		Status:  common.ChannelStatusEnabled,
		Name:    "ali-wan3",
		Models:  "wan3.0-video",
		Group:   "default",
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:    "task_public_wan3",
		UserId:    7,
		ChannelId: channel.Id,
		Platform:  constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeAli)),
		Status:    model.TaskStatusInProgress,
		Progress:  "20%",
		Properties: model.Properties{
			OriginModelName:   "wan3.0-video",
			UpstreamModelName: "wan3.0-video",
		},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "upstream-wan3"},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_public_wan3", nil)
	c.Set("id", 7)
	c.Set("task_id", "task_public_wan3")
	c.Set("configurable_native_profile_id", "dashscope-wan3-video")

	body, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.Equal(t, "task_public_wan3", gjson.GetBytes(body, "output.task_id").String())
	require.Equal(t, "SUCCEEDED", gjson.GetBytes(body, "output.task_status").String())
	require.Equal(t, 1.5, gjson.GetBytes(body, "usage.input_video_duration").Float())
	require.True(t, gjson.GetBytes(body, "future_field.kept").Bool())

	reloaded, exists, err := model.GetByTaskId(7, "task_public_wan3")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusSuccess, reloaded.Status)
	require.Equal(t, "https://cdn.example/wan3.mp4", reloaded.GetResultURL())
}

func TestSeedanceNativeFetchNormalizesLegacySnapshotWhenUpstreamFetchFails(t *testing.T) {
	setupRelayTaskTestDB(t)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/video/tasks/mvt-upstream", r.URL.Path)
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      9105,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "sk-seedance",
		BaseURL: common.GetPointer(upstream.URL),
		Status:  common.ChannelStatusEnabled,
		Name:    "seedance-service-inference-fallback",
		Models:  "doubao-seedance-2-0-fast-260128",
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"},
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	storedResponse := []byte(`{
		"code":"success",
		"message":"",
		"data":{
			"task_id":"task_hFov9erMp4JxHoKFLHeeYIInprezMjZv",
			"status":"SUCCESS",
			"result_url":"https://cdn.example/stored-result.mp4",
			"data":{"task":{
				"status":"completed",
				"usage":{"completion_tokens":40594,"total_tokens":40594},
				"metadata":{
					"seed":78256,
					"resolution":"480p",
					"ratio":"16:9",
					"duration":4,
					"framespersecond":24,
					"generate_audio":true,
					"draft":false,
					"priority":0,
					"output_format":"mp4"
				}
			}}
		}
	}`)
	require.NoError(t, model.DB.Create(&model.Task{
		CreatedAt:  1787564840,
		UpdatedAt:  1787564976,
		TaskID:     "task_hFov9erMp4JxHoKFLHeeYIInprezMjZv",
		UserId:     8,
		ChannelId:  channel.Id,
		Platform:   constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeConfigurable)),
		Status:     model.TaskStatusSuccess,
		Progress:   "100%",
		FinishTime: 1787564976,
		Properties: model.Properties{
			OriginModelName:   "doubao-seedance-2-0-fast-260128",
			UpstreamModelName: "dreamina-seedance-2-0-fast-hc",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "mvt-upstream",
			ResultURL:      "https://cdn.example/stored-result.mp4",
		},
		Data: storedResponse,
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task_hFov9erMp4JxHoKFLHeeYIInprezMjZv", nil)
	c.Set("id", 8)
	c.Set("task_id", "task_hFov9erMp4JxHoKFLHeeYIInprezMjZv")
	c.Set("configurable_native_profile_id", "seedance2-service-inference")

	body, taskErr := videoFetchByIDRespBodyBuilder(c)
	require.Nil(t, taskErr)
	require.False(t, gjson.GetBytes(body, "task").Exists())
	require.Equal(t, "mvt-upstream", gjson.GetBytes(body, "id").String())
	require.Equal(t, "succeeded", gjson.GetBytes(body, "status").String())
	require.Equal(t, "https://cdn.example/stored-result.mp4", gjson.GetBytes(body, "content.video_url").String())
	require.Equal(t, int64(40594), gjson.GetBytes(body, "usage.total_tokens").Int())
	require.Equal(t, int64(4), gjson.GetBytes(body, "duration").Int())
	require.False(t, gjson.GetBytes(body, "code").Exists())
	require.False(t, gjson.GetBytes(body, "data").Exists())
	require.False(t, gjson.GetBytes(body, "priority").Exists())
	require.Equal(t, "mp4", gjson.GetBytes(body, "output_format").String())
}

func TestVideoGenerationsFetchRealtimeConfigurableChannelSettlesTerminalTask(t *testing.T) {
	setupRelayTaskTestDB(t)

	const userID = 7
	const tokenID = 99
	const channelID = 9102
	const preConsumed = 1250000
	const actualQuota = 32611
	const initialUserQuota = 1000000
	const initialTokenQuota = 2000000

	expr := `tier("480_720p_no_video_input", c * 1.4)`
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
		require.Equal(t, "/v2/video/tasks/upstream-seedance-task", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"task":{
				"id":"upstream-seedance-task",
				"status":"completed",
				"model":"dreamina-seedance-2-0-mini-260615-max",
				"outputs":["https://cdn.example/seedance.mp4"],
				"metadata":{
					"usage":{"completion_tokens":50638,"total_tokens":50638}
				}
			}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:           channelID,
		ChannelRatio: common.GetPointer(0.6),
		Type:         constant.ChannelTypeConfigurable,
		Key:          "sk-seedance",
		BaseURL:      common.GetPointer(upstream.URL),
		Status:       common.ChannelStatusEnabled,
		Name:         "seedance-service-inference",
		Models:       "dreamina-seedance-2-0-mini-260615",
		Group:        "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "seedance2-service-inference",
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
			OriginModelName:   "dreamina-seedance-2-0-mini-260615",
			UpstreamModelName: "dreamina-seedance-2-0-mini-260615-max",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-seedance-task",
			BillingSource:  service.BillingSourceWallet,
			TokenId:        tokenID,
			BillingContext: &model.TaskBillingContext{
				DeferredCost:    true,
				GroupRatio:      0.92,
				OriginModelName: "dreamina-seedance-2-0-mini-260615",
				TieredBillingSnapshot: &billingexpr.BillingSnapshot{
					BillingMode:               "tiered_expr",
					ModelName:                 "dreamina-seedance-2-0-mini-260615",
					ExprString:                expr,
					ExprHash:                  billingexpr.ExprHashString(expr),
					GroupRatio:                0.92,
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
	require.Equal(t, model.TaskCostSettled, gjson.Get(log.Other, "task_cost_state").String())
	expectedCost := common.QuotaRound(float64(common.QuotaRound(float64(actualQuota)/0.92)) * 0.6)
	require.Equal(t, int64(expectedCost), gjson.Get(log.Other, "cost_quota").Int())
	require.Equal(t, int64(actualQuota-expectedCost), gjson.Get(log.Other, "profit_quota").Int())
	require.True(t, reloaded.PrivateData.BillingContext.CostSettled)
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
		require.Equal(t, "/v2/video/tasks/upstream-seedance-task", r.URL.Path)
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
