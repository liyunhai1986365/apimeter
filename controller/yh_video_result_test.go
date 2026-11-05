package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHRejectsMalformedResultBeforePersistence(t *testing.T) {
	r := yhAssetTestRouter(t, "https://yh-result.invalid")
	r.GET("/v1/videos/:task_id", middleware.TokenAuth(), middleware.DistributeVideoTaskFetch(), RelayTaskFetch)
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	var response string
	calls := 0
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return yhHTTPResponse(req, 200, []byte(response), nil), nil
	})
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	adaptor := &configurable.TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: ch.Type, ChannelBaseUrl: ch.GetBaseURL(), ApiKey: ch.Key, ChannelSetting: ch.GetSetting()}})
	oldAdaptor := service.GetTaskAdaptorFunc
	service.GetTaskAdaptorFunc = func(_ constant.TaskPlatform) service.TaskPollingAdaptor { return &configurable.TaskAdaptor{} }
	t.Cleanup(func() { service.GetTaskAdaptorFunc = oldAdaptor })
	task := &model.Task{TaskID: "task_malformed", UserId: 1, ChannelId: 20, Status: model.TaskStatusInProgress, Quota: 123,
		Properties:  model.Properties{OriginModelName: tgxRegressionModel},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-malformed", BillingContext: &model.TaskBillingContext{PerCallBilling: true}},
		Data:        []byte(`{"task_id":"cgt-malformed","status":"SUBMITTED"}`),
	}
	require.NoError(t, model.DB.Create(task).Error)
	for _, malformed := range []string{`"invalid-result"`, `[]`, `true`, `123`} {
		response = fmt.Sprintf(`{"code":"success","internal_marker":"private","data":{"task_id":"cgt-malformed","status":"SUCCESS","data":%s}}`, malformed)
		for _, entry := range []string{"native", "generic", "service_poll", "controller_poll"} {
			t.Run(malformed+"/"+entry, func(t *testing.T) {
				beforeCalls := calls
				var before, after model.User
				require.NoError(t, model.DB.First(&before, 1).Error)
				switch entry {
				case "native", "generic":
					path := "/api/v3/contents/generations/tasks/cgt-malformed"
					if entry == "generic" {
						path = "/v1/videos/task_malformed"
					}
					out := seedanceCall(r, http.MethodGet, path, "", 1)
					require.Equal(t, http.StatusBadGateway, out.Code, out.Body.String())
					require.NotContains(t, out.Body.String(), "internal_marker")
				case "service_poll":
					require.NoError(t, service.UpdateVideoTasks(context.Background(), constant.TaskPlatform("999"), map[int][]string{20: {"cgt-malformed"}}, map[string]*model.Task{"cgt-malformed": task}))
				case "controller_poll":
					require.Error(t, updateVideoSingleTask(context.Background(), adaptor, ch, "cgt-malformed", map[string]*model.Task{"cgt-malformed": task}))
				}
				var saved model.Task
				require.NoError(t, model.DB.First(&saved, task.ID).Error)
				require.Equal(t, beforeCalls+1, calls, "every entry must fetch and validate the mocked response")
				require.EqualValues(t, model.TaskStatusInProgress, saved.Status)
				require.JSONEq(t, `{"task_id":"cgt-malformed","status":"SUBMITTED"}`, string(saved.Data))
				require.Empty(t, saved.PrivateData.ResultURL)
				require.Zero(t, saved.FinishTime)
				require.Equal(t, 123, saved.Quota)
				require.NoError(t, model.DB.First(&after, 1).Error)
				require.Equal(t, before.Quota, after.Quota)
				listed := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks", "", 1)
				require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
			})
		}
	}
	// A record saved before this fix cannot break listing or expose its raw body.
	task.Status, task.Data = model.TaskStatusSuccess, []byte(response)
	require.NoError(t, model.DB.Save(task).Error)
	listed := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks", "", 1)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.EqualValues(t, 1, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
	require.NotContains(t, listed.Body.String(), "internal_marker")
	require.False(t, gjson.GetBytes(listed.Body.Bytes(), "items.0.content.video_url").Exists())
}

