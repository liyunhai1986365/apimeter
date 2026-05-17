package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type ChannelModelErrorStats struct {
	ModelName     string `json:"model_name"`
	ErrorCount    int64  `json:"error_count"`
	LatestError   string `json:"latest_error"`
	LatestErrorAt int64  `json:"latest_error_at"`
}

func GetChannelModelErrorStats(channelID int, threshold int, windowSeconds int64, now int64) ([]ChannelModelErrorStats, error) {
	if channelID <= 0 || threshold <= 0 || windowSeconds <= 0 {
		return []ChannelModelErrorStats{}, nil
	}
	if now <= 0 {
		now = common.GetTimestamp()
	}
	since := now - windowSeconds
	if since < 0 {
		since = 0
	}

	var rows []struct {
		ModelName     string
		ErrorCount    int64
		LatestErrorAt int64
	}
	err := model.LOG_DB.Model(&model.Log{}).
		Select("model_name, COUNT(*) AS error_count, MAX(created_at) AS latest_error_at").
		Where("channel_id = ? AND type = ? AND created_at >= ? AND model_name <> ?", channelID, model.LogTypeError, since, "").
		Group("model_name").
		Having("COUNT(*) >= ?", threshold).
		Order("error_count DESC, latest_error_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	stats := make([]ChannelModelErrorStats, 0, len(rows))
	for _, row := range rows {
		var latest model.Log
		latestErr := model.LOG_DB.Model(&model.Log{}).
			Where("channel_id = ? AND type = ? AND model_name = ? AND created_at >= ?", channelID, model.LogTypeError, row.ModelName, since).
			Order("created_at DESC, id DESC").
			First(&latest).Error
		if latestErr != nil {
			return nil, latestErr
		}
		stats = append(stats, ChannelModelErrorStats{
			ModelName:     row.ModelName,
			ErrorCount:    row.ErrorCount,
			LatestError:   strings.TrimSpace(latest.Content),
			LatestErrorAt: row.LatestErrorAt,
		})
	}
	return stats, nil
}
