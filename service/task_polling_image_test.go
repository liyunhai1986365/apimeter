package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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