func TestYHServiceTierPersistsFromSubmission(t *testing.T) {
	for _, entry := range []string{"native", "generic"} {
		t.Run(entry, func(t *testing.T) {
			r := yhAssetTestRouter(t, "https://yh-tier.invalid")
			r.POST("/v1/videos", middleware.TokenAuth(), middleware.Distribute(), RelayTask)
			client := service.GetHttpClient()
			original := client.Transport
			t.Cleanup(func() { client.Transport = original })
			client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
				return yhHTTPResponse(req, 200, []byte(`{"task_id":"cgt-tier","status":"SUBMITTED"}`), nil), nil
			})
			path, body := "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"test"}],"service_tier":"flex"}`
			if entry == "generic" {
				path, body = "/v1/videos", `{"model":"`+tgxRegressionModel+`","prompt":"test","metadata":{"service_tier":"flex"}}`
			}
			created := seedanceCall(r, http.MethodPost, path, body, 1)
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())
			task, exists, err := model.GetByTaskIDOrUpstreamID(1, "cgt-tier")
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, "flex", task.PrivateData.YHServiceTier)
			for _, status := range []model.TaskStatus{model.TaskStatusSubmitted, model.TaskStatusInProgress, model.TaskStatusSuccess} {
				task.Status = status
				task.Data, err = common.Marshal(map[string]any{"data": map[string]any{"task_id": "cgt-tier", "status": status, "data": map[string]any{}}})
				require.NoError(t, err)
				require.NoError(t, model.DB.Save(task).Error)
				for _, tier := range []string{"flex", "default"} {
					listed := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks?filter.service_tier="+tier, "", 1)
					require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
					want := int64(0)
					if tier == "flex" {
						want = 1
					}
					require.Equal(t, want, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
				}
			}
		})
	}
}

func TestYHListMatchesForegroundStatus(t *testing.T) {
	for _, entry := range []string{"native", "generic"} {
		t.Run(entry, func(t *testing.T) {
			r := yhAssetTestRouter(t, "https://yh-result.invalid")
			r.GET("/v1/videos/:task_id", middleware.TokenAuth(), middleware.DistributeVideoTaskFetch(), RelayTaskFetch)
			client := service.GetHttpClient()
			original := client.Transport
			t.Cleanup(func() { client.Transport = original })
			calls := 0
			client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				return yhHTTPResponse(req, 200, []byte(`{"data":{"task_id":"cgt-status","status":"IN_PROGRESS","data":null}}`), nil), nil
			})
			task := &model.Task{TaskID: "task_status", UserId: 1, ChannelId: 20, Platform: "999", Status: model.TaskStatusSubmitted,
				Properties:  model.Properties{OriginModelName: tgxRegressionModel},
				PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-status", YHServiceTier: "flex"},
				Data:        []byte(`{"task_id":"cgt-status","status":"SUBMITTED"}`),
			}
			require.NoError(t, model.DB.Create(task).Error)
			path := "/api/v3/contents/generations/tasks/cgt-status"
			if entry == "generic" {
				path = "/v1/videos/task_status"
			}
			fetched := seedanceCall(r, http.MethodGet, path, "", 1)
			require.Equal(t, http.StatusOK, fetched.Code, fetched.Body.String())
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			require.EqualValues(t, model.TaskStatusInProgress, saved.Status)
			require.JSONEq(t, string(task.Data), string(saved.Data), "nonterminal polling keeps the older payload")
			for _, filter := range []string{"", "?filter.status=running", "?filter.status=running&filter.service_tier=flex"} {
				listed := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks"+filter, "", 1)
				require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
				require.EqualValues(t, 1, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
				require.Equal(t, "running", gjson.GetBytes(listed.Body.Bytes(), "items.0.status").String())
				require.Equal(t, "flex", gjson.GetBytes(listed.Body.Bytes(), "items.0.service_tier").String())
			}
			queued := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks?filter.status=queued", "", 1)
			require.Equal(t, http.StatusOK, queued.Code, queued.Body.String())
			require.Zero(t, gjson.GetBytes(queued.Body.Bytes(), "total").Int())
			require.Equal(t, 1, calls, "local listing must not query the supplier")
		})
	}
}

