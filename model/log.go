package model

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
)

type Log struct {
	Id                int    `json:"id" gorm:"index:idx_created_at_id,priority:1;index:idx_user_id_id,priority:2"`
	UserId            int    `json:"user_id" gorm:"index;index:idx_user_id_id,priority:1"`
	CreatedAt         int64  `json:"created_at" gorm:"bigint;index:idx_created_at_id,priority:2;index:idx_created_at_type"`
	Type              int    `json:"type" gorm:"index:idx_created_at_type"`
	Content           string `json:"content"`
	Username          string `json:"username" gorm:"index;index:index_username_model_name,priority:2;default:''"`
	TokenName         string `json:"token_name" gorm:"index;default:''"`
	ModelName         string `json:"model_name" gorm:"index;index:index_username_model_name,priority:1;default:''"`
	Quota             int    `json:"quota" gorm:"default:0"`
	PromptTokens      int    `json:"prompt_tokens" gorm:"default:0"`
	CompletionTokens  int    `json:"completion_tokens" gorm:"default:0"`
	UseTime           int    `json:"use_time" gorm:"default:0"`
	IsStream          bool   `json:"is_stream"`
	ChannelId         int    `json:"channel" gorm:"index"`
	ChannelName       string `json:"channel_name" gorm:"->"`
	TokenId           int    `json:"token_id" gorm:"default:0;index"`
	Group             string `json:"group" gorm:"index"`
	Ip                string `json:"ip" gorm:"index;default:''"`
	RequestId         string `json:"request_id,omitempty" gorm:"type:varchar(64);index:idx_logs_request_id;default:''"`
	UpstreamRequestId string `json:"upstream_request_id,omitempty" gorm:"type:varchar(128);index:idx_logs_upstream_request_id;default:''"`
	Other             string `json:"other"`
}

func resolveTokenIDsForFilters(userId int, tokenName string, workspaceName string) ([]int, bool, error) {
	tokenName = strings.TrimSpace(tokenName)
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName == "" {
		return nil, false, nil
	}
	query := DB.Table("tokens").
		Select("tokens.id").
		Joins("JOIN workspaces ON workspaces.id = tokens.workspace_id").
		Where("workspaces.name = ?", workspaceName)
	if userId > 0 {
		query = query.Where("tokens.user_id = ?", userId)
	}
	if tokenName != "" {
		query = query.Where("tokens.name = ?", tokenName)
	}
	var ids []int
	if err := query.Pluck("tokens.id", &ids).Error; err != nil {
		return nil, false, err
	}
	return ids, true, nil
}

func applyTokenIDFilter(tx *gorm.DB, prefix string, tokenIDs []int, tokenIDsResolved bool) *gorm.DB {
	if !tokenIDsResolved {
		return tx
	}
	if len(tokenIDs) == 0 {
		return tx.Where("1 = 0")
	}
	return tx.Where(prefix+"token_id IN ?", tokenIDs)
}

// don't use iota, avoid change log type value
const (
	LogTypeUnknown = 0
	LogTypeTopup   = 1
	LogTypeConsume = 2
	LogTypeManage  = 3
	LogTypeSystem  = 4
	LogTypeError   = 5
	LogTypeRefund  = 6
)

func stripChannelCostFields(otherMap map[string]interface{}) {
	delete(otherMap, "channel_ratio")
	delete(otherMap, "cost_base_quota")
	delete(otherMap, "cost_quota")
	delete(otherMap, "profit_quota")
	delete(otherMap, "profit_rate")
}

func StripChannelCostFieldsFromLogs(logs []*Log) {
	for i := range logs {
		otherMap, _ := common.StrToMap(logs[i].Other)
		if otherMap == nil {
			continue
		}
		stripChannelCostFields(otherMap)
		logs[i].Other = common.MapToJsonStr(otherMap)
	}
}

func formatUserLogs(logs []*Log, startIdx int) {
	for i := range logs {
		logs[i].ChannelName = ""
		var otherMap map[string]interface{}
		otherMap, _ = common.StrToMap(logs[i].Other)
		if otherMap != nil {
			// Remove admin-only debug fields.
			delete(otherMap, "admin_info")
			// delete(otherMap, "reject_reason")
			delete(otherMap, "stream_status")
			stripChannelCostFields(otherMap)
		}
		logs[i].Other = common.MapToJsonStr(otherMap)
		logs[i].Id = startIdx + i + 1
	}
}

