package model

import (
	"context"

	"github.com/QuantumNous/new-api/constant"
)

// Scan local tasks in bounded pages, without holding a transaction across the
// callback. Filtering JSON properties in Go keeps SQLite/MySQL/Postgres parity.
func VisitYHUserTasks(ctx context.Context, userID int, visit func(*Task) error) error {
	var channels []Channel
	if err := DB.WithContext(ctx).Select("id", "setting").Where("type = ?", constant.ChannelTypeConfigurable).Find(&channels).Error; err != nil {
		return err
	}
	var ids []int
	for _, ch := range channels {
		if p := ch.GetSetting().Protocol; p != nil && p.ProfileID == "seedance-yh" {
			ids = append(ids, ch.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var last int64
	for {
		var tasks []Task
		query := DB.WithContext(ctx).Where("user_id = ? AND channel_id IN ?", userID, ids)
		if last != 0 {
			query = query.Where("id < ?", last)
		}
		if err := query.Order("id DESC").Limit(200).Find(&tasks).Error; err != nil {
			return err
		}
		for i := range tasks {
			if err := visit(&tasks[i]); err != nil {
				return err
			}
		}
		if len(tasks) < 200 {
			return nil
		}
		last = tasks[len(tasks)-1].ID
	}
}
