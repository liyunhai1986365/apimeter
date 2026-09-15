package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestPollingBatchesPreserveEveryLocalTask(t *testing.T) {
	tasks := []*model.Task{
		{ID: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "same", Key: "a"}},
		{ID: 2, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "other", Key: "a"}},
		{ID: 3, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "same", Key: "b"}},
		{ID: 4, ChannelId: 21, PrivateData: model.TaskPrivateData{UpstreamTaskID: "same", Key: "c"}},
		{ID: 5, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "same", Key: "a"}},
	}
	seen := map[int64]int{}
	for _, batch := range groupTasksForPolling(tasks) {
		ids := map[string]bool{}
		for _, task := range batch {
			require.Equal(t, batch[0].ChannelId, task.ChannelId)
			require.False(t, ids[task.GetUpstreamTaskID()], "the production polling map must never overwrite a local task")
			ids[task.GetUpstreamTaskID()] = true
			seen[task.ID]++
		}
	}
	for _, task := range tasks {
		require.Equal(t, 1, seen[task.ID])
	}
}