func GetLogByTokenId(tokenId int) (logs []*Log, err error) {
	err = LOG_DB.Model(&Log{}).Where("token_id = ?", tokenId).Order("id desc").Limit(common.MaxRecentItems).Find(&logs).Error
	formatUserLogs(logs, 0)
	return logs, err
}

func RecordLog(userId int, logType int, content string) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

// RecordLogWithAdminInfo 记录操作日志，并将管理员相关信息存入 Other.admin_info，
func RecordLogWithAdminInfo(userId int, logType int, content string, adminInfo map[string]interface{}) {
	if logType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(userId, false)
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      logType,
		Content:   content,
	}
	if len(adminInfo) > 0 {
		other := map[string]interface{}{
			"admin_info": adminInfo,
		}
		log.Other = common.MapToJsonStr(other)
	}
	if err := LOG_DB.Create(log).Error; err != nil {
		common.SysLog("failed to record log: " + err.Error())
	}
}

func RecordTopupLog(userId int, content string, callerIp string, paymentMethod string, callbackPaymentMethod string) {
	username, _ := GetUsernameById(userId, false)
	adminInfo := map[string]interface{}{
		"server_ip":               common.GetIp(),
		"node_name":               common.NodeName,
		"caller_ip":               callerIp,
		"payment_method":          paymentMethod,
		"callback_payment_method": callbackPaymentMethod,
		"version":                 common.Version,
	}
	other := map[string]interface{}{
		"admin_info": adminInfo,
	}
	log := &Log{
		UserId:    userId,
		Username:  username,
		CreatedAt: common.GetTimestamp(),
		Type:      LogTypeTopup,
		Content:   content,
		Ip:        callerIp,
		Other:     common.MapToJsonStr(other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record topup log: " + err.Error())
	}
}

func RecordErrorLog(c *gin.Context, userId int, channelId int, modelName string, tokenName string, content string, tokenId int, useTimeSeconds int,
	isStream bool, group string, other map[string]interface{}) {
	logger.LogInfo(c, fmt.Sprintf("record error log: userId=%d, channelId=%d, modelName=%s, tokenName=%s, content=%s", userId, channelId, modelName, tokenName, content))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(other)
	// 判断是否需要记录 IP
	needRecordIp := true
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		needRecordIp = settingMap.RecordIpLogEnabled()
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeError,
		Content:          content,
		PromptTokens:     0,
		CompletionTokens: 0,
		TokenName:        tokenName,
		ModelName:        modelName,
		Quota:            0,
		ChannelId:        channelId,
		TokenId:          tokenId,
		UseTime:          useTimeSeconds,
		IsStream:         isStream,
		Group:            group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
	}
}

type RecordConsumeLogParams struct {
	ChannelId        int                    `json:"channel_id"`
	PromptTokens     int                    `json:"prompt_tokens"`
	CompletionTokens int                    `json:"completion_tokens"`
	ModelName        string                 `json:"model_name"`
	TokenName        string                 `json:"token_name"`
	Quota            int                    `json:"quota"`
	Content          string                 `json:"content"`
	TokenId          int                    `json:"token_id"`
	UseTimeSeconds   int                    `json:"use_time_seconds"`
	IsStream         bool                   `json:"is_stream"`
	Group            string                 `json:"group"`
	Other            map[string]interface{} `json:"other"`
}

