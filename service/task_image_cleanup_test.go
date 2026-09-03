package service

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestTaskImageCleanupBackfillsWithoutDeletingTaskOrBilling(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	tasks := []model.Task{
		{TaskID: "old-image", Status: model.TaskStatusSuccess, FinishTime: now - 90000, Quota: 987, Data: []byte(`{"data":{"images":[{"b64_json":"OLD"}]},"usage":{"tokens":123}}`)},
		{TaskID: "new-image", Status: model.TaskStatusSuccess, FinishTime: now - 100, Quota: 789, Data: []byte(`{"data":{"images":[{"b64_json":"NEW"}]}}`)},
		{TaskID: "pending-image", Status: model.TaskStatusInProgress, SubmitTime: now - 90000, Data: []byte(`{"image":"data:image/png;base64,REQUEST"}`)},
		{TaskID: "video", Status: model.TaskStatusSuccess, FinishTime: now - 90000, Data: []byte(`{"video":{"url":"https://example.com/video.mp4"}}`)},
	}
	require.NoError(t, model.DB.Create(&tasks).Error)
	state := TaskImageCleanupState{}
	require.NoError(t, RunTaskImageCleanup(context.Background(), &state, now, 1000, time.Second*5, nil))
	require.Equal(t, 3, state.Scanned)
	var count int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
	require.Equal(t, int64(4), count)
	var old, recent, pending model.Task
	require.NoError(t, model.DB.First(&old, tasks[0].ID).Error)
	require.NoError(t, model.DB.First(&recent, tasks[1].ID).Error)
	require.NoError(t, model.DB.First(&pending, tasks[2].ID).Error)
	require.Equal(t, model.TaskImageCleared, old.ImageBase64State)
	require.Equal(t, 987, old.Quota)
	require.Contains(t, string(old.Data), `"tokens":123`)
	require.Equal(t, model.TaskImagePresent, recent.ImageBase64State)
	require.Contains(t, string(recent.Data), "NEW")
	require.Equal(t, model.TaskImageUnknown, pending.ImageBase64State)
	require.Contains(t, string(pending.Data), "REQUEST")
}

func TestTaskImageCleanupCursorSkipsPoisonRowsAndResumes(t *testing.T) {
	truncate(t)
	now := common.GetTimestamp()
	bad := model.Task{TaskID: "bad-image", Status: model.TaskStatusSuccess, Data: []byte(`{"b64_json":`)}
	good := model.Task{TaskID: "good-image", Status: model.TaskStatusSuccess, FinishTime: now - 90000, Data: []byte(`{"data":{"images":[{"b64_json":"OLD"}]}}`)}
	require.NoError(t, model.DB.Create(&bad).Error)
	require.NoError(t, model.DB.Create(&good).Error)
	state := TaskImageCleanupState{}
	require.NoError(t, RunTaskImageCleanup(context.Background(), &state, now, 2, time.Second*5, nil))
	require.Equal(t, 1, state.Failed)
	require.Equal(t, bad.ID, state.BackfillAfterID)
	require.NoError(t, RunTaskImageCleanup(context.Background(), &state, now, 2, time.Second*5, nil))
	require.Equal(t, good.ID, state.BackfillAfterID)
	var saved model.Task
	require.NoError(t, model.DB.First(&saved, good.ID).Error)
	require.Equal(t, model.TaskImageCleared, saved.ImageBase64State)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, RunTaskImageCleanup(ctx, &state, now, 1000, time.Second, nil), context.Canceled)
}

func TestTaskImageCleanupSchedulingOptInAndLease(t *testing.T) {
	truncate(t)
	t.Setenv("TASK_IMAGE_BASE64_CLEANUP_ENABLED", "false")
	handler := taskImageCleanupHandler{}
	withSystemTaskRegistry(t, handler)
	runSystemTaskScheduler()
	require.Zero(t, countSystemTasks(t, handler.Type()))
	t.Setenv("TASK_IMAGE_BASE64_CLEANUP_ENABLED", "true")
	runSystemTaskScheduler()
	runSystemTaskScheduler()
	require.Equal(t, int64(1), countSystemTasks(t, handler.Type()))
	job, err := model.GetLatestSystemTask(handler.Type())
	require.NoError(t, err)
	claimed, won, err := model.ClaimSystemTask(job.ID, handler.Type(), "image-cleanup-test", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.True(t, won)
	_, wonAgain, err := model.ClaimSystemTask(job.ID, handler.Type(), "other-node", common.GetTimestamp()+60)
	require.NoError(t, err)
	require.False(t, wonAgain)
	handler.Run(context.Background(), claimed, "image-cleanup-test")
	job, err = model.GetLatestSystemTask(handler.Type())
	require.NoError(t, err)
	require.Equal(t, model.SystemTaskStatusSucceeded, job.Status)
}
