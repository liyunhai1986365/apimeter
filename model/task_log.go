package model

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func taskPrivateIDExpressions() (upstream, official string, err error) {
	switch DB.Dialector.Name() {
	case "mysql":
		upstream = "JSON_UNQUOTE(JSON_EXTRACT(private_data, '$.upstream_task_id'))"
	case "postgres":
		upstream = "CAST(private_data AS json) ->> 'upstream_task_id'"
	case "sqlite":
		// SQLite does not enforce JSON validity. Legacy empty or malformed
		// metadata must not break a whole log page or another task's ID lookup.
		upstream = "CASE WHEN json_valid(private_data) THEN json_extract(private_data, '$.upstream_task_id') END"
	default:
		return "", "", fmt.Errorf("unsupported database for task IDs")
	}
	return upstream, strings.ReplaceAll(upstream, "upstream_task_id", "official_task_id"), nil
}

// loadTaskLogIDs reads only the IDs for the already authorized page, in one
// batch. Never load private keys or potentially large billing/request snapshots.
func loadTaskLogIDs(tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	upstream, official, err := taskPrivateIDExpressions()
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(tasks))
	byID := make(map[int64]*Task, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
		byID[task.ID] = task
	}
	var rows []struct {
		ID             int64
		UpstreamTaskID string
		OfficialTaskID string
	}
	projection := "id, COALESCE(NULLIF((" + upstream + "), 'null'), '') AS upstream_task_id, " +
		"COALESCE(NULLIF((" + official + "), 'null'), '') AS official_task_id"
	if err := DB.Model(&Task{}).Select(projection).Where("id IN ?", ids).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		task := byID[row.ID]
		task.PrivateData.UpstreamTaskID = row.UpstreamTaskID
		task.PrivateData.OfficialTaskID = row.OfficialTaskID
	}
	return nil
}

// GetTaskLogDetail reads the saved result without polling the provider or settling
// billing. A positive userId enforces the same owner/workspace scope as the list.
func GetTaskLogDetail(ctx context.Context, taskID string, userId int, allowedWorkspaceIds []int) (*Task, error) {
	if taskID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	query := DB.WithContext(ctx).Where("task_id = ?", taskID)
	if userId > 0 {
		query = query.Where("user_id = ?", userId).Omit("channel_id")
		query = applyTaskTokenFilters(query, userId, SyncTaskQueryParams{AllowedWorkspaceIds: allowedWorkspaceIds})
	}
	var task Task
	if err := query.First(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}
