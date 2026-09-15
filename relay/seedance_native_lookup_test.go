package relay

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSeedanceLookupScopeLegacyAndAmbiguity(t *testing.T) {
	setupRelayTaskTestDB(t)
	records := []model.Task{
		{TaskID: "task_a", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-shared"}},
		{TaskID: "task_b", UserId: 2, ChannelId: 21, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-shared"}},
		{TaskID: "cgt-legacy", UserId: 1},
		{TaskID: "task_middle", UserId: 1, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task_vendor", OfficialTaskID: "cgt-official"}},
		{TaskID: "task_other", UserId: 2, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task_other_vendor", OfficialTaskID: "cgt-private"}},
		{TaskID: "task_wildcard", UserId: 1, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-%_\"quote"}},
	}
	for i := range records {
		require.NoError(t, model.DB.Create(&records[i]).Error)
	}
	for _, tc := range []struct {
		user     int
		id, want string
	}{{1, "cgt-official", "task_middle"}, {1, "task_vendor", "task_middle"}, {1, "cgt-shared", "task_a"}, {2, "cgt-shared", "task_b"}, {1, "task_a", "task_a"}, {1, "cgt-legacy", "cgt-legacy"}, {1, "cgt-%_\"quote", "task_wildcard"}} {
		task, ok, err := model.GetByTaskIDOrUpstreamID(tc.user, tc.id)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, tc.want, task.TaskID)
	}
	for _, id := range []string{"task_b", "cgt-private", "task_other_vendor", "missing", "cgt-%", ""} {
		_, ok, err := model.GetByTaskIDOrUpstreamID(1, id)
		require.NoError(t, err)
		require.False(t, ok)
	}
	require.NoError(t, model.DB.Create(&model.Task{TaskID: "task_collision", UserId: 1, ChannelId: 22, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-shared"}}).Error)
	_, ok, err := model.GetByTaskIDOrUpstreamID(1, "cgt-shared")
	require.ErrorContains(t, err, "ambiguous")
	require.False(t, ok)
	require.NoError(t, model.DB.Create(&model.Task{TaskID: "task_alias_collision", UserId: 1, PrivateData: model.TaskPrivateData{OfficialTaskID: "task_vendor"}}).Error)
	_, ok, err = model.GetByTaskIDOrUpstreamID(1, "task_vendor")
	require.ErrorContains(t, err, "ambiguous")
	require.False(t, ok)
}
