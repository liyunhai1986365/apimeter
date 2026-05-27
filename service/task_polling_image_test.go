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

func TestUpdateImageTasksFetchesDuomiGeminiImageTaskEndpoint(t *testing.T) {
	truncate(t)

	upstreamTaskID := "7154f43c-7765-4e77-d51e-0db4eee107b0"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/duomiapi.com/api/gemini/nano-banana/"+upstreamTaskID, r.URL.Path)
		require.Equal(t, "duomi-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"task_id":"7154f43c-7765-4e77-d51e-0db4eee107b0","state":"succeeded","data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg","file_name":"output.jpeg"}],"description":"done"},"create_time":"1757061229","update_time":"1757061252","msg":"","status":"3","action":"generate"},"exec_time":0.05,"ip":"118.125.2.163"}`))
	}))
	defer upstream.Close()

	duomiLikeBaseURL := upstream.URL + "/duomiapi.com"
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      11,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "duomi-key",
		BaseURL: &duomiLikeBaseURL,
		Status:  common.ChannelStatusEnabled,
		Name:    "duomi-gemini-image",
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
		Properties: model.Properties{
			OriginModelName: "gemini-3-pro-image-preview",
		},
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
	require.Equal(t, "https://cdn3.dmiapi.com/output.jpeg", reloaded.PrivateData.ResultURL)
	require.Equal(t, int64(1757061229), reloaded.CreatedAt)
}

type fakeSunoPollingAdaptor struct {
	response string
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
