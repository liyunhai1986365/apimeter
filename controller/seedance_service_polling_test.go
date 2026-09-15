package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
)

// Exercise the service entry used by the production poller, with a real profile
// adaptor and HTTP upstream, rather than the legacy controller polling helper.
func TestSeedanceServicePollingIdentityAndResponseGuards(t *testing.T) {
	for _, tc := range []struct {
		name, profile, body string
		httpStatus          int
		expected            model.TaskStatus
	}{
		{"nested_wrong_id", "seedance2-service-inference", `{"task":{"id":"cgt-other","status":"succeeded","outputs":["https://other.example/video.mp4"]}}`, 200, model.TaskStatusInProgress},
		{"data_wrong_id", "seedance2-modelsell", `{"data":{"task_id":"cgt-other","status":"succeeded"}}`, 200, model.TaskStatusInProgress},
		{"flat_wrong_id", "seedance-tgxmaas", `{"id":"cgt-other","status":"succeeded"}`, 200, model.TaskStatusInProgress},
		{"official_wrong_id", "seedance-tgxmaas", `{"id":"cgt-owner","upstream_task_id":"cgt-other","status":"succeeded"}`, 200, model.TaskStatusInProgress},
		{"unavailable", "seedance-tgxmaas", `{"id":"cgt-owner","status":"failed"}`, 503, model.TaskStatusInProgress},
		{"invalid_json", "seedance-tgxmaas", `not json`, 200, model.TaskStatusInProgress},
		{"missing_status", "seedance-tgxmaas", `{"id":"cgt-owner","upstream_task_id":"cgt-official"}`, 200, model.TaskStatusInProgress},
		{"unknown_status", "seedance-tgxmaas", `{"id":"cgt-owner","status":"unexpected"}`, 200, model.TaskStatusInProgress},
		{"success", "seedance-tgxmaas", `{"id":"cgt-owner","status":"succeeded","content":{"video_url":"https://example.com/video.mp4"},"usage":{"total_tokens":9007199254740993,"extra":0}}`, 200, model.TaskStatusSuccess},
		{"failure", "seedance-tgxmaas", `{"id":"cgt-owner","status":"failed","error":{"message":"failed"}}`, 200, model.TaskStatusFailure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				require.Equal(t, "Bearer test", r.Header.Get("Authorization"))
				w.WriteHeader(tc.httpStatus)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			seedanceTestRouter(t, upstream.URL, "test", "doubao-seedance-2-0-260128")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.Type = constant.ChannelTypeConfigurable
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: tc.profile}})
			require.NoError(t, model.DB.Save(ch).Error)
			original := service.GetTaskAdaptorFunc
			service.GetTaskAdaptorFunc = func(_ constant.TaskPlatform) service.TaskPollingAdaptor { return &configurable.TaskAdaptor{} }
			t.Cleanup(func() { service.GetTaskAdaptorFunc = original })
			task := &model.Task{TaskID: "task_service_poll", UserId: 1, ChannelId: 20, Quota: 123, Status: model.TaskStatusInProgress, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-owner", OfficialTaskID: "cgt-official", BillingContext: &model.TaskBillingContext{PerCallBilling: true}}}
			require.NoError(t, model.DB.Create(task).Error)
			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			poll := func(task *model.Task) {
				require.NoError(t, service.UpdateVideoTasks(context.Background(), constant.TaskPlatform("999"), map[int][]string{20: {"cgt-owner"}}, map[string]*model.Task{"cgt-owner": task}))
			}
			poll(task)
			require.EqualValues(t, 1, requests.Load(), "must reach the actual service HTTP polling path")
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.Equal(t, tc.expected, saved.Status)
			if tc.expected == model.TaskStatusInProgress {
				require.Empty(t, saved.Data)
				require.Empty(t, saved.PrivateData.ResultURL)
				require.Empty(t, task.Data)
				require.Equal(t, 123, saved.Quota)
			} else {
				require.Equal(t, tc.body, string(saved.Data), "preserve full upstream response without numeric rounding")
				poll(&saved)
				require.EqualValues(t, 1, requests.Load(), "persisted terminal task must not be polled or billed again")
			}
			require.NoError(t, model.DB.First(&after, 1).Error)
			expectedQuota := before.Quota
			if tc.expected == model.TaskStatusFailure {
				expectedQuota += 123
			}
			require.Equal(t, expectedQuota, after.Quota)
		})
	}
}
