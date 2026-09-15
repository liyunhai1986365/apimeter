package relay

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSeedanceGenericQueryGuards(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     model.TaskStatus
	}{
		{"wrong_id", `{"id":"task-other","status":"succeeded","content":{"video_url":"https://other.example/video.mp4"}}`, model.TaskStatusInProgress},
		{"terminal_regression", `{"id":"task-owner","status":"running"}`, model.TaskStatusSuccess},
		{"missing_status", `{"id":"task-owner"}`, model.TaskStatusInProgress},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupRelayTaskTestDB(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer upstream.Close()
			ch := model.Channel{Id: 9105, Type: constant.ChannelTypeConfigurable, Key: "test", BaseURL: common.GetPointer(upstream.URL)}
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
			require.NoError(t, model.DB.Create(&ch).Error)
			task := &model.Task{TaskID: "task_local", UserId: 7, ChannelId: 9105, Status: tc.status, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task-owner", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
			require.NoError(t, model.DB.Create(task).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/v1/videos/task_local", nil)
			tryConfigurableFetch(c, task, false)
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			t.Logf("status before=%s after=%s result_url=%s", tc.status, saved.Status, saved.PrivateData.ResultURL)
			require.Equal(t, tc.status, saved.Status)
			require.Empty(t, saved.PrivateData.ResultURL)
		})
	}
}

func TestSeedanceQueryEntrypointsPreserveIntegrityAndFormat(t *testing.T) {
	for _, path := range []string{"/v1/videos/task_local", "/v1/video/generations/task_local", "/api/v3/contents/generations/tasks/cgt-owner"} {
		t.Run(path, func(t *testing.T) {
			for _, tc := range []struct {
				name, body                string
				initial                   model.TaskStatus
				upstreamStatus, errStatus int
			}{
				{"wrong_id", `{"id":"task-other","status":"succeeded"}`, model.TaskStatusInProgress, 200, 502},
				{"missing_status", `{"id":"task-owner"}`, model.TaskStatusInProgress, 200, 502},
				{"unknown_status", `{"id":"task-owner","status":"unexpected"}`, model.TaskStatusInProgress, 200, 502},
				{"only_ids", `{"id":"task-owner","upstream_task_id":"cgt-owner"}`, model.TaskStatusInProgress, 200, 503},
				{"throttled", `{"error":{"code":"Throttled"}}`, model.TaskStatusInProgress, 429, 429},
				{"unavailable", `{"error":{"code":"Unavailable"}}`, model.TaskStatusInProgress, 503, 503},
				{"terminal_stale", `{"id":"task-owner","status":"running"}`, model.TaskStatusSuccess, 200, 0},
				{"terminal_failure", `{"id":"task-owner","status":"failed"}`, model.TaskStatusSuccess, 200, 0},
				{"terminal_unavailable", `{"error":{"code":"Unavailable"}}`, model.TaskStatusSuccess, 503, 0},
			} {
				t.Run(tc.name, func(t *testing.T) {
					setupRelayTaskTestDB(t)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Retry-After", "2")
						w.WriteHeader(tc.upstreamStatus)
						_, _ = w.Write([]byte(tc.body))
					}))
					defer upstream.Close()
					ch := model.Channel{Id: 9105, Type: constant.ChannelTypeConfigurable, Key: "test", BaseURL: common.GetPointer(upstream.URL)}
					ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
					require.NoError(t, model.DB.Create(&ch).Error)
					task := &model.Task{TaskID: "task_local", UserId: 7, ChannelId: 9105, Platform: "999", Status: tc.initial, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task-owner", OfficialTaskID: "cgt-owner", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
					if tc.initial == model.TaskStatusSuccess {
						task.Data = []byte(`{"id":"task-owner","status":"succeeded","content":{"video_url":"https://owner.example/video.mp4"}}`)
					}
					require.NoError(t, model.DB.Create(task).Error)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("GET", path, nil)
					c.Set("id", 7)
					c.Set("task_id", "task_local")
					if isVolcengineVideoTaskQueryRequest(c) {
						c.Set("task_id", "cgt-owner")
					}
					body, taskErr := videoFetchByIDRespBodyBuilder(c)
					if tc.errStatus != 0 {
						require.NotNil(t, taskErr)
						require.Equal(t, tc.errStatus, taskErr.StatusCode)
						require.Empty(t, body)
					} else {
						require.Nil(t, taskErr)
						require.NotEmpty(t, body)
						switch path {
						case "/v1/videos/task_local":
							require.Equal(t, "task_local", gjson.GetBytes(body, "id").String())
							require.Equal(t, "completed", gjson.GetBytes(body, "status").String())
						case "/v1/video/generations/task_local":
							require.Equal(t, "task_local", gjson.GetBytes(body, "data.task_id").String())
							require.Equal(t, "SUCCESS", gjson.GetBytes(body, "data.status").String())
						default:
							require.Equal(t, "cgt-owner", gjson.GetBytes(body, "id").String())
							require.Equal(t, "succeeded", gjson.GetBytes(body, "status").String())
						}
					}
					var saved model.Task
					require.NoError(t, model.DB.First(&saved, task.ID).Error)
					require.Equal(t, tc.initial, saved.Status)
					require.Equal(t, string(task.Data), string(saved.Data))
					require.Empty(t, saved.PrivateData.ResultURL)
				})
			}
		})
	}
}

