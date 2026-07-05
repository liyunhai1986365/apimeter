package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type RetryRouteEvent struct {
	Id                int    `json:"id"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"bigint;default:0"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index"`
	UpstreamRequestId string `json:"upstream_request_id" gorm:"type:varchar(128);index"`
	AttemptIndex      int    `json:"attempt_index" gorm:"default:0"`
	UserId            int    `json:"user_id" gorm:"index"`
	Username          string `json:"username" gorm:"type:varchar(128);index;default:''"`
	TokenId           int    `json:"token_id" gorm:"index;default:0"`
	TokenName         string `json:"token_name" gorm:"type:varchar(128);index;default:''"`
	WorkspaceId       int    `json:"workspace_id" gorm:"index;default:0"`
	WorkspaceName     string `json:"workspace_name" gorm:"type:varchar(128);index;default:''"`
	OriginalModel     string `json:"original_model" gorm:"type:varchar(191);index;default:''"`
	UpstreamModel     string `json:"upstream_model" gorm:"type:varchar(191);index;default:''"`
	IsStream          bool   `json:"is_stream" gorm:"default:false"`
	RequestPath       string `json:"request_path" gorm:"type:varchar(255);index;default:''"`
	RuleSource        string `json:"rule_source" gorm:"type:varchar(32);index;default:''"`
	RuleName          string `json:"rule_name" gorm:"type:varchar(128);index;default:''"`
	Action            string `json:"action" gorm:"type:varchar(32);index;default:''"`
	Matched           bool   `json:"matched" gorm:"default:false"`
	Routed            bool   `json:"routed" gorm:"default:false"`
	FinalSuccess      bool   `json:"final_success" gorm:"default:false"`
	FinalStatus       string `json:"final_status" gorm:"type:varchar(32);index;default:''"`
	SourceChannelId   int    `json:"source_channel_id" gorm:"index;default:0"`
	SourceChannelName string `json:"source_channel_name" gorm:"type:varchar(128);default:''"`
	SourceChannelType int    `json:"source_channel_type" gorm:"index;default:0"`
	TargetChannelId   int    `json:"target_channel_id" gorm:"index;default:0"`
	TargetChannelName string `json:"target_channel_name" gorm:"type:varchar(128);default:''"`
	TargetChannelType int    `json:"target_channel_type" gorm:"index;default:0"`
	SourceGroup       string `json:"source_group" gorm:"type:varchar(64);index;default:''"`
	TargetGroup       string `json:"target_group" gorm:"type:varchar(64);index;default:''"`
	StatusCode        int    `json:"status_code" gorm:"index;default:0"`
	ErrorType         string `json:"error_type" gorm:"type:varchar(64);index;default:''"`
	ErrorCode         string `json:"error_code" gorm:"type:varchar(128);index;default:''"`
	ErrorMessage      string `json:"error_message" gorm:"type:text"`
	LogId             int    `json:"log_id" gorm:"index;default:0"`
	RequestHash       string `json:"request_hash" gorm:"type:varchar(128);index;default:''"`
	ResponseHash      string `json:"response_hash" gorm:"type:varchar(128);index;default:''"`
	UseTimeMs         int    `json:"use_time_ms" gorm:"default:0"`
	Extra             string `json:"extra" gorm:"type:text"`
}

type RetryRouteEventQuery struct {
	RequestId       string
	Username        string
	UserId          int
	TokenName       string
	TokenId         int
	ModelName       string
	RuleName        string
	Action          string
	SourceChannelId int
	TargetChannelId int
	SourceGroup     string
	TargetGroup     string
	StatusCode      int
	ErrorCode       string
	ErrorType       string
	FinalStatus     string
	StartTimestamp  int64
	EndTimestamp    int64
	Page            int
	PageSize        int
}

type RetryRouteEventStats struct {
	Total               int64            `json:"total"`
	Matched             int64            `json:"matched"`
	Routed              int64            `json:"routed"`
	FinalSuccess        int64            `json:"final_success"`
	RecoverySuccessRate float64          `json:"recovery_success_rate"`
	ActionBreakdown     map[string]int64 `json:"action_breakdown"`
	RuleBreakdown       map[string]int64 `json:"rule_breakdown"`
	SourceChannelTop    map[string]int64 `json:"source_channel_top"`
	TargetChannelTop    map[string]int64 `json:"target_channel_top"`
	ModelBreakdown      map[string]int64 `json:"model_breakdown"`
	ErrorCodeBreakdown  map[string]int64 `json:"error_code_breakdown"`
}

func RecordRetryRouteEvent(event *RetryRouteEvent) error {
	if event == nil {
		return nil
	}
	now := common.GetTimestamp()
	if event.CreatedAt == 0 {
		event.CreatedAt = now
	}
	event.UpdatedAt = now
	trimRetryRouteEvent(event)
	return DB.Create(event).Error
}

