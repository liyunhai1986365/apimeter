package service

import (
	"context"
	"io"
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
	"github.com/stretchr/testify/require"
)

func TestUpdateImageTasksFetchesImageTaskEndpoint(t *testing.T) {
	truncate(t)

	upstreamTaskID := "7154f43c-7765-4e77-d51e-0db4eee107b0"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tasks/"+upstreamTaskID, r.URL.Path)
		require.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0","state":"succeeded","progress":0,"create_time":1779125744,"update_time":1779125807,"action":"generate","data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png","file_name":"output.png"}]}}`))
	}))
	defer upstream.Close()

	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      11,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-upstream",
		BaseURL: &upstream.URL,
		Status:  common.ChannelStatusEnabled,
		Name:    "async-image",
	}).Error)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     upstreamTaskID,
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
			UpstreamTaskID: upstreamTaskID,
			AsyncImage:     true,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	err := UpdateImageTasks(context.Background(), map[int][]string{11: []string{upstreamTaskID}}, map[string]*model.Task{upstreamTaskID: task})
	require.NoError(t, err)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("task_id = ?", upstreamTaskID).First(&reloaded).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, "https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png", reloaded.PrivateData.ResultURL)
}

func TestIsImageAsyncTaskMatchesLegacyOpenAITaskID(t *testing.T) {
	task := &model.Task{
		TaskID:   "7154f43c-7765-4e77-d51e-0db4eee107b0",
		Platform: constant.TaskPlatform("1"),
		Action:   constant.TaskActionGenerate,
		Properties: model.Properties{
			OriginModelName: "gpt-image-2",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "7154f43c-7765-4e77-d51e-0db4eee107b0",
		},
	}
	require.True(t, isImageAsyncTask(task))
}

func TestUpdateImageTasksUsesConfigurableImageFetcher(t *testing.T) {
	truncate(t)

	originFetcher := ConfigurableImageTaskFetcher
	called := false
	ConfigurableImageTaskFetcher = func(ch *model.Channel, task *model.Task, taskID, key, proxy string) ([]byte, int, bool, error) {
		called = true
		require.Equal(t, constant.ChannelTypeConfigurable, ch.Type)
		require.Equal(t, "apixo-task", taskID)
		require.Equal(t, "sk-apixo", key)
		require.Equal(t, "", proxy)
		return []byte(`{"id":"apixo-task","state":"success","progress":100,"data":{"images":[{"url":"https://file.apixo.ai/output.png"}]}}`), http.StatusOK, true, nil
	}
	t.Cleanup(func() { ConfigurableImageTaskFetcher = originFetcher })

	channel := model.Channel{
		Id:      901,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "sk-apixo",
		BaseURL: common.GetPointer("https://api.apixo.ai"),
		Status:  common.ChannelStatusEnabled,
		Name:    "apixo-image",
		Models:  "gpt-image-2",
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "apixo-gpt-image-2",
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "apixo-task",
		UserId:     7,
		ChannelId:  channel.Id,
		Action:     constant.TaskActionGenerate,
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		CreatedAt:  now,
		UpdatedAt:  now,
		SubmitTime: now,
		Platform:   constant.TaskPlatform("999"),
		Properties: model.Properties{
			OriginModelName: "gpt-image-2",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "apixo-task",
			AsyncImage:     true,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	err := UpdateImageTasks(context.Background(), map[int][]string{channel.Id: []string{"apixo-task"}}, map[string]*model.Task{"apixo-task": task})
	require.NoError(t, err)
	require.True(t, called)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&reloaded).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, "https://file.apixo.ai/output.png", reloaded.PrivateData.ResultURL)
}