func TestYHListUsesLocalTimeoutAndPreservesCompletedResults(t *testing.T) {
	r := yhAssetTestRouter(t, "https://yh-result.invalid")
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		t.Error("local listing must not query the supplier")
		return nil, fmt.Errorf("unexpected supplier call")
	})
	for index, data := range []string{
		`{"task_id":"cgt-timeout","status":"SUBMITTED"}`,
		`{"data":{"task_id":"cgt-timeout","status":"IN_PROGRESS","data":null}}`,
	} {
		task := &model.Task{TaskID: fmt.Sprintf("task_timeout_%d", index), UserId: 1, ChannelId: 20, Status: model.TaskStatusFailure, FailReason: "task timeout", FinishTime: 1,
			Properties: model.Properties{OriginModelName: tgxRegressionModel}, Data: []byte(data),
		}
		require.NoError(t, model.DB.Create(task).Error)
	}
	result := `{"content":{"video_url":"https://example.com/result.mp4","last_frame_url":"https://example.com/frame.png","extra":false},"usage":{"total_tokens":10,"extra":9007199254740993},"seed":9007199254740993,"duration":5,"service_tier":"flex"}`
	task := &model.Task{TaskID: "task_success", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess,
		Properties:  model.Properties{OriginModelName: tgxRegressionModel},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-success"},
		Data:        []byte(`{"data":{"task_id":"cgt-success","status":"SUCCESS","data":` + result + `}}`),
	}
	require.NoError(t, model.DB.Create(task).Error)
	for page := 1; page <= 2; page++ {
		listed := seedanceCall(r, http.MethodGet, fmt.Sprintf("/api/v3/contents/generations/tasks?filter.status=failed&page_size=1&page_num=%d", page), "", 1)
		require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
		require.EqualValues(t, 2, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
		require.Len(t, gjson.GetBytes(listed.Body.Bytes(), "items").Array(), 1)
		require.Equal(t, "failed", gjson.GetBytes(listed.Body.Bytes(), "items.0.status").String())
		require.Equal(t, "task timeout", gjson.GetBytes(listed.Body.Bytes(), "items.0.failure_reason.message").String())
		require.False(t, gjson.GetBytes(listed.Body.Bytes(), "items.0.content").Exists())
	}
	listed := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks?filter.status=succeeded&filter.service_tier=flex", "", 1)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.EqualValues(t, 1, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
	for _, field := range []string{"content", "usage", "seed", "duration", "service_tier"} {
		require.Equal(t, gjson.Get(result, field).Raw, gjson.GetBytes(listed.Body.Bytes(), "items.0."+field).Raw, field)
	}
}

func TestYHGenericDurationReachesUpstream(t *testing.T) {
	r := yhAssetTestRouter(t, "https://yh-duration.invalid")
	r.POST("/v1/videos", middleware.TokenAuth(), middleware.Distribute(), RelayTask)
	client := service.GetHttpClient()
	original := client.Transport
	t.Cleanup(func() { client.Transport = original })
	var sent []byte
	calls := 0
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		require.Equal(t, "/v1/video/generations", req.URL.Path)
		var err error
		sent, err = io.ReadAll(req.Body)
		require.NoError(t, err)
		return yhHTTPResponse(req, 200, []byte(fmt.Sprintf(`{"task_id":"cgt-duration-%d","status":"SUBMITTED"}`, calls)), nil), nil
	})
	for _, tc := range []struct{ fields, want string }{
		{`"duration":0`, "0"},
		{`"Duration":0,"metadata":{"duration":12}`, "0"},
		{`"Seconds":"5","omni_reference_task_type":"auto","metadata":{"duration":12}`, "5"},
	} {
		before := calls
		created := seedanceCall(r, http.MethodPost, "/v1/videos", `{"model":"`+tgxRegressionModel+`","prompt":"test",`+tc.fields+`}`, 1)
		require.Equal(t, http.StatusOK, created.Code, created.Body.String())
		require.Equal(t, before+1, calls)
		require.Equal(t, tc.want, gjson.GetBytes(sent, "metadata.duration").Raw)
	}
}

func TestYHListPreservesLegacyFailureStatuses(t *testing.T) {
	r := yhAssetTestRouter(t, "https://yh-result.invalid")
	for _, status := range []string{"cancelled", "expired"} {
		task := &model.Task{TaskID: "task_" + status, UserId: 1, ChannelId: 20, Status: model.TaskStatusFailure,
			Properties: model.Properties{OriginModelName: tgxRegressionModel},
			Data:       []byte(fmt.Sprintf(`{"id":"task_%s","status":"%s","error":{"code":"TaskTerminated","message":"%s"}}`, status, status, status)),
		}
		require.NoError(t, model.DB.Create(task).Error)
		listed := seedanceCall(r, http.MethodGet, "/api/v3/contents/generations/tasks?filter.status="+status, "", 1)
		require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
		require.EqualValues(t, 1, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
		require.Equal(t, status, gjson.GetBytes(listed.Body.Bytes(), "items.0.status").String())
		require.Equal(t, status, gjson.GetBytes(listed.Body.Bytes(), "items.0.failure_reason.message").String())
	}
}