func RecordConsumeLog(c *gin.Context, userId int, params RecordConsumeLogParams) int {
	if !common.LogConsumeEnabled {
		return 0
	}
	logger.LogInfo(c, fmt.Sprintf("record consume log: userId=%d, params=%s", userId, common.GetJsonString(params)))
	username := c.GetString("username")
	requestId := c.GetString(common.RequestIdKey)
	upstreamRequestId := c.GetString(common.UpstreamRequestIdKey)
	otherStr := common.MapToJsonStr(params.Other)
	// 判断是否需要记录 IP
	needRecordIp := true
	if settingMap, err := GetUserSetting(userId, false); err == nil {
		needRecordIp = settingMap.RecordIpLogEnabled()
	}
	log := &Log{
		UserId:           userId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             LogTypeConsume,
		Content:          params.Content,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		TokenName:        params.TokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		UseTime:          params.UseTimeSeconds,
		IsStream:         params.IsStream,
		Group:            params.Group,
		Ip: func() string {
			if needRecordIp {
				return c.ClientIP()
			}
			return ""
		}(),
		RequestId:         requestId,
		UpstreamRequestId: upstreamRequestId,
		Other:             otherStr,
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		logger.LogError(c, "failed to record log: "+err.Error())
		return 0
	}
	if common.DataExportEnabled {
		gopool.Go(func() {
			LogQuotaData(QuotaDataLogParams{
				UserID:    userId,
				Username:  username,
				ModelName: params.ModelName,
				Quota:     params.Quota,
				CreatedAt: common.GetTimestamp(),
				TokenUsed: params.PromptTokens + params.CompletionTokens,
				TokenID:   params.TokenId,
				UseGroup:  params.Group,
				ChannelID: params.ChannelId,
				NodeName:  common.NodeName,
			})
		})
	}
	gopool.Go(func() {
		if _, err := RecordBillingV2ConsumeLog(log.Id); err != nil {
			common.SysLog("failed to record billing v2 consume log: " + err.Error())
		}
	})
	return log.Id
}

type RecordTaskBillingLogParams struct {
	UserId           int
	LogType          int
	Content          string
	ChannelId        int
	ModelName        string
	Quota            int
	PromptTokens     int
	CompletionTokens int
	TokenId          int
	Group            string
	Other            map[string]interface{}
}

func RecordTaskBillingLog(params RecordTaskBillingLogParams) {
	if params.LogType == LogTypeConsume && !common.LogConsumeEnabled {
		return
	}
	username, _ := GetUsernameById(params.UserId, false)
	tokenName := ""
	if params.TokenId > 0 {
		if token, err := GetTokenById(params.TokenId); err == nil {
			tokenName = token.Name
		}
	}
	log := &Log{
		UserId:           params.UserId,
		Username:         username,
		CreatedAt:        common.GetTimestamp(),
		Type:             params.LogType,
		Content:          params.Content,
		TokenName:        tokenName,
		ModelName:        params.ModelName,
		Quota:            params.Quota,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		ChannelId:        params.ChannelId,
		TokenId:          params.TokenId,
		Group:            params.Group,
		Other:            common.MapToJsonStr(params.Other),
	}
	err := LOG_DB.Create(log).Error
	if err != nil {
		common.SysLog("failed to record task billing log: " + err.Error())
	}
}

func dedupeRepeatedErrorLogsByRequestId(tx *gorm.DB, applyFilters func(*gorm.DB, string) *gorm.DB) *gorm.DB {
	newer := LOG_DB.Table("logs AS newer").Select("1")
	newer = applyFilters(newer, "newer")
	newer = newer.
		Where("newer.type = ?", LogTypeError).
		Where("newer.request_id = logs.request_id").
		Where("newer.id > logs.id").
		Limit(1)

	return tx.Where("NOT (logs.type = ? AND logs.request_id <> '' AND EXISTS (?))", LogTypeError, newer)
}