func TestUpdateVideoTasksPollsConfigurableProfile(t *testing.T) {
	truncate(t)

	upstreamTaskID := "dashscope-task"
	originGetTaskAdaptorFunc := GetTaskAdaptorFunc
	adaptor := &profileAwarePollingAdaptor{}
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatform("41"), platform)
		return adaptor
	}
	t.Cleanup(func() { GetTaskAdaptorFunc = originGetTaskAdaptorFunc })

	channel := model.Channel{
		Id:      901,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "sk-upstream",
		BaseURL: common.GetPointer("https://dashscope.aliyuncs.com"),
		Status:  common.ChannelStatusEnabled,
		Name:    "happyhorse",
		Models:  "happyhorse-1.0-t2v",
		Group:   "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "happyhorse-video",
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	now := time.Now().Unix()
	task := &model.Task{
		TaskID:     "task_public",
		UserId:     7,
		ChannelId:  channel.Id,
		Action:     constant.TaskActionGenerate,
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		CreatedAt:  now,
		UpdatedAt:  now,
		SubmitTime: now,
		Platform:   constant.TaskPlatform("41"),
		Properties: model.Properties{
			OriginModelName:   "happyhorse-1.0-t2v",
			UpstreamModelName: "happyhorse-1.0-t2v",
		},
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: upstreamTaskID,
		},
	}
	require.NoError(t, model.DB.Create(task).Error)

	err := UpdateVideoTasks(context.Background(), task.Platform, map[int][]string{channel.Id: []string{upstreamTaskID}}, map[string]*model.Task{upstreamTaskID: task})
	require.NoError(t, err)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("task_id = ?", task.TaskID).First(&reloaded).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.Equal(t, "https://dashscope-result/video.mp4", reloaded.PrivateData.ResultURL)
	require.Contains(t, string(reloaded.Data), `"request_id":"req-1"`)
	require.Equal(t, "happyhorse-video", adaptor.profileID)
	require.Equal(t, constant.ChannelTypeConfigurable, adaptor.channelType)
}

type fakeSunoPollingAdaptor struct {
	response string
}

type profileAwarePollingAdaptor struct {
	profileID    string
	channelType  int
	baseURL      string
	key          string
	fetchTaskID  string
	parseProfile string
}

func (a *profileAwarePollingAdaptor) Init(info *relaycommon.RelayInfo) {
	if info != nil && info.ChannelMeta != nil {
		a.channelType = info.ChannelMeta.ChannelType
		a.baseURL = info.ChannelMeta.ChannelBaseUrl
		a.key = info.ChannelMeta.ApiKey
		if info.ChannelMeta.ChannelSetting.Protocol != nil {
			a.profileID = info.ChannelMeta.ChannelSetting.Protocol.ProfileID
		}
	}
}