func UpdateRetryRouteEventTarget(id int, channel *Channel, targetGroup string) error {
	if id <= 0 {
		return nil
	}
	updates := map[string]interface{}{
		"updated_at":   common.GetTimestamp(),
		"routed":       true,
		"target_group": strings.TrimSpace(targetGroup),
	}
	if channel != nil {
		updates["target_channel_id"] = channel.Id
		updates["target_channel_name"] = channel.Name
		updates["target_channel_type"] = channel.Type
	}
	return DB.Model(&RetryRouteEvent{}).Where("id = ?", id).Updates(updates).Error
}

func AttachRetryRouteEventLog(requestId string, logId int) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" || logId <= 0 {
		return nil
	}
	return DB.Model(&RetryRouteEvent{}).
		Where("request_id = ? AND log_id = 0", requestId).
		Updates(map[string]interface{}{"log_id": logId, "updated_at": common.GetTimestamp()}).
		Error
}

func MarkRetryRouteEventsFinal(requestId string, success bool, status string) error {
	requestId = strings.TrimSpace(requestId)
	if requestId == "" {
		return nil
	}
	return DB.Model(&RetryRouteEvent{}).
		Where("request_id = ?", requestId).
		Updates(map[string]interface{}{
			"final_success": success,
			"final_status":  strings.TrimSpace(status),
			"updated_at":    common.GetTimestamp(),
		}).Error
}

func GetRetryRouteEvents(query RetryRouteEventQuery) ([]RetryRouteEvent, int64, error) {
	tx := applyRetryRouteEventFilters(DB.Model(&RetryRouteEvent{}), query)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	page, pageSize := normalizeRetryRouteEventPage(query.Page, query.PageSize)
	var events []RetryRouteEvent
	err := tx.Order("id desc").Limit(pageSize).Offset((page - 1) * pageSize).Find(&events).Error
	return events, total, err
}

