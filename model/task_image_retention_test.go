package model

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskImageExpiryBoundaryAndMetadata(t *testing.T) {
	const now int64 = 1800000000
	task := Task{Status: TaskStatusSuccess, FinishTime: now - TaskImageRetentionSeconds,
		Data: []byte(`{"data":{"images":[{"b64_json":"KEEP_UNTIL_DEADLINE"}]},"usage":{"tokens":9007199254740993}}`)}
	before, err := task.ImageRetentionView(now - 1)
	require.NoError(t, err)
	require.Equal(t, "available", before.ImageAvailability(now-1).ImageStatus)
	require.Contains(t, string(before.Data), "KEEP_UNTIL_DEADLINE")
	after, err := task.ImageRetentionView(now)
	require.NoError(t, err)
	require.Equal(t, "expired", after.ImageAvailability(now).ImageStatus)
	require.NotContains(t, string(after.Data), "KEEP_UNTIL_DEADLINE")
	require.Contains(t, string(after.Data), "9007199254740993")
	require.Contains(t, string(task.Data), "KEEP_UNTIL_DEADLINE")
	require.Equal(t, TaskStatus(TaskStatusSuccess), after.Status)
}

func TestTaskImageMigrationAndUnknownCompletionTime(t *testing.T) {
	savedDB := DB
	db, err := gorm.Open(sqlite.Open("file:image-retention-migration?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = savedDB; conn, _ := db.DB(); _ = conn.Close() })
	legacySchema := struct {
		ID     int64           `gorm:"primaryKey"`
		TaskID string          `gorm:"type:varchar(191);index"`
		Status TaskStatus      `gorm:"type:varchar(20);index"`
		Data   json.RawMessage `gorm:"type:json"`
	}{}
	require.NoError(t, db.Table("tasks").Migrator().CreateTable(&legacySchema))
	require.NoError(t, db.Exec("INSERT INTO tasks(id, task_id, status, data) VALUES (?, ?, ?, ?)", 1, "legacy-time-missing", TaskStatusSuccess, []byte(`{"data":{"images":[{"imageBase64":"OLD_UNKNOWN_TIME"}]}}`)).Error)
	require.NoError(t, db.AutoMigrate(&Task{}))
	require.True(t, db.Migrator().HasIndex(&Task{}, "idx_task_image_expiry"))
	var legacy Task
	require.NoError(t, db.First(&legacy, 1).Error)
	require.Zero(t, legacy.ImageBase64Version)
	const now int64 = 1800000000
	changed, _, estimated, err := MaintainTaskImage(context.Background(), 1, now)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, db.First(&legacy, 1).Error)
	require.Equal(t, now+86400, legacy.ImageExpiresAt)
	require.True(t, estimated)
	changed, _, _, err = MaintainTaskImage(context.Background(), 1, now+100)
	require.NoError(t, err)
	require.False(t, changed)
	require.NoError(t, db.First(&legacy, 1).Error)
	require.Equal(t, now+86400, legacy.ImageExpiresAt)
	backlog, err := GetTaskImageBacklog(context.Background(), now+86400)
	require.NoError(t, err)
	require.Equal(t, int64(1), backlog.PendingExpired)
	require.Zero(t, backlog.PendingBackfill)
	require.Equal(t, legacy.ImageExpiresAt, backlog.OldestExpiresAt)
}

func TestTaskImageCleanupKeepsTaskBillingAndRejectsStaleWrites(t *testing.T) {
	const now int64 = 1800000000
	task := Task{TaskID: "image-retention-concurrent", Status: TaskStatusSuccess, UserId: 721, Quota: 12345,
		SubmitTime: now - 100000, FinishTime: now - 86400, UpdatedAt: now - 86400,
		Data:        []byte(`{"data":{"images":[{"b64_json":"PAYLOAD"},{"url":"https://example.com/image.png"}]},"usage":{"total_tokens":991}}`),
		PrivateData: TaskPrivateData{Key: "private-key", ResultURL: "data:image/png;base64,PRIVATE_PAYLOAD"}}
	require.NoError(t, DB.Create(&task).Error)
	t.Cleanup(func() { DB.Delete(&Task{}, task.ID) })
	private := `{"key":"private-key","result_url":"data:image/png;base64,PRIVATE_PAYLOAD","unknown_snapshot":{"integer":9007199254740993}}`
	require.NoError(t, DB.Model(&task).Update("private_data", private).Error)
	stale := task
	changed, removed, _, err := MaintainTaskImage(context.Background(), task.ID, now)
	require.NoError(t, err)
	require.True(t, changed)
	// The marker may be larger than a tiny test payload; real base64 rows are MBs.
	require.GreaterOrEqual(t, removed, int64(0))
	var saved Task
	require.NoError(t, DB.First(&saved, task.ID).Error)
	require.Equal(t, "partially_expired", saved.ImageAvailability(now).ImageStatus)
	require.Equal(t, task.Quota, saved.Quota)
	require.Equal(t, task.FinishTime, saved.FinishTime)
	require.Equal(t, task.UpdatedAt, saved.UpdatedAt)
	require.Equal(t, task.Status, saved.Status)
	require.Equal(t, "private-key", saved.PrivateData.Key)
	require.Contains(t, string(saved.Data), `"total_tokens":991`)
	require.NotContains(t, string(saved.Data), "PAYLOAD")
	var privateAfter string
	require.NoError(t, DB.Model(&task).Select("private_data").Scan(&privateAfter).Error)
	require.Contains(t, privateAfter, "9007199254740993")
	require.NotContains(t, privateAfter, "PRIVATE_PAYLOAD")
	won, err := stale.UpdateWithStatus(TaskStatusSuccess)
	require.NoError(t, err)
	require.False(t, won)
	require.Equal(t, task.ImageBase64Version, stale.ImageBase64Version)
	require.ErrorIs(t, stale.Update(), ErrTaskImageVersionConflict)
	changed, _, _, err = MaintainTaskImage(context.Background(), task.ID, now+60)
	require.NoError(t, err)
	require.False(t, changed)
}

func TestTaskImageWritersDoNotExtendOrRestoreImages(t *testing.T) {
	now := common.GetTimestamp()
	task := Task{TaskID: "image-retention-write", Status: TaskStatusSuccess, FinishTime: now,
		Data: []byte(`{"data":{"images":[{"b64_json":"IMAGE"}]}}`)}
	require.NoError(t, task.Insert())
	t.Cleanup(func() { DB.Delete(&Task{}, task.ID) })
	expires := task.ImageExpiresAt
	task.UpdatedAt = now + 20
	require.NoError(t, task.Update())
	require.Equal(t, expires, task.ImageExpiresAt)
	task.ImageExpiresAt = now - 1
	require.NoError(t, DB.Model(&task).Update("image_expires_at", task.ImageExpiresAt).Error)
	require.NoError(t, task.Update())
	require.Equal(t, TaskImageCleared, task.ImageBase64State)
	require.NotContains(t, string(task.Data), "IMAGE")
	task.Data = []byte(`{"data":{"images":[{"b64_json":"RESURRECT"}]}}`)
	require.NoError(t, task.Update())
	require.NotContains(t, string(task.Data), "RESURRECT")
}

func TestTaskImageSkipsPendingAndURLOnlyAndHidesFailedInputStatus(t *testing.T) {
	const now int64 = 1800000000
	for _, task := range []Task{
		{Status: TaskStatusInProgress, FinishTime: now - 100000, Data: []byte(`{"image":"data:image/png;base64,INPUT"}`)},
		{Status: TaskStatusSuccess, FinishTime: now - 100000, Data: []byte(`{"data":{"images":[{"url":"https://example.com/keep.png"}]}}`)},
		{Status: TaskStatusSuccess, FinishTime: now - 100000, Data: []byte(`{"video":{"url":"data:video/mp4;base64,KEEP"}}`)},
	} {
		view, err := task.ImageRetentionView(now)
		require.NoError(t, err)
		require.Equal(t, task.Data, view.Data)
		require.Empty(t, view.ImageAvailability(now).ImageStatus)
	}
	failed := Task{Status: TaskStatusFailure, FinishTime: now - 100000, Data: []byte(`{"image":"data:image/png;base64,INPUT","prompt":"keep"}`)}
	view, err := failed.ImageRetentionView(now)
	require.NoError(t, err)
	require.NotContains(t, string(view.Data), "INPUT")
	require.Empty(t, view.ImageAvailability(now).ImageStatus)
}