func (a *profileAwarePollingAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	a.baseURL = baseURL
	a.key = key
	a.fetchTaskID, _ = body["task_id"].(string)
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"request_id":"req-1","output":{"task_id":"dashscope-task","task_status":"SUCCEEDED","video_url":"https://dashscope-result/video.mp4"},"usage":{"duration":5,"ratio":"16:9"}}`)),
	}, nil
}

func (a *profileAwarePollingAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	a.parseProfile = a.profileID
	if a.profileID != "happyhorse-video" {
		return relaycommon.FailTaskInfo("missing configurable profile"), nil
	}
	return &relaycommon.TaskInfo{
		TaskID:   a.fetchTaskID,
		Status:   string(model.TaskStatusSuccess),
		Url:      "https://dashscope-result/video.mp4",
		Progress: "100%",
	}, nil
}

func (a *profileAwarePollingAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return 0
}

func (a fakeSunoPollingAdaptor) Init(info *relaycommon.RelayInfo) {}

func (a fakeSunoPollingAdaptor) FetchTask(baseURL string, key string, body map[string]any, proxy string) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(a.response)),
	}, nil
}

func (a fakeSunoPollingAdaptor) ParseTaskResult(body []byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}

func (a fakeSunoPollingAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	return 0
}

func TestUpdateSunoTasksKeepsMultiOutputTaskPendingUntilAllChildrenSucceed(t *testing.T) {
	truncate(t)

	upstreamOne := "upstream_1"
	upstreamTwo := "upstream_2"
	now := time.Now().Unix()
	baseURL := "https://api.wike.cc"
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      35,
		Type:    constant.ChannelTypeSunoAPI,
		Key:     "suno-key",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
		Name:    "suno",
	}).Error)

	task := &model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: 35,
		Action:    constant.SunoActionMusic,
		Status:    model.TaskStatusQueued,
		Progress:  "0%",
		CreatedAt: now,
		UpdatedAt: now,
		Platform:  constant.TaskPlatformSuno,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: upstreamOne + "," + upstreamTwo,
		},
		Data: []byte(`null`),
	}
	require.NoError(t, model.DB.Create(task).Error)

	originGetTaskAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatformSuno, platform)
		return fakeSunoPollingAdaptor{response: `{"code":"success","data":[{"task_id":"upstream_1","status":"SUCCESS","data":[{"id":"song_1","audio_url":"https://cdn.example/song1.mp3"}]},{"task_id":"upstream_2","status":"QUEUED","data":null}]}`}
	}
	t.Cleanup(func() { GetTaskAdaptorFunc = originGetTaskAdaptorFunc })

	err := UpdateSunoTasks(context.Background(), map[int][]string{35: []string{upstreamOne, upstreamTwo}}, map[string]*model.Task{
		upstreamOne: task,
		upstreamTwo: task,
	})
	require.NoError(t, err)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "task_public").First(&reloaded).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusQueued), reloaded.Status)
	require.NotEqual(t, "100%", reloaded.Progress)
	require.Zero(t, reloaded.FinishTime)

	var songs []dto.SunoSong
	require.NoError(t, common.Unmarshal(reloaded.Data, &songs))
	require.Len(t, songs, 1)
	require.Equal(t, "song_1", songs[0].ID)
}

func TestUpdateSunoTasksMergesAllSuccessfulChildOutputs(t *testing.T) {
	truncate(t)

	upstreamOne := "upstream_1"
	upstreamTwo := "upstream_2"
	now := time.Now().Unix()
	baseURL := "https://api.wike.cc"
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      35,
		Type:    constant.ChannelTypeSunoAPI,
		Key:     "suno-key",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
		Name:    "suno",
	}).Error)

	task := &model.Task{
		TaskID:    "task_public_success",
		UserId:    7,
		ChannelId: 35,
		Action:    constant.SunoActionMusic,
		Status:    model.TaskStatusQueued,
		Progress:  "0%",
		CreatedAt: now,
		UpdatedAt: now,
		Platform:  constant.TaskPlatformSuno,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: upstreamOne + "," + upstreamTwo,
		},
		Data: []byte(`null`),
	}
	require.NoError(t, model.DB.Create(task).Error)

	originGetTaskAdaptorFunc := GetTaskAdaptorFunc
	GetTaskAdaptorFunc = func(platform constant.TaskPlatform) TaskPollingAdaptor {
		require.Equal(t, constant.TaskPlatformSuno, platform)
		return fakeSunoPollingAdaptor{response: `{"code":"success","data":[{"task_id":"upstream_1","status":"SUCCESS","finish_time":111,"data":[{"id":"song_1","audio_url":"https://cdn.example/song1.mp3"}]},{"task_id":"upstream_2","status":"SUCCESS","finish_time":222,"data":[{"id":"song_2","audio_url":"https://cdn.example/song2.mp3"}]}]}`}
	}
	t.Cleanup(func() { GetTaskAdaptorFunc = originGetTaskAdaptorFunc })

	err := UpdateSunoTasks(context.Background(), map[int][]string{35: []string{upstreamOne, upstreamTwo}}, map[string]*model.Task{
		upstreamOne: task,
		upstreamTwo: task,
	})
	require.NoError(t, err)

	var reloaded model.Task
	require.NoError(t, model.DB.Where("task_id = ?", "task_public_success").First(&reloaded).Error)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), reloaded.Status)
	require.Equal(t, "100%", reloaded.Progress)
	require.EqualValues(t, 222, reloaded.FinishTime)

	var songs []dto.SunoSong
	require.NoError(t, common.Unmarshal(reloaded.Data, &songs))
	require.Len(t, songs, 2)
	require.Equal(t, "song_1", songs[0].ID)
	require.Equal(t, "song_2", songs[1].ID)
}