func GetRetryRouteEventByID(id int) (*RetryRouteEvent, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var event RetryRouteEvent
	if err := DB.First(&event, id).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

func GetRetryRouteEventStats(query RetryRouteEventQuery) (RetryRouteEventStats, error) {
	stats := RetryRouteEventStats{
		ActionBreakdown:    map[string]int64{},
		RuleBreakdown:      map[string]int64{},
		SourceChannelTop:   map[string]int64{},
		TargetChannelTop:   map[string]int64{},
		ModelBreakdown:     map[string]int64{},
		ErrorCodeBreakdown: map[string]int64{},
	}
	base := applyRetryRouteEventFilters(DB.Model(&RetryRouteEvent{}), query)
	if err := base.Count(&stats.Total).Error; err != nil {
		return stats, err
	}
	if err := applyRetryRouteEventFilters(DB.Model(&RetryRouteEvent{}), query).Where("matched = ?", true).Count(&stats.Matched).Error; err != nil {
		return stats, err
	}
	if err := applyRetryRouteEventFilters(DB.Model(&RetryRouteEvent{}), query).Where("routed = ?", true).Count(&stats.Routed).Error; err != nil {
		return stats, err
	}
	if err := applyRetryRouteEventFilters(DB.Model(&RetryRouteEvent{}), query).Where("final_success = ?", true).Count(&stats.FinalSuccess).Error; err != nil {
		return stats, err
	}
	if stats.Total > 0 {
		stats.RecoverySuccessRate = float64(stats.FinalSuccess) / float64(stats.Total)
	}
	stats.ActionBreakdown = retryRouteBreakdown(query, "action")
	stats.RuleBreakdown = retryRouteBreakdown(query, "rule_name")
	stats.SourceChannelTop = retryRouteBreakdown(query, "source_channel_id")
	stats.TargetChannelTop = retryRouteBreakdown(query, "target_channel_id")
	stats.ModelBreakdown = retryRouteBreakdown(query, "original_model")
	stats.ErrorCodeBreakdown = retryRouteBreakdown(query, "error_code")
	return stats, nil
}

func applyRetryRouteEventFilters(tx *gorm.DB, query RetryRouteEventQuery) *gorm.DB {
	if query.RequestId = strings.TrimSpace(query.RequestId); query.RequestId != "" {
		tx = tx.Where("request_id = ?", query.RequestId)
	}
	if query.Username = strings.TrimSpace(query.Username); query.Username != "" {
		tx = tx.Where("username = ?", query.Username)
	}
	if query.UserId > 0 {
		tx = tx.Where("user_id = ?", query.UserId)
	}
	if query.TokenName = strings.TrimSpace(query.TokenName); query.TokenName != "" {
		tx = tx.Where("token_name = ?", query.TokenName)
	}
	if query.TokenId > 0 {
		tx = tx.Where("token_id = ?", query.TokenId)
	}
	if query.ModelName = strings.TrimSpace(query.ModelName); query.ModelName != "" {
		tx = tx.Where("original_model LIKE ?", "%"+query.ModelName+"%")
	}
	if query.RuleName = strings.TrimSpace(query.RuleName); query.RuleName != "" {
		tx = tx.Where("rule_name LIKE ?", "%"+query.RuleName+"%")
	}
	if query.Action = strings.TrimSpace(query.Action); query.Action != "" {
		tx = tx.Where("action = ?", query.Action)
	}
	if query.SourceChannelId > 0 {
		tx = tx.Where("source_channel_id = ?", query.SourceChannelId)
	}
	if query.TargetChannelId > 0 {
		tx = tx.Where("target_channel_id = ?", query.TargetChannelId)
	}
	if query.SourceGroup = strings.TrimSpace(query.SourceGroup); query.SourceGroup != "" {
		tx = tx.Where("source_group = ?", query.SourceGroup)
	}
	if query.TargetGroup = strings.TrimSpace(query.TargetGroup); query.TargetGroup != "" {
		tx = tx.Where("target_group = ?", query.TargetGroup)
	}
	if query.StatusCode > 0 {
		tx = tx.Where("status_code = ?", query.StatusCode)
	}
	if query.ErrorCode = strings.TrimSpace(query.ErrorCode); query.ErrorCode != "" {
		tx = tx.Where("error_code = ?", query.ErrorCode)
	}
	if query.ErrorType = strings.TrimSpace(query.ErrorType); query.ErrorType != "" {
		tx = tx.Where("error_type = ?", query.ErrorType)
	}
	if query.FinalStatus = strings.TrimSpace(query.FinalStatus); query.FinalStatus != "" {
		tx = tx.Where("final_status = ?", query.FinalStatus)
	}
	if query.StartTimestamp > 0 {
		tx = tx.Where("created_at >= ?", query.StartTimestamp)
	}
	if query.EndTimestamp > 0 {
		tx = tx.Where("created_at <= ?", query.EndTimestamp)
	}
	return tx
}

func normalizeRetryRouteEventPage(page int, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	return page, pageSize
}

func retryRouteBreakdown(query RetryRouteEventQuery, column string) map[string]int64 {
	type row struct {
		Key   string `gorm:"column:key"`
		Total int64  `gorm:"column:total"`
	}
	rows := make([]row, 0)
	if err := applyRetryRouteEventFilters(DB.Model(&RetryRouteEvent{}), query).
		Select(column+" AS key, COUNT(*) AS total").
		Where(column+" <> ?", zeroRetryRouteBreakdownValue(column)).
		Group(column).
		Order("total desc").
		Limit(20).
		Scan(&rows).Error; err != nil {
		common.SysLog("failed to build retry route breakdown: " + err.Error())
		return map[string]int64{}
	}
	result := map[string]int64{}
	for _, item := range rows {
		result[item.Key] = item.Total
	}
	return result
}

func zeroRetryRouteBreakdownValue(column string) interface{} {
	switch column {
	case "source_channel_id", "target_channel_id":
		return 0
	default:
		return ""
	}
}

func trimRetryRouteEvent(event *RetryRouteEvent) {
	event.RequestId = strings.TrimSpace(event.RequestId)
	event.UpstreamRequestId = strings.TrimSpace(event.UpstreamRequestId)
	event.Username = strings.TrimSpace(event.Username)
	event.TokenName = strings.TrimSpace(event.TokenName)
	event.WorkspaceName = strings.TrimSpace(event.WorkspaceName)
	event.OriginalModel = strings.TrimSpace(event.OriginalModel)
	event.UpstreamModel = strings.TrimSpace(event.UpstreamModel)
	event.RequestPath = strings.TrimSpace(event.RequestPath)
	event.RuleSource = strings.TrimSpace(event.RuleSource)
	event.RuleName = strings.TrimSpace(event.RuleName)
	event.Action = strings.TrimSpace(event.Action)
	event.FinalStatus = strings.TrimSpace(event.FinalStatus)
	event.SourceChannelName = strings.TrimSpace(event.SourceChannelName)
	event.TargetChannelName = strings.TrimSpace(event.TargetChannelName)
	event.SourceGroup = strings.TrimSpace(event.SourceGroup)
	event.TargetGroup = strings.TrimSpace(event.TargetGroup)
	event.ErrorType = strings.TrimSpace(event.ErrorType)
	event.ErrorCode = strings.TrimSpace(event.ErrorCode)
	event.ErrorMessage = strings.TrimSpace(event.ErrorMessage)
	event.RequestHash = strings.TrimSpace(event.RequestHash)
	event.ResponseHash = strings.TrimSpace(event.ResponseHash)
}
