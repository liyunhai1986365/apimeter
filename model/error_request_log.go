package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type ErrorRequestLog struct {
	Id                int    `json:"id"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index"`
	LogId             int    `json:"log_id" gorm:"index;default:0"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index;default:''"`
	UpstreamRequestId string `json:"upstream_request_id" gorm:"type:varchar(128);index;default:''"`
	UserId            int    `json:"user_id" gorm:"index;default:0"`
	Username          string `json:"username" gorm:"type:varchar(128);index;default:''"`
	TokenId           int    `json:"token_id" gorm:"index;default:0"`
	TokenName         string `json:"token_name" gorm:"type:varchar(128);index;default:''"`
	ModelName         string `json:"model_name" gorm:"type:varchar(191);index;default:''"`
	Group             string `json:"group" gorm:"type:varchar(64);index;default:''"`
	IsStream          bool   `json:"is_stream" gorm:"default:false"`
	RequestMethod     string `json:"request_method" gorm:"type:varchar(16);index;default:''"`
	RequestPath       string `json:"request_path" gorm:"type:varchar(255);index;default:''"`
	RequestURL        string `json:"request_url" gorm:"type:text"`
	RequestHeaders    string `json:"request_headers" gorm:"type:text"`
	RequestBody       string `json:"request_body" gorm:"type:text"`
	RequestHash       string `json:"request_hash" gorm:"type:varchar(128);index;default:''"`
	RequestTruncated  bool   `json:"request_truncated" gorm:"default:false"`
	ContentLength     int64  `json:"content_length" gorm:"default:0"`
	ErrorType         string `json:"error_type" gorm:"type:varchar(64);index;default:''"`
	ErrorCode         string `json:"error_code" gorm:"type:varchar(128);index;default:''"`
	StatusCode        int    `json:"status_code" gorm:"index;default:0"`
}

func RecordErrorRequestLog(record *ErrorRequestLog) error {
	if record == nil {
		return nil
	}
	if record.CreatedAt == 0 {
		record.CreatedAt = common.GetTimestamp()
	}
	trimErrorRequestLog(record)
	return LOG_DB.Create(record).Error
}

func GetErrorRequestLogByLogID(logID int) (*ErrorRequestLog, error) {
	if logID <= 0 {
		return nil, nil
	}
	var record ErrorRequestLog
	if err := LOG_DB.Where("log_id = ?", logID).Order("id desc").First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func trimErrorRequestLog(record *ErrorRequestLog) {
	record.RequestId = strings.TrimSpace(record.RequestId)
	record.UpstreamRequestId = strings.TrimSpace(record.UpstreamRequestId)
	record.Username = strings.TrimSpace(record.Username)
	record.TokenName = strings.TrimSpace(record.TokenName)
	record.ModelName = strings.TrimSpace(record.ModelName)
	record.Group = strings.TrimSpace(record.Group)
	record.RequestMethod = strings.TrimSpace(record.RequestMethod)
	record.RequestPath = strings.TrimSpace(record.RequestPath)
	record.RequestURL = strings.TrimSpace(record.RequestURL)
	record.RequestHash = strings.TrimSpace(record.RequestHash)
	record.ErrorType = strings.TrimSpace(record.ErrorType)
	record.ErrorCode = strings.TrimSpace(record.ErrorCode)
}
