package relay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestSeedanceFetchRejectsNestedTaskMismatch(t *testing.T) {
	setupRelayTaskTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v2/video/tasks/cgt-owner", r.URL.Path)
		_, _ = w.Write([]byte(`{"task":{"id":"cgt-other","status":"succeeded","outputs":["https://other.example/video.mp4"]}}`))
	}))
	defer upstream.Close()
	ch := model.Channel{Id: 9105, Type: constant.ChannelTypeConfigurable, Key: "test", BaseURL: common.GetPointer(upstream.URL)}
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-service-inference"}})
	require.NoError(t, model.DB.Create(&ch).Error)
	task := &model.Task{TaskID: "task_review", UserId: 7, ChannelId: 9105, Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
	require.NoError(t, model.DB.Create(task).Error)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/api/v3/contents/generations/tasks/cgt-owner", nil)
	require.Empty(t, tryConfigurableFetch(c, task, true))
	_, rejected := c.Get("seedance_native_fetch_error")
	require.True(t, rejected)
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, task.ID).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), saved.Status)
	require.Empty(t, saved.GetResultURL())
}

func TestSeedanceFetchConcurrentWinnerAndPersistenceError(t *testing.T) {
	for _, scenario := range []string{"failed", "succeeded", "database_error"} {
		t.Run(scenario, func(t *testing.T) {
			setupRelayTaskTestDB(t)
			var task *model.Task
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if scenario == "database_error" {
					require.NoError(t, model.DB.Callback().Update().Before("gorm:update").Register("seedance_test_fail", func(db *gorm.DB) { db.AddError(fmt.Errorf("injected update failure")) }))
				} else {
					status := model.TaskStatusFailure
					if scenario == "succeeded" {
						status = model.TaskStatusSuccess
					}
					require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{"status": status, "data": []byte(fmt.Sprintf(`{"id":"cgt-owner","status":%q}`, scenario))}).Error)
				}
				responseStatus := "succeeded"
				if scenario == "succeeded" {
					responseStatus = "failed"
				}
				_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"cgt-owner","status":%q}`, responseStatus)))
			}))
			defer upstream.Close()
			defer model.DB.Callback().Update().Remove("seedance_test_fail")
			ch := model.Channel{Id: 9105, Type: constant.ChannelTypeVolcEngine, Key: "test", BaseURL: common.GetPointer(upstream.URL)}
			require.NoError(t, model.DB.Create(&ch).Error)
			task = &model.Task{TaskID: "task_review", UserId: 7, ChannelId: 9105, Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
			require.NoError(t, model.DB.Create(task).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/api/v3/contents/generations/tasks/cgt-owner", nil)
			body := tryConfigurableFetch(c, task, true)
			if scenario == "database_error" {
				require.Empty(t, body)
				value, exists := c.Get("seedance_native_fetch_error")
				require.True(t, exists)
				require.Equal(t, "task_persistence_failed", value.(*dto.TaskError).Code)
				var saved model.Task
				require.NoError(t, model.DB.First(&saved, task.ID).Error)
				require.Equal(t, model.TaskStatus(model.TaskStatusInProgress), saved.Status)
			} else {
				require.Equal(t, scenario, gjson.GetBytes(body, "status").String())
			}
		})
	}
}

func TestSeedanceUnchangedFetchRechecksConcurrentState(t *testing.T) {
	for _, winner := range []string{"failed", "succeeded", "running", "missing"} {
		t.Run(winner, func(t *testing.T) {
			setupRelayTaskTestDB(t)
			var task *model.Task
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch winner {
				case "failed", "succeeded":
					status := model.TaskStatusFailure
					if winner == "succeeded" {
						status = model.TaskStatusSuccess
					}
					require.NoError(t, model.DB.Model(&model.Task{}).Where("id = ?", task.ID).Updates(map[string]any{"status": status, "data": []byte(fmt.Sprintf(`{"id":"cgt-owner","status":%q}`, winner))}).Error)
				case "missing":
					require.NoError(t, model.DB.Delete(&model.Task{}, task.ID).Error)
				}
				_, _ = w.Write([]byte(`{"id":"cgt-owner","status":"running","future":{"preserve":true}}`))
			}))
			defer upstream.Close()
			ch := model.Channel{Id: 9105, Type: constant.ChannelTypeVolcEngine, Key: "test", BaseURL: common.GetPointer(upstream.URL)}
			require.NoError(t, model.DB.Create(&ch).Error)
			task = &model.Task{TaskID: "task_unchanged", UserId: 7, ChannelId: ch.Id, Status: model.TaskStatusInProgress, Progress: "50%", PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner"}}
			require.NoError(t, model.DB.Create(task).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("GET", "/api/v3/contents/generations/tasks/cgt-owner", nil)
			body := tryConfigurableFetch(c, task, true)
			if winner == "missing" {
				require.Empty(t, body)
				value, ok := c.Get("seedance_native_fetch_error")
				require.True(t, ok)
				require.Equal(t, "get_task_failed", value.(*dto.TaskError).Code)
			} else {
				require.Equal(t, winner, gjson.GetBytes(body, "status").String())
				if winner == "running" {
					require.True(t, gjson.GetBytes(body, "future.preserve").Bool())
				}
			}
		})
	}
}
