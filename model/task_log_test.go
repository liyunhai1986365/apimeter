package model

import (
	"context"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskLogsExcludeMediaAndPrivateSnapshots(t *testing.T) {
	truncateTables(t)
	data, err := common.Marshal(map[string]any{"data": map[string]any{"images": []any{map[string]any{"b64_json": strings.Repeat("A", 1024*1024)}}}})
	require.NoError(t, err)
	task := Task{TaskID: "task-large-image", UserId: 1001, ChannelId: 9, SubmitTime: 100, Status: TaskStatusSuccess,
		Data: data, PrivateData: TaskPrivateData{Key: "private-secret", ResultURL: "https://example.com/result.png"},
		Properties: Properties{OriginModelName: "gpt-image-2"}}
	require.NoError(t, DB.Create(&task).Error)
	for _, tasks := range [][]*Task{
		TaskGetAllTasks(0, 100, SyncTaskQueryParams{StartTimestamp: 90, EndTimestamp: 110}),
		TaskGetAllUserTask(1001, 0, 100, SyncTaskQueryParams{}),
	} {
		require.Len(t, tasks, 1)
		require.Empty(t, tasks[0].Data)
		require.Equal(t, TaskPrivateData{}, tasks[0].PrivateData)
		require.Equal(t, task.Properties, tasks[0].Properties)
		body, err := common.Marshal(tasks)
		require.NoError(t, err)
		require.Less(t, len(body), 2000)
	}
	detail, err := GetTaskLogDetail(context.Background(), task.TaskID, 1001, nil)
	require.NoError(t, err)
	require.Equal(t, []byte(data), []byte(detail.Data))
	require.Equal(t, task.PrivateData.ResultURL, detail.GetResultURL())
	require.Zero(t, detail.ChannelId)
	// Provider-facing queries must keep the complete saved result too.
	providerTask, exists, err := GetByTaskId(1001, task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, []byte(data), []byte(providerTask.Data))
}

func TestTaskLogDetailEnforcesOwnerAndWorkspace(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.Create(&[]Workspace{
		{Id: 131, UserId: 1001, Name: "Alpha", Status: WorkspaceStatusEnabled},
		{Id: 132, UserId: 1001, Name: "Beta", Status: WorkspaceStatusEnabled},
	}).Error)
	require.NoError(t, DB.Create(&[]Token{
		{Id: 231, UserId: 1001, WorkspaceId: 131, Name: "alpha", Key: "alpha-task-log"},
		{Id: 232, UserId: 1001, WorkspaceId: 132, Name: "beta", Key: "beta-task-log"},
	}).Error)
	require.NoError(t, DB.Create(&[]Task{
		{TaskID: "task-alpha", UserId: 1001, TokenId: 231},
		{TaskID: "task-beta", UserId: 1001, TokenId: 232},
		{TaskID: "task-no-token", UserId: 1001},
		{TaskID: "task-other-owner", UserId: 2002, TokenId: 231},
	}).Error)
	for _, tc := range []struct {
		name, taskID string
		userID       int
		workspaces   []int
		allowed      bool
	}{
		{"owner", "task-beta", 1001, nil, true},
		{"workspace", "task-alpha", 1001, []int{131}, true},
		{"different-workspace", "task-beta", 1001, []int{131}, false},
		{"empty-scope", "task-alpha", 1001, []int{}, false},
		{"unassigned-token", "task-no-token", 1001, []int{131}, false},
		{"different-owner", "task-other-owner", 1001, nil, false},
		{"workspace-owner-boundary", "task-other-owner", 1001, []int{131}, false},
		{"admin", "task-other-owner", 0, nil, true},
		{"missing", "task-missing", 0, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := GetTaskLogDetail(context.Background(), tc.taskID, tc.userID, tc.workspaces)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
			}
		})
	}
}
