package relay

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupImageTaskTestDB(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
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
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestRecordImageAsyncSubmitResponseStoresTaskAndKeepsPublicID(t *testing.T) {
	setupImageTaskTestDB(t)

	info := &relaycommon.RelayInfo{
		UserId:              7,
		UsingGroup:          "default",
		OriginModelName:     "gpt-image-2",
		IsAsyncImageRequest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ChannelId:   11,
		},
		PriceData: types.PriceData{
			QuotaToPreConsume: 123,
		},
	}

	apiErr := recordImageAsyncSubmitResponse(info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
	require.Nil(t, apiErr)

	task, exists, err := model.GetByTaskId(7, "7154f43c-7765-4e77-d51e-0db4eee107b0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "7154f43c-7765-4e77-d51e-0db4eee107b0", task.TaskID)
	require.Equal(t, "7154f43c-7765-4e77-d51e-0db4eee107b0", task.GetUpstreamTaskID())
	require.Equal(t, constant.TaskActionGenerate, task.Action)
	require.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), task.Status)
	require.True(t, task.PrivateData.AsyncImage)
	require.JSONEq(t, `{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`, string(task.Data))
}

func TestRecordImageAsyncSubmitResponseSkipsTaskWhenClientDidNotRequestAsync(t *testing.T) {
	setupImageTaskTestDB(t)

	info := &relaycommon.RelayInfo{
		UserId:          7,
		UsingGroup:      "default",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ChannelId:   11,
		},
		PriceData: types.PriceData{
			QuotaToPreConsume: 123,
		},
	}

	apiErr := recordImageAsyncSubmitResponse(info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
	require.Nil(t, apiErr)

	_, exists, err := model.GetByTaskId(7, "7154f43c-7765-4e77-d51e-0db4eee107b0")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestWaitImageAsyncSubmitResponseConvertsSucceededTaskToOpenAIImageResponse(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", r.URL.Path)
		require.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0","state":"succeeded","progress":0,"create_time":1779125744,"update_time":1779125807,"action":"generate","data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","file_name":"output.png"}]}}`))
	}))
	defer upstream.Close()

	info := &relaycommon.RelayInfo{
		StartTime:       time.Unix(1779125700, 0),
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "sk-upstream",
		},
	}

	router := gin.New()
	router.POST("/v1/images/generations", func(c *gin.Context) {
		body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
		require.Nil(t, apiErr)
		c.Data(http.StatusOK, "application/json", body)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"created": 1779125744,
		"data": [{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","b64_json":"","revised_prompt":""}]
	}`, recorder.Body.String())
}

func TestWaitImageAsyncSubmitResponseNormalizesDuomiGeminiTaskPayload(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)

	upstreamTaskID := "f5044b53-4e32-2ee6-48b4-73bd60cd6754"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/duomiapi.com/api/gemini/nano-banana/"+upstreamTaskID, r.URL.Path)
		require.Equal(t, "duomi-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"task_id":"f5044b53-4e32-2ee6-48b4-73bd60cd6754","state":"succeeded","data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg","file_name":"output.jpeg"}],"description":"done"},"create_time":"1757061229","update_time":"1757061252","msg":"","status":"3","action":"generate"},"exec_time":0.05,"ip":"118.125.2.163"}`))
	}))
	defer upstream.Close()

	info := &relaycommon.RelayInfo{
		StartTime:       time.Unix(1779125700, 0),
		OriginModelName: "gemini-3-pro-image-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: upstream.URL + "/duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}

	router := gin.New()
	router.POST("/v1/images/generations", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 50*time.Millisecond)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"code":200,"msg":"success","data":{"task_id":"f5044b53-4e32-2ee6-48b4-73bd60cd6754"}}`))
		require.Nil(t, apiErr)
		c.Data(http.StatusOK, "application/json", body)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"created": 1757061229,
		"data": [{"url":"https://cdn3.dmiapi.com/output.jpeg","b64_json":"","revised_prompt":""}]
	}`, recorder.Body.String())
}

func TestWaitImageAsyncSubmitResponseSkipsWhenClientRequestedAsync(t *testing.T) {
	info := &relaycommon.RelayInfo{
		IsAsyncImageRequest: true,
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations?async=true", nil)

	body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
	require.Nil(t, apiErr)
	require.Nil(t, body)
}

func TestWaitImageAsyncSubmitResponseUsesConfiguredWaitTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task_wait","state":"running","progress":0,"action":"generate","data":{"images":[]}}`))
	}))
	defer upstream.Close()

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "sk-upstream",
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ImageAsyncWaitTimeoutSeconds: common.GetPointer(2),
		},
	})

	body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"task_wait"}`))
	require.NotNil(t, apiErr)
	require.Nil(t, body)
	require.Contains(t, apiErr.Error(), "image async task wait timeout")
	require.Equal(t, 1, calls)
}

func TestWaitImageAsyncSubmitResponseCanDisableSyncWait(t *testing.T) {
	gin.SetMode(gin.TestMode)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: "https://upstream.example",
			ApiKey:         "sk-upstream",
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ImageAsyncWaitTimeoutSeconds: common.GetPointer(0),
		},
	})

	body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"task_wait"}`))
	require.NotNil(t, apiErr)
	require.Nil(t, body)
	require.Contains(t, apiErr.Error(), "image async task wait disabled")
}

func TestImageAsyncWaitStepsRoundsUp(t *testing.T) {
	require.Equal(t, 0, imageAsyncWaitSteps(0))
	require.Equal(t, 1, imageAsyncWaitSteps(1))
	require.Equal(t, 1, imageAsyncWaitSteps(2))
	require.Equal(t, 2, imageAsyncWaitSteps(3))
}