func GetAllLogs(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, startIdx int, num int, channel int, group string, requestId string, upstreamRequestId string, workspaceName ...string) (logs []*Log, total int64, err error) {
	workspace := ""
	if len(workspaceName) > 0 {
		workspace = workspaceName[0]
	}
	tokenIDs, tokenIDsResolved, err := resolveTokenIDsForFilters(0, tokenName, workspace)
	if err != nil {
		return nil, 0, err
	}
	applyFilters := func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = applyTokenIDFilter(tx, prefix, tokenIDs, tokenIDsResolved)
		if logType != LogTypeUnknown {
			tx = tx.Where(prefix+"type = ?", logType)
		}
		if modelName != "" {
			tx = tx.Where(prefix+"model_name like ?", modelName)
		}
		if username != "" {
			tx = tx.Where(prefix+"username = ?", username)
		}
		if tokenName != "" {
			tx = tx.Where(prefix+"token_name = ?", tokenName)
		}
		if requestId != "" {
			tx = tx.Where(prefix+"request_id = ?", requestId)
		}
		if upstreamRequestId != "" {
			tx = tx.Where(prefix+"upstream_request_id = ?", upstreamRequestId)
		}
		if startTimestamp != 0 {
			tx = tx.Where(prefix+"created_at >= ?", startTimestamp)
		}
		if endTimestamp != 0 {
			tx = tx.Where(prefix+"created_at <= ?", endTimestamp)
		}
		if channel != 0 {
			tx = tx.Where(prefix+"channel_id = ?", channel)
		}
		if group != "" {
			tx = tx.Where(prefix+logGroupCol+" = ?", group)
		}
		return tx
	}
	tx := applyFilters(LOG_DB.Model(&Log{}), "logs")
	tx = dedupeRepeatedErrorLogsByRequestId(tx, applyFilters)
	err = tx.Model(&Log{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	channelIds := types.NewSet[int]()
	for _, log := range logs {
		if log.ChannelId != 0 {
			channelIds.Add(log.ChannelId)
		}
	}

	if channelIds.Len() > 0 {
		var channels []struct {
			Id   int    `gorm:"column:id"`
			Name string `gorm:"column:name"`
		}
		if common.MemoryCacheEnabled {
			// Cache get channel
			for _, channelId := range channelIds.Items() {
				if cacheChannel, err := CacheGetChannel(channelId); err == nil {
					channels = append(channels, struct {
						Id   int    `gorm:"column:id"`
						Name string `gorm:"column:name"`
					}{
						Id:   channelId,
						Name: cacheChannel.Name,
					})
				}
			}
		} else {
			// Bulk query channels from DB
			if err = DB.Table("channels").Select("id, name").Where("id IN ?", channelIds.Items()).Find(&channels).Error; err != nil {
				return logs, total, err
			}
		}
		channelMap := make(map[int]string, len(channels))
		for _, channel := range channels {
			channelMap[channel.Id] = channel.Name
		}
		for i := range logs {
			logs[i].ChannelName = channelMap[logs[i].ChannelId]
		}
	}

	return logs, total, err
}

const logSearchCountLimit = 10000

func GetUserLogs(userId int, logType int, startTimestamp int64, endTimestamp int64, modelName string, tokenName string, startIdx int, num int, group string, requestId string, upstreamRequestId string, workspaceName ...string) (logs []*Log, total int64, err error) {
	var modelNamePattern string
	if modelName != "" {
		modelNamePattern, err = sanitizeLikePattern(modelName)
		if err != nil {
			return nil, 0, err
		}
	}
	workspace := ""
	if len(workspaceName) > 0 {
		workspace = workspaceName[0]
	}
	tokenIDs, tokenIDsResolved, err := resolveTokenIDsForFilters(userId, tokenName, workspace)
	if err != nil {
		return nil, 0, err
	}
	applyFilters := func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.Where(prefix+"user_id = ?", userId)
		tx = applyTokenIDFilter(tx, prefix, tokenIDs, tokenIDsResolved)
		if logType != LogTypeUnknown {
			tx = tx.Where(prefix+"type = ?", logType)
		}
		if modelName != "" {
			tx = tx.Where(prefix+"model_name LIKE ? ESCAPE '!'", modelNamePattern)
		}
		if tokenName != "" {
			tx = tx.Where(prefix+"token_name = ?", tokenName)
		}
		if requestId != "" {
			tx = tx.Where(prefix+"request_id = ?", requestId)
		}
		if upstreamRequestId != "" {
			tx = tx.Where(prefix+"upstream_request_id = ?", upstreamRequestId)
		}
		if startTimestamp != 0 {
			tx = tx.Where(prefix+"created_at >= ?", startTimestamp)
		}
		if endTimestamp != 0 {
			tx = tx.Where(prefix+"created_at <= ?", endTimestamp)
		}
		if group != "" {
			tx = tx.Where(prefix+logGroupCol+" = ?", group)
		}
		return tx
	}
	tx := applyFilters(LOG_DB.Model(&Log{}), "logs")
	tx = dedupeRepeatedErrorLogsByRequestId(tx, applyFilters)
	err = tx.Model(&Log{}).Limit(logSearchCountLimit).Count(&total).Error
	if err != nil {
		common.SysError("failed to count user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}
	err = tx.Order("logs.id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		common.SysError("failed to search user logs: " + err.Error())
		return nil, 0, errors.New("查询日志失败")
	}

	formatUserLogs(logs, startIdx)
	return logs, total, err
}

type Stat struct {
	Quota       int `json:"quota"`
	Rpm         int `json:"rpm"`
	Tpm         int `json:"tpm"`
	BaseQuota   int `json:"base_quota"`
	CostQuota   int `json:"cost_quota"`
	ProfitQuota int `json:"profit_quota"`
}

type ModelProfitStat struct {
	ModelName    string `json:"model_name"`
	RequestCount int    `json:"request_count"`
	Quota        int    `json:"quota"`
	BaseQuota    int    `json:"base_quota"`
	CostQuota    int    `json:"cost_quota"`
	ProfitQuota  int    `json:"profit_quota"`
}

type ModelProfitStatsSummary struct {
	Quota       int               `json:"quota"`
	BaseQuota   int               `json:"base_quota"`
	CostQuota   int               `json:"cost_quota"`
	ProfitQuota int               `json:"profit_quota"`
	Items       []ModelProfitStat `json:"items"`
}

type SubscriptionKeyUsagePoint struct {
	Date     string `json:"date"`
	Label    string `json:"label"`
	Requests int    `json:"requests"`
	Quota    int    `json:"quota"`
	Tokens   int    `json:"tokens"`
}

type SubscriptionKeyUsageStats struct {
	Days          int                         `json:"days"`
	TotalRequests int                         `json:"total_requests"`
	TodayRequests int                         `json:"today_requests"`
	TotalQuota    int                         `json:"total_quota"`
	TotalTokens   int                         `json:"total_tokens"`
	Points        []SubscriptionKeyUsagePoint `json:"points"`
}

type subscriptionUsageLogRow struct {
	CreatedAt        int64
	Quota            int
	PromptTokens     int
	CompletionTokens int
}

func GetSubscriptionKeyUsageStats(userId int, tokenId int, days int, now time.Time) (SubscriptionKeyUsageStats, error) {
	if days <= 0 {
		days = 7
	}
	if days > 30 {
		days = 30
	}
	if now.IsZero() {
		now = time.Now()
	}
	anchor := now.UTC()
	end := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 23, 59, 59, 0, time.UTC)
	startDay := end.AddDate(0, 0, -(days - 1))
	start := time.Date(startDay.Year(), startDay.Month(), startDay.Day(), 0, 0, 0, 0, time.UTC)
	todayStart := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), 0, 0, 0, 0, time.UTC)

	stats := SubscriptionKeyUsageStats{
		Days:   days,
		Points: make([]SubscriptionKeyUsagePoint, 0, days),
	}
	pointByDate := make(map[string]*SubscriptionKeyUsagePoint, days)
	for i := 0; i < days; i++ {
		day := start.AddDate(0, 0, i)
		date := day.Format("2006-01-02")
		point := SubscriptionKeyUsagePoint{
			Date:  date,
			Label: day.Format("01-02"),
		}
		stats.Points = append(stats.Points, point)
		pointByDate[date] = &stats.Points[len(stats.Points)-1]
	}

	var rows []subscriptionUsageLogRow
	if err := LOG_DB.Model(&Log{}).
		Select("created_at", "quota", "prompt_tokens", "completion_tokens").
		Where("user_id = ? AND token_id = ? AND type = ? AND created_at >= ? AND created_at <= ?", userId, tokenId, LogTypeConsume, start.Unix(), end.Unix()).
		Find(&rows).Error; err != nil {
		return stats, err
	}
	for _, row := range rows {
		t := time.Unix(row.CreatedAt, 0).UTC()
		date := t.Format("2006-01-02")
		point, ok := pointByDate[date]
		if !ok {
			continue
		}
		tokens := row.PromptTokens + row.CompletionTokens
		point.Requests++
		point.Quota += row.Quota
		point.Tokens += tokens
		stats.TotalRequests++
		stats.TotalQuota += row.Quota
		stats.TotalTokens += tokens
		if row.CreatedAt >= todayStart.Unix() {
			stats.TodayRequests++
		}
	}
	return stats, nil
}

