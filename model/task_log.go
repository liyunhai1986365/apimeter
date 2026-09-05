package model

import (
	"context"

	"gorm.io/gorm"
)

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