func TestImageResponseCaptureDoesNotWriteBodyOnIntermediateFlush(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := newImageResponseCapture(c.Writer)

	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)
	_, err := capture.Write([]byte(`{"id":"2c094bad-ecf3-c02f-90f7-cf0df7033c42"}`))
	require.NoError(t, err)
	capture.Flush()

	require.Empty(t, recorder.Body.String())

	capture.ReplaceBody(http.StatusOK, []byte(`{"created":1779125744,"data":[{"url":"https://cdn3.dmiapi.com/output.png","b64_json":"","revised_prompt":""}]}`))
	capture.WriteCaptured()

	require.JSONEq(t, `{
		"created":1779125744,
		"data":[{"url":"https://cdn3.dmiapi.com/output.png","b64_json":"","revised_prompt":""}]
	}`, recorder.Body.String())
	require.Equal(t, fmt.Sprintf("%d", recorder.Body.Len()), recorder.Header().Get("Content-Length"))
}

func TestImageTaskFetchReturnsOpenAICompatibleTaskPayload(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", r.URL.Path)
		require.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0","state":"succeeded","progress":0,"create_time":1779125744,"update_time":1779125807,"action":"generate","data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","file_name":"output.png"}]}}`))
	}))
	defer upstream.Close()

	baseURL := upstream.URL
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      11,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-upstream",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
		Name:    "async-image",
	}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:     "7154f43c-7765-4e77-d51e-0db4eee107b0",
		UserId:     7,
		ChannelId:  11,
		Action:     constant.TaskActionGenerate,
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		CreatedAt:  now,
		UpdatedAt:  now,
		SubmitTime: now,
		Platform:   constant.TaskPlatform("1"),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "7154f43c-7765-4e77-d51e-0db4eee107b0",
		},
	}).Error)

	router := gin.New()
	router.GET("/v1/tasks/:id", func(c *gin.Context) {
		c.Set("id", 7)
		if taskErr := ImageTaskFetch(c); taskErr != nil {
			c.JSON(taskErr.StatusCode, taskErr)
		}
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
		"state":"succeeded",
		"progress":0,
		"create_time":1779125744,
		"update_time":1779125807,
		"action":"generate",
		"data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","file_name":"output.png"}]}
	}`, recorder.Body.String())
}

func TestImageTaskFetchUsesBearerForStandardUpstreams(t *testing.T) {
	// Covered by the existing upstream server test; this placeholder keeps the
	// file focused on task payload behavior.
}

func TestConfigurableNativeFetchStoresFailureReason(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/tasks/upstream-task", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"task_id":"upstream-task","task_status":"FAILED","code":"InvalidParameter","message":"The parameter is invalid."},"request_id":"req-1"}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:       901,
		Type:     constant.ChannelTypeConfigurable,
		Key:      "sk-upstream",
		BaseURL:  common.GetPointer(upstream.URL),
		Status:   common.ChannelStatusEnabled,
		Name:     "happyhorse",
		Models:   "happyhorse-1.0-t2v",
		Group:    "default",
		Priority: common.GetPointer[int64](1),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "happyhorse-video",
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
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-task",
		},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_public", nil)
	c.Set("configurable_native_profile_id", "happyhorse-video")

	body := tryConfigurableNativeFetch(c, &model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: channel.Id,
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-task",
		},
	})
	require.True(t, strings.Contains(string(body), `"message":"The parameter is invalid."`), string(body))

	reloaded, exists, err := model.GetByTaskId(7, "task_public")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, "The parameter is invalid.", reloaded.FailReason)
}

func TestBuildImageTaskFetchRequestUsesDuomiTaskEndpointAndRawAuthorization(t *testing.T) {
	req, err := buildImageTaskFetchRequest("https://duomiapi.com", "duomi-key", "7154f43c-7765-4e77-d51e-0db4eee107b0")
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", req.URL.String())
	require.Equal(t, "duomi-key", req.Header.Get("Authorization"))
	require.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestBuildImageTaskFetchRequestUsesDuomiGeminiTaskEndpoint(t *testing.T) {
	req, err := buildImageTaskFetchRequest("https://duomiapi.com", "duomi-key", "7154f43c-7765-4e77-d51e-0db4eee107b0", "gemini-3-pro-image-preview")
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/api/gemini/nano-banana/7154f43c-7765-4e77-d51e-0db4eee107b0", req.URL.String())
	require.Equal(t, "duomi-key", req.Header.Get("Authorization"))
}

func TestConvertImageTaskResponseNormalizesDuomiGeminiPayload(t *testing.T) {
	task := &model.Task{
		TaskID:    "7154f43c-7765-4e77-d51e-0db4eee107b0",
		Action:    constant.TaskActionGenerate,
		CreatedAt: 1779125700,
		UpdatedAt: 1779125701,
		Properties: model.Properties{
			OriginModelName: "gemini-3-pro-image-preview",
		},
	}

	output, err := convertImageTaskResponse(task, []byte(`{
		"code":200,
		"msg":"success",
		"data":{
			"task_id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
			"state":"succeeded",
			"data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg","file_name":"output.jpeg"}],"description":"done"},
			"create_time":"1757061229",
			"update_time":"1757061252",
			"msg":"",
			"status":"3",
			"action":"generate"
		},
		"exec_time":0.05,
		"ip":"118.125.2.163"
	}`))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
		"state":"succeeded",
		"progress":0,
		"create_time":1757061229,
		"update_time":1757061252,
		"action":"generate",
		"data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg","file_name":"output.jpeg"}]}
	}`, string(output))
}
