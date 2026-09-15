package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func seedancePollingFixture(t *testing.T, configurableProfile bool, timeout int) {
	t.Helper()
	oldAdaptor, oldLimit, oldTimeout := service.GetTaskAdaptorFunc, constant.TaskQueryLimit, constant.TaskTimeoutMinutes
	service.GetTaskAdaptorFunc = func(_ constant.TaskPlatform) service.TaskPollingAdaptor {
		if configurableProfile {
			return &configurable.TaskAdaptor{}
		}
		return &doubao.TaskAdaptor{}
	}
	constant.TaskQueryLimit = 100
	constant.TaskTimeoutMinutes = timeout
	t.Cleanup(func() {
		service.GetTaskAdaptorFunc = oldAdaptor
		constant.TaskQueryLimit = oldLimit
		constant.TaskTimeoutMinutes = oldTimeout
	})
}

func TestSeedanceMultiKeyTaskPinsCredential(t *testing.T) {
	for _, profile := range []bool{false, true} {
		name := "direct"
		if profile {
			name = "configurable"
		}
		t.Run(name, func(t *testing.T) {
			var selected atomic.Value
			var gets atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					require.Contains(t, []string{"Bearer test-a", "Bearer test-b"}, r.Header.Get("Authorization"))
					selected.Store(r.Header.Get("Authorization"))
					_, _ = w.Write([]byte(`{"id":"cgt-key"}`))
				} else {
					require.Equal(t, selected.Load(), r.Header.Get("Authorization"))
					gets.Add(1)
					_, _ = w.Write([]byte(`{"id":"cgt-key","status":"running"}`))
				}
			}))
			defer upstream.Close()
			r := seedanceTestRouter(t, upstream.URL, "test-a\ntest-b", "doubao-seedance-2-0-260128")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.ChannelInfo.IsMultiKey = true
			ch.ChannelInfo.MultiKeyMode = constant.MultiKeyModeRandom
			if profile {
				ch.Type = constant.ChannelTypeConfigurable
				ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
			}
			require.NoError(t, model.DB.Save(ch).Error)
			created := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"doubao-seedance-2-0-260128","content":[{"type":"text","text":"test"}]}`, 1)
			require.Equal(t, 200, created.Code, created.Body.String())
			task, exists, err := model.GetByTaskIDOrUpstreamID(1, "cgt-key")
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, selected.Load(), "Bearer "+task.PrivateData.Key)
			// Even changing the channel key cannot redirect an existing task's account.
			require.NoError(t, model.DB.Model(ch).Update("key", "replacement-key").Error)
			queried := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/cgt-key", "", 1)
			require.Equal(t, 200, queried.Code, queried.Body.String())
			seedancePollingFixture(t, profile, 0)
			service.RunTaskPollingOnce(context.Background(), func(int, int) {})
			require.EqualValues(t, 2, gets.Load(), "foreground and real background poll must use the saved key")
		})
	}
}

func TestSeedanceDirectRejectsMalformedTaskResponses(t *testing.T) {
	for _, body := range []string{
		`{"status":"succeeded","content":{"video_url":"https://unverified.example/video.mp4"}}`,
		`{"id":null,"status":"succeeded"}`, `{"id":123,"status":"succeeded"}`, `{"id":"","status":"succeeded"}`,
		`{"id":"cgt-invalid","status":null}`, `{"id":"cgt-invalid","status":""}`, `{"id":"cgt-invalid","status":123}`, `{"id":"cgt-invalid","status":"unexpected"}`,
	} {
		t.Run(body, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer upstream.Close()
			r := seedanceTestRouter(t, upstream.URL, "test", "doubao-seedance-2-0-260128")
			task := &model.Task{TaskID: "task_invalid", UserId: 1, ChannelId: 20, Platform: "45", Status: model.TaskStatusQueued, Progress: "10%", Quota: 123, SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-invalid"}}
			require.NoError(t, model.DB.Create(task).Error)
			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			response := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/cgt-invalid", "", 1)
			require.Equal(t, 502, response.Code, response.Body.String())
			seedancePollingFixture(t, false, 0)
			service.RunTaskPollingOnce(context.Background(), func(int, int) {})
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.Equal(t, model.TaskStatus(model.TaskStatusQueued), saved.Status)
			require.Empty(t, saved.Data)
			require.Empty(t, saved.PrivateData.ResultURL)
			require.Equal(t, 123, saved.Quota)
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.Equal(t, before.Quota, after.Quota)
		})
	}
}

func TestSeedancePollingSeparatesChannelTaskIDs(t *testing.T) {
	var upstreamCalls atomic.Int32
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		require.Equal(t, "Bearer test-a", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":"task-vendor-shared","status":"succeeded","content":{"video_url":"https://account-a.example/video.mp4"}}`))
	}))
	defer a.Close()
	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		require.Equal(t, "Bearer test-b", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":"task-vendor-shared","status":"running"}`))
	}))
	defer b.Close()
	seedanceTestRouter(t, a.URL, "test-a", "doubao-seedance-2-0-260128")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.Type = constant.ChannelTypeConfigurable
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
	require.NoError(t, model.DB.Save(ch).Error)
	second := *ch
	second.Id = 21
	second.Key = "test-b"
	second.BaseURL = common.GetPointer(b.URL)
	require.NoError(t, model.DB.Create(&second).Error)
	seedancePollingFixture(t, true, 0)
	for i := 0; i < 2; i++ {
		require.NoError(t, model.DB.Create(&model.Task{TaskID: []string{"task_local_a", "task_local_b"}[i], UserId: i + 1, ChannelId: 20 + i, Platform: "999", Status: model.TaskStatusQueued, SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: "task-vendor-shared", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}).Error)
	}
	service.RunTaskPollingOnce(context.Background(), func(int, int) {})
	var tasks []model.Task
	require.NoError(t, model.DB.Order("id").Find(&tasks).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), tasks[0].Status)
	require.Equal(t, "https://account-a.example/video.mp4", tasks[0].PrivateData.ResultURL)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), tasks[1].Status)
	require.Empty(t, tasks[1].PrivateData.ResultURL)
	// An incorrectly assembled caller must also be stopped before any HTTP call.
	beforeCalls := upstreamCalls.Load()
	require.NoError(t, service.UpdateVideoTasks(context.Background(), "999", map[int][]string{20: {"task-vendor-shared"}}, map[string]*model.Task{"task-vendor-shared": &tasks[1]}))
	require.Equal(t, beforeCalls, upstreamCalls.Load())
}

