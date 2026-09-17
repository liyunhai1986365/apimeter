package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestModelsellResponsesCompleteThroughFetchAndPoll(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"wrapped_different_inner_id", `{"code":"success","data":{"task_id":"task_modelsell","status":"SUCCESS","result_url":"https://cdn.example/result.mp4","data":{"task":{"id":"mvt-modelsell","status":"completed","usage":{"total_tokens":50638}}}}}`},
		{"flat_official", `{"id":"task_modelsell","status":"succeeded","content":{"video_url":"https://cdn.example/result.mp4"},"usage":{"total_tokens":40594}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			r := seedanceTestRouter(t, upstream.URL, "test", "doubao-seedance-2-0-260128")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.Type = constant.ChannelTypeConfigurable
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-modelsell"}})
			require.NoError(t, model.DB.Save(ch).Error)
			a := &configurable.TaskAdaptor{}
			a.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: 999, ChannelSetting: ch.GetSetting()}})
			parsed, err := a.ParseTaskResult([]byte(tc.body))
			require.NoError(t, err)
			require.Equal(t, string(model.TaskStatusSuccess), parsed.Status)
			require.Equal(t, "task_modelsell", parsed.TaskID)
			original := service.GetTaskAdaptorFunc
			service.GetTaskAdaptorFunc = func(_ constant.TaskPlatform) service.TaskPollingAdaptor { return &configurable.TaskAdaptor{} }
			t.Cleanup(func() { service.GetTaskAdaptorFunc = original })
			for _, mode := range []string{"fetch", "poll"} {
				t.Run(mode, func(t *testing.T) {
					task := &model.Task{TaskID: "task_local_" + mode, Platform: "999", UserId: 1, ChannelId: 20, Quota: 123, Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task_modelsell", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
					require.NoError(t, model.DB.Create(task).Error)
					if mode == "fetch" {
						w := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks/"+task.TaskID, "", 1)
						require.Equal(t, 200, w.Code, w.Body.String())
						require.Equal(t, "succeeded", gjson.Get(w.Body.String(), "status").String())
						require.Equal(t, int64(parsed.TotalTokens), gjson.Get(w.Body.String(), "usage.total_tokens").Int())
					} else {
						require.NoError(t, service.UpdateVideoTasks(context.Background(), "999", map[int][]string{20: {"task_modelsell"}}, map[string]*model.Task{"task_modelsell": task}))
					}
					var saved model.Task
					require.NoError(t, model.DB.First(&saved, task.ID).Error)
					require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
					require.JSONEq(t, tc.body, string(saved.Data))
					require.Equal(t, "https://cdn.example/result.mp4", saved.PrivateData.ResultURL)
				})
			}
		})
	}
}