func logOtherNumber(other map[string]interface{}, key string) (float64, bool) {
	if other == nil {
		return 0, false
	}
	value, ok := other[key]
	if !ok {
		return 0, false
	}
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func logCostSnapshot(quota int, otherText string) (baseQuota int, costQuota int, profitQuota int) {
	other, _ := common.StrToMap(otherText)
	if base, ok := logOtherNumber(other, "cost_base_quota"); ok {
		baseQuota = int(math.Round(base))
	} else {
		groupRatio, ok := logOtherNumber(other, "group_ratio")
		if ok && groupRatio > 0 {
			baseQuota = int(math.Round(float64(quota) / groupRatio))
		} else {
			baseQuota = quota
		}
	}

	if cost, ok := logOtherNumber(other, "cost_quota"); ok {
		costQuota = int(math.Round(cost))
	} else {
		channelRatio, ok := logOtherNumber(other, "channel_ratio")
		if !ok || channelRatio == 0 {
			channelRatio = 1
		}
		costQuota = int(math.Round(float64(baseQuota) * channelRatio))
	}
	if profit, ok := logOtherNumber(other, "profit_quota"); ok {
		profitQuota = int(math.Round(profit))
	} else {
		profitQuota = quota - costQuota
	}
	return baseQuota, costQuota, profitQuota
}

func usageStatLogTypes(logType int) []int {
	switch logType {
	case LogTypeUnknown:
		return []int{LogTypeConsume, LogTypeRefund}
	case LogTypeConsume, LogTypeRefund:
		return []int{logType}
	default:
		return nil
	}
}

func usageStatSign(logType int) int {
	if logType == LogTypeRefund {
		return -1
	}
	return 1
}

func SumUsedQuota(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, workspaceName ...string) (stat Stat, err error) {
	statTypes := usageStatLogTypes(logType)
	if len(statTypes) == 0 {
		return stat, nil
	}

	// 为rpm和tpm创建单独的查询
	rpmTpmQuery := LOG_DB.Table("logs").Select("count(*) rpm, sum(prompt_tokens) + sum(completion_tokens) tpm")
	costQuery := LOG_DB.Table("logs").Select("type, quota, other")
	workspace := ""
	if len(workspaceName) > 0 {
		workspace = workspaceName[0]
	}
	tokenIDs, tokenIDsResolved, err := resolveTokenIDsForFilters(0, tokenName, workspace)
	if err != nil {
		return stat, err
	}
	rpmTpmQuery = applyTokenIDFilter(rpmTpmQuery, "", tokenIDs, tokenIDsResolved)
	costQuery = applyTokenIDFilter(costQuery, "", tokenIDs, tokenIDsResolved)

	if username != "" {
		rpmTpmQuery = rpmTpmQuery.Where("username = ?", username)
		costQuery = costQuery.Where("username = ?", username)
	}
	if tokenName != "" {
		rpmTpmQuery = rpmTpmQuery.Where("token_name = ?", tokenName)
		costQuery = costQuery.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", startTimestamp)
		costQuery = costQuery.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		rpmTpmQuery = rpmTpmQuery.Where("created_at <= ?", endTimestamp)
		costQuery = costQuery.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		modelNamePattern, err := sanitizeLikePattern(modelName)
		if err != nil {
			return stat, err
		}
		rpmTpmQuery = rpmTpmQuery.Where("model_name LIKE ? ESCAPE '!'", modelNamePattern)
		costQuery = costQuery.Where("model_name LIKE ? ESCAPE '!'", modelNamePattern)
	}
	if channel != 0 {
		rpmTpmQuery = rpmTpmQuery.Where("channel_id = ?", channel)
		costQuery = costQuery.Where("channel_id = ?", channel)
	}
	if group != "" {
		rpmTpmQuery = rpmTpmQuery.Where(logGroupCol+" = ?", group)
		costQuery = costQuery.Where(logGroupCol+" = ?", group)
	}

	rpmTpmQuery = rpmTpmQuery.Where("type = ?", LogTypeConsume)
	costQuery = costQuery.Where("type IN ?", statTypes)

	// 只统计最近60秒的rpm和tpm
	rpmTpmQuery = rpmTpmQuery.Where("created_at >= ?", time.Now().Add(-60*time.Second).Unix())

	// 执行查询
	if logType == LogTypeUnknown || logType == LogTypeConsume {
		if err := rpmTpmQuery.Scan(&stat).Error; err != nil {
			common.SysError("failed to query rpm/tpm stat: " + err.Error())
			return stat, errors.New("查询统计数据失败")
		}
	}
	var costRows []struct {
		Type  int
		Quota int
		Other string
	}
	if err := costQuery.Scan(&costRows).Error; err != nil {
		common.SysError("failed to query cost stat: " + err.Error())
		return stat, errors.New("查询统计数据失败")
	}
	for _, row := range costRows {
		sign := usageStatSign(row.Type)
		baseQuota, costQuota, profitQuota := logCostSnapshot(row.Quota, row.Other)
		stat.Quota += sign * row.Quota
		stat.BaseQuota += sign * baseQuota
		stat.CostQuota += sign * costQuota
		stat.ProfitQuota += sign * profitQuota
	}

	return stat, nil
}

func SumModelProfitStats(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string, channel int, group string, workspaceName ...string) (summary ModelProfitStatsSummary, err error) {
	query := LOG_DB.Table("logs").Select("model_name, quota, other")
	workspace := ""
	if len(workspaceName) > 0 {
		workspace = workspaceName[0]
	}
	tokenIDs, tokenIDsResolved, err := resolveTokenIDsForFilters(0, tokenName, workspace)
	if err != nil {
		return summary, err
	}
	query = applyTokenIDFilter(query, "", tokenIDs, tokenIDsResolved)

	if username != "" {
		query = query.Where("username = ?", username)
	}
	if tokenName != "" {
		query = query.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		query = query.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		query = query.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		modelNamePattern, err := sanitizeLikePattern(modelName)
		if err != nil {
			return summary, err
		}
		query = query.Where("model_name LIKE ? ESCAPE '!'", modelNamePattern)
	}
	if channel != 0 {
		query = query.Where("channel_id = ?", channel)
	}
	if group != "" {
		query = query.Where(logGroupCol+" = ?", group)
	}
	query = query.Where("type = ?", LogTypeConsume)

	var rows []struct {
		ModelName string
		Quota     int
		Other     string
	}
	if err := query.Scan(&rows).Error; err != nil {
		common.SysError("failed to query model profit stats: " + err.Error())
		return summary, errors.New("查询统计数据失败")
	}

	itemByModel := make(map[string]*ModelProfitStat)
	for _, row := range rows {
		model := strings.TrimSpace(row.ModelName)
		if model == "" {
			model = "unknown"
		}
		item := itemByModel[model]
		if item == nil {
			item = &ModelProfitStat{ModelName: model}
			itemByModel[model] = item
		}
		baseQuota, costQuota, profitQuota := logCostSnapshot(row.Quota, row.Other)
		item.RequestCount++
		item.Quota += row.Quota
		item.BaseQuota += baseQuota
		item.CostQuota += costQuota
		item.ProfitQuota += profitQuota
		summary.Quota += row.Quota
		summary.BaseQuota += baseQuota
		summary.CostQuota += costQuota
		summary.ProfitQuota += profitQuota
	}

	summary.Items = make([]ModelProfitStat, 0, len(itemByModel))
	for _, item := range itemByModel {
		summary.Items = append(summary.Items, *item)
	}
	sort.Slice(summary.Items, func(i, j int) bool {
		if summary.Items[i].ProfitQuota == summary.Items[j].ProfitQuota {
			return summary.Items[i].Quota > summary.Items[j].Quota
		}
		return summary.Items[i].ProfitQuota > summary.Items[j].ProfitQuota
	})
	return summary, nil
}

func GetQuotaDatesFromLogs(startTime int64, endTime int64, username string, tokenName string, workspaceName string, userId int) (quotaData []*QuotaData, err error) {
	tokenIDs, tokenIDsResolved, err := resolveTokenIDsForFilters(userId, tokenName, workspaceName)
	if err != nil {
		return nil, err
	}
	tx := LOG_DB.Table("logs").
		Select("model_name, sum(1) as count, sum(quota) as quota, sum(prompt_tokens) + sum(completion_tokens) as token_used, created_at").
		Where("type = ?", LogTypeConsume)
	tx = applyTokenIDFilter(tx, "", tokenIDs, tokenIDsResolved)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTime != 0 {
		tx = tx.Where("created_at >= ?", startTime)
	}
	if endTime != 0 {
		tx = tx.Where("created_at <= ?", endTime)
	}
	err = tx.Group("model_name, created_at").Find(&quotaData).Error
	return quotaData, err
}

func GetUsageDimensionTrendsFromLogs(startTime int64, endTime int64, username string, tokenName string, workspaceName string, userId int) (trendData []*UsageDimensionTrendData, err error) {
	tokenIDs, tokenIDsResolved, err := resolveTokenIDsForFilters(userId, tokenName, workspaceName)
	if err != nil {
		return nil, err
	}
	tx := LOG_DB.Table("logs").
		Select("token_id, token_name, sum(1) as count, sum(quota) as quota, sum(prompt_tokens) + sum(completion_tokens) as token_used, created_at").
		Where("type = ?", LogTypeConsume)
	tx = applyTokenIDFilter(tx, "", tokenIDs, tokenIDsResolved)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTime != 0 {
		tx = tx.Where("created_at >= ?", startTime)
	}
	if endTime != 0 {
		tx = tx.Where("created_at <= ?", endTime)
	}
	if err = tx.Group("token_id, token_name, created_at").Find(&trendData).Error; err != nil {
		return nil, err
	}
	enrichUsageDimensionTrendWorkspaces(trendData)
	return trendData, nil
}

func enrichUsageDimensionTrendWorkspaces(trendData []*UsageDimensionTrendData) {
	if len(trendData) == 0 {
		return
	}
	tokenIdSet := make(map[int]struct{})
	for _, item := range trendData {
		if item != nil && item.TokenId > 0 {
			tokenIdSet[item.TokenId] = struct{}{}
		}
	}
	if len(tokenIdSet) == 0 {
		return
	}
	tokenIds := make([]int, 0, len(tokenIdSet))
	for tokenId := range tokenIdSet {
		tokenIds = append(tokenIds, tokenId)
	}
	var tokens []Token
	if err := DB.Select("id", "workspace_id").Where("id IN ?", tokenIds).Find(&tokens).Error; err != nil {
		common.SysError("failed to query usage trend tokens: " + err.Error())
		return
	}
	tokenWorkspaceIds := make(map[int]int, len(tokens))
	workspaceIdSet := make(map[int]struct{})
	for _, token := range tokens {
		tokenWorkspaceIds[token.Id] = token.WorkspaceId
		if token.WorkspaceId > 0 {
			workspaceIdSet[token.WorkspaceId] = struct{}{}
		}
	}
	if len(workspaceIdSet) == 0 {
		return
	}
	workspaceIds := make([]int, 0, len(workspaceIdSet))
	for workspaceId := range workspaceIdSet {
		workspaceIds = append(workspaceIds, workspaceId)
	}
	var workspaces []Workspace
	if err := DB.Select("id", "name").Where("id IN ?", workspaceIds).Find(&workspaces).Error; err != nil {
		common.SysError("failed to query usage trend workspaces: " + err.Error())
		return
	}
	workspaceNames := make(map[int]string, len(workspaces))
	for _, workspace := range workspaces {
		workspaceNames[workspace.Id] = workspace.Name
	}
	for _, item := range trendData {
		if item == nil {
			continue
		}
		workspaceId := tokenWorkspaceIds[item.TokenId]
		item.WorkspaceId = workspaceId
		item.WorkspaceName = workspaceNames[workspaceId]
	}
}

func SumUsedToken(logType int, startTimestamp int64, endTimestamp int64, modelName string, username string, tokenName string) (token int) {
	tx := LOG_DB.Table("logs").Select("ifnull(sum(prompt_tokens),0) + ifnull(sum(completion_tokens),0)")
	if username != "" {
		tx = tx.Where("username = ?", username)
	}
	if tokenName != "" {
		tx = tx.Where("token_name = ?", tokenName)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}
	if modelName != "" {
		tx = tx.Where("model_name = ?", modelName)
	}
	tx.Where("type = ?", LogTypeConsume).Scan(&token)
	return token
}

func DeleteOldLog(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	var total int64 = 0

	for {
		if nil != ctx.Err() {
			return total, ctx.Err()
		}

		result := LOG_DB.Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
		if nil != result.Error {
			return total, result.Error
		}

		total += result.RowsAffected

		if result.RowsAffected < int64(limit) {
			break
		}
	}

	return total, nil
}

func CountOldLog(ctx context.Context, targetTimestamp int64) (int64, error) {
	var total int64
	if err := LOG_DB.WithContext(ctx).Model(&Log{}).Where("created_at < ?", targetTimestamp).Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func DeleteOldLogBatch(ctx context.Context, targetTimestamp int64, limit int) (int64, error) {
	if limit <= 0 {
		limit = 100
	}
	if nil != ctx.Err() {
		return 0, ctx.Err()
	}

	result := LOG_DB.WithContext(ctx).Where("created_at < ?", targetTimestamp).Limit(limit).Delete(&Log{})
	if nil != result.Error {
		return 0, result.Error
	}

	return result.RowsAffected, nil
}