func TestSeedanceGenericQueryConcurrentWinnerAndWriteFailure(t *testing.T) {
	for _, path := range []string{"/v1/videos/task_local", "/v1/video/generations/task_local"} {
		t.Run(path, func(t *testing.T) {
			for _, scenario := range []string{"winner_failed", "winner_succeeded", "unchanged_winner_failed", "database_error"} {
				t.Run(scenario, func(t *testing.T) {
					setupRelayTaskTestDB(t)
					var task *model.Task
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						responseStatus := "succeeded"
						if scenario == "database_error" {
							require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("seedance_generic_write_fail", func(db *gorm.DB) { db.AddError(fmt.Errorf("injected failure")) }))
						} else {
							status := model.TaskStatusFailure
							rawStatus := "failed"
							if scenario == "winner_succeeded" {
								status = model.TaskStatusSuccess
								rawStatus = "succeeded"
								responseStatus = "failed"
							}
							if scenario == "unchanged_winner_failed" {
								responseStatus = "running"
							}
							require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{"status": status, "data": []byte(fmt.Sprintf(`{"id":"task-owner","status":%q}`, rawStatus))}).Error)
						}
						_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"task-owner","status":%q}`, responseStatus)))
					}))
					defer upstream.Close()
					defer model.DB.Callback().Update().Remove("seedance_generic_write_fail")
					ch := model.Channel{Id: 9105, Type: constant.ChannelTypeConfigurable, Key: "test", BaseURL: common.GetPointer(upstream.URL)}
					ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
					require.NoError(t, model.DB.Create(&ch).Error)
					task = &model.Task{TaskID: "task_local", UserId: 7, ChannelId: 9105, Platform: "999", Status: model.TaskStatusInProgress, Progress: "50%", Quota: 200, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task-owner", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
					require.NoError(t, model.DB.Create(task).Error)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("GET", path, nil)
					c.Set("id", 7)
					c.Set("task_id", "task_local")
					body, taskErr := videoFetchByIDRespBodyBuilder(c)
					var saved model.Task
					require.NoError(t, model.DB.First(&saved, task.ID).Error)
					require.Equal(t, 200, saved.Quota, "losing query must not settle or refund")
					if scenario == "database_error" {
						require.NotNil(t, taskErr)
						require.Equal(t, "task_persistence_failed", taskErr.Code)
						require.Empty(t, body)
						require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), saved.Status)
					} else {
						require.Nil(t, taskErr)
						expected := model.TaskStatusFailure
						if scenario == "winner_succeeded" {
							expected = model.TaskStatusSuccess
						}
						require.Equal(t, model.TaskStatus(expected), saved.Status)
						if path == "/v1/videos/task_local" {
							require.Equal(t, "task_local", gjson.GetBytes(body, "id").String())
							public := "failed"
							if expected == model.TaskStatusSuccess {
								public = "completed"
							}
							require.Equal(t, public, gjson.GetBytes(body, "status").String())
						} else {
							require.Equal(t, expected, gjson.GetBytes(body, "data.status").String())
						}
					}
				})
			}
		})
	}
}
