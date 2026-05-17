package model

import "github.com/QuantumNous/new-api/common"

type ModelChannelTestResult struct {
	Id           int     `json:"id"`
	ModelID      int     `json:"model_id" gorm:"index:idx_model_channel_test_model_time,priority:1"`
	ModelName    string  `json:"model_name" gorm:"type:varchar(128);index"`
	ChannelID    int     `json:"channel_id" gorm:"index"`
	ChannelName  string  `json:"channel_name"`
	ChannelType  int     `json:"channel_type"`
	Success      bool    `json:"success" gorm:"index"`
	Message      string  `json:"message" gorm:"type:text"`
	ErrorCode    string  `json:"error_code" gorm:"type:varchar(128);index"`
	ResponseTime float64 `json:"response_time"`
	EndpointType string  `json:"endpoint_type" gorm:"type:varchar(64)"`
	IsStream     bool    `json:"is_stream"`
	CreatedAt    int64   `json:"created_at" gorm:"bigint;index:idx_model_channel_test_model_time,priority:2"`
}

func InsertModelChannelTestResults(results []ModelChannelTestResult) error {
	if len(results) == 0 {
		return nil
	}
	now := common.GetTimestamp()
	for i := range results {
		if results[i].CreatedAt == 0 {
			results[i].CreatedAt = now
		}
	}
	return DB.Create(&results).Error
}

func GetLatestModelChannelTestResults(modelID int) ([]ModelChannelTestResult, error) {
	var rows []ModelChannelTestResult
	err := DB.Where("model_id = ?", modelID).
		Order("created_at desc, id desc").
		Limit(200).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	latest := make([]ModelChannelTestResult, 0, len(rows))
	for _, row := range rows {
		if seen[row.ChannelID] {
			continue
		}
		seen[row.ChannelID] = true
		latest = append(latest, row)
	}
	return latest, nil
}