func TestSeedanceLocalTimeoutOverridesRunningCache(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"cgt-timeout","status":"running"}`))
	}))
	defer upstream.Close()
	r := seedanceTestRouter(t, upstream.URL, "test", "doubao-seedance-2-0-260128")
	seedancePollingFixture(t, false, 1)
	task := &model.Task{TaskID: "task_timeout", UserId: 1, ChannelId: 20, Platform: "45", Status: model.TaskStatusQueued, SubmitTime: time.Now().Unix() - 120, Quota: 123, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-timeout"}}
	require.NoError(t, model.DB.Create(task).Error)
	var before, after model.User
	require.NoError(t, model.DB.First(&before, 1).Error)
	service.RunTaskPollingOnce(context.Background(), func(int, int) {})
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusFailure), saved.Status)
	require.Zero(t, saved.Quota)
	for i := 0; i < 2; i++ {
		response := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/cgt-timeout", "", 1)
		require.Equal(t, 200, response.Code)
		require.Equal(t, "failed", gjson.GetBytes(response.Body.Bytes(), "status").String())
		require.Equal(t, saved.FailReason, gjson.GetBytes(response.Body.Bytes(), "error.message").String())
	}
	require.NoError(t, model.DB.First(&after, 1).Error)
	require.Equal(t, before.Quota+123, after.Quota)
}

func TestSeedancePollingSeparatesSameChannelAccountIDs(t *testing.T) {
	var callsA, callsB atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer account-a":
			callsA.Add(1)
			_, _ = w.Write([]byte(`{"id":"task-shared","status":"succeeded","content":{"video_url":"https://a.example/video.mp4"}}`))
		case "Bearer account-b":
			callsB.Add(1)
			_, _ = w.Write([]byte(`{"id":"task-shared","status":"running"}`))
		default:
			t.Error("poller did not use the saved task credential")
			w.WriteHeader(401)
		}
	}))
	defer upstream.Close()
	tgxMaasResourceTestRouter(t, upstream.URL, "channel-key", tgxRegressionModel)
	seedancePollingFixture(t, true, 0)
	for i, key := range []string{"account-a", "account-b"} {
		require.NoError(t, model.DB.Create(&model.Task{TaskID: []string{"task_a", "task_b"}[i], UserId: i + 1, ChannelId: 20, Platform: "999", Status: model.TaskStatusQueued, SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{Key: key, UpstreamTaskID: "task-shared", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}).Error)
	}
	service.RunTaskPollingOnce(context.Background(), func(int, int) {})
	var tasks []model.Task
	require.NoError(t, model.DB.Order("id").Find(&tasks).Error)
	require.EqualValues(t, 1, callsA.Load())
	require.EqualValues(t, 1, callsB.Load())
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), tasks[0].Status)
	require.Equal(t, "https://a.example/video.mp4", tasks[0].PrivateData.ResultURL)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), tasks[1].Status)
	require.Empty(t, tasks[1].PrivateData.ResultURL)
}

func TestSeedanceConfiguredStatusIsAuthoritative(t *testing.T) {
	for _, rawStatus := range []string{`"failed"`, `"unexpected"`, `null`, `""`, `123`, "missing"} {
		t.Run(rawStatus, func(t *testing.T) {
			body := `{"status":"success","task":{"id":"cgt-owner","outputs":["https://example.com/video.mp4"]}}`
			if rawStatus != "missing" {
				body = `{"status":"success","task":{"id":"cgt-owner","status":` + rawStatus + `,"outputs":["https://example.com/video.mp4"]}}`
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer upstream.Close()
			r := seedanceTestRouter(t, upstream.URL, "test", "doubao-seedance-2-0-260128")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.Type = constant.ChannelTypeConfigurable
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"}})
			require.NoError(t, model.DB.Save(ch).Error)
			task := &model.Task{TaskID: "task_nested_status", UserId: 1, ChannelId: 20, Platform: "999", Status: model.TaskStatusQueued, SubmitTime: time.Now().Unix(), PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
			require.NoError(t, model.DB.Create(task).Error)
			response := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/cgt-owner", "", 1)
			seedancePollingFixture(t, true, 0)
			service.RunTaskPollingOnce(context.Background(), func(int, int) {})
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			if rawStatus == `"failed"` {
				require.Equal(t, 200, response.Code, response.Body.String())
				require.Equal(t, "failed", gjson.GetBytes(response.Body.Bytes(), "status").String())
				require.Equal(t, model.TaskStatus(model.TaskStatusFailure), saved.Status)
			} else {
				require.Equal(t, 502, response.Code, response.Body.String())
				require.Equal(t, model.TaskStatus(model.TaskStatusQueued), saved.Status)
				require.Empty(t, saved.Data)
			}
		})
	}
}
