package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const (
	ChannelOperationActionDisable = "disable"
	ChannelOperationActionEnable  = "enable"

	ChannelOperationSourceAuto   = "auto"
	ChannelOperationSourceManual = "manual"
)

type ChannelOperationRecord struct {
	Id          int    `json:"id"`
	ChannelID   int    `json:"channel_id" gorm:"index"`
	ChannelName string `json:"channel_name" gorm:"type:varchar(128)"`
	Action      string `json:"action" gorm:"type:varchar(32);index"`
	Source      string `json:"source" gorm:"type:varchar(32);index"`
	Status      int    `json:"status" gorm:"index"`
	Reason      string `json:"reason" gorm:"type:text"`
	ModelName   string `json:"model_name" gorm:"type:varchar(128);index"`
	CreatedAt   int64  `json:"created_at" gorm:"bigint;index"`
}

type ChannelOperationRecordQuery struct {
	ChannelID      int
	Action         string
	Source         string
	ModelName      string
	StartTimestamp int64
	EndTimestamp   int64
	Page           int
	PageSize       int
}

func RecordChannelOperation(record ChannelOperationRecord) error {
	if record.CreatedAt == 0 {
		record.CreatedAt = common.GetTimestamp()
	}
	record.Action = strings.TrimSpace(record.Action)
	record.Source = strings.TrimSpace(record.Source)
	record.ModelName = strings.TrimSpace(record.ModelName)
	return DB.Create(&record).Error
}

func GetChannelOperationRecords(query ChannelOperationRecordQuery) ([]ChannelOperationRecord, int64, error) {
	tx := DB.Model(&ChannelOperationRecord{})
	if query.ChannelID > 0 {
		tx = tx.Where("channel_id = ?", query.ChannelID)
	}
	if query.Action = strings.TrimSpace(query.Action); query.Action != "" {
		tx = tx.Where("action = ?", query.Action)
	}
	if query.Source = strings.TrimSpace(query.Source); query.Source != "" {
		tx = tx.Where("source = ?", query.Source)
	}
	if query.ModelName = strings.TrimSpace(query.ModelName); query.ModelName != "" {
		tx = tx.Where("model_name LIKE ?", "%"+query.ModelName+"%")
	}
	if query.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}

	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page := query.Page
	if page <= 0 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	offset := (page - 1) * pageSize

	var records []ChannelOperationRecord
	err := tx.Order("id desc").Limit(pageSize).Offset(offset).Find(&records).Error
	return records, total, err
}
