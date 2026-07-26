package model

import (
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const agentAnalyticsUserChunkSize = 400

type AgentAnalyticsSummary struct {
	TotalUsers         int64   `json:"total_users"`
	ActiveUsers        int64   `json:"active_users"`
	NewUsers           int64   `json:"new_users"`
	UsageUsers         int64   `json:"usage_users"`
	TotalRequests      int64   `json:"total_requests"`
	SuccessfulRequests int64   `json:"successful_requests"`
	FailedRequests     int64   `json:"failed_requests"`
	TotalTokens        int64   `json:"total_tokens"`
	TotalQuota         int64   `json:"total_quota"`
	ProfitQuota        int64   `json:"profit_quota"`
	ProfitAmount       float64 `json:"profit_amount"`
	Currency           string  `json:"currency"`
	SuccessRate        float64 `json:"success_rate"`
}

type AgentAnalyticsTrendPoint struct {
	Timestamp int64 `json:"timestamp"`
	Requests  int64 `json:"requests"`
	Errors    int64 `json:"errors"`
	Tokens    int64 `json:"tokens"`
	Quota     int64 `json:"quota"`
}

type AgentAnalyticsModelItem struct {
	ModelName string `json:"model_name"`
	Requests  int64  `json:"requests"`
	Tokens    int64  `json:"tokens"`
	Quota     int64  `json:"quota"`
}

type AgentAnalyticsUserItem struct {
	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Requests int64  `json:"requests"`
	Errors   int64  `json:"errors"`
	Tokens   int64  `json:"tokens"`
	Quota    int64  `json:"quota"`
}

type AgentAnalyticsLogItem struct {
	Id               int    `json:"id"`
	UserId           int    `json:"user_id"`
	Username         string `json:"username"`
	CreatedAt        int64  `json:"created_at"`
	Type             int    `json:"type"`
	TokenName        string `json:"token_name"`
	ModelName        string `json:"model_name"`
	Quota            int    `json:"quota"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	UseTime          int    `json:"use_time"`
	IsStream         bool   `json:"is_stream"`
	Group            string `json:"group"`
	RequestId        string `json:"request_id,omitempty"`
	StatusCode       int    `json:"status_code,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	Other            string `json:"-"`
}

type agentAnalyticsLogOther struct {
	StatusCode int `json:"status_code"`
}

func hydrateAgentAnalyticsLogFailures(logs []AgentAnalyticsLogItem) {
	for i := range logs {
		log := &logs[i]
		if log.Type != LogTypeError {
			log.ErrorMessage = ""
			log.Other = ""
			continue
		}

		if log.Other != "" {
			var other agentAnalyticsLogOther
			if err := common.UnmarshalJsonStr(log.Other, &other); err == nil {
				log.StatusCode = other.StatusCode
			}
		}
		log.Other = ""

		message := strings.TrimSpace(log.ErrorMessage)
		if !strings.HasPrefix(message, "status_code=") {
			log.ErrorMessage = message
			continue
		}

		statusText, remainder, hasMessage := strings.Cut(message, ",")
		status, err := strconv.Atoi(strings.TrimPrefix(statusText, "status_code="))
		if err != nil {
			log.ErrorMessage = message
			continue
		}
		if log.StatusCode == 0 {
			log.StatusCode = status
		}
		if hasMessage {
			log.ErrorMessage = strings.TrimSpace(remainder)
		} else {
			log.ErrorMessage = ""
		}
	}
}

type AgentAnalytics struct {
	StartTimestamp int64                      `json:"start_timestamp"`
	EndTimestamp   int64                      `json:"end_timestamp"`
	BucketSeconds  int64                      `json:"bucket_seconds"`
	Summary        AgentAnalyticsSummary      `json:"summary"`
	Trend          []AgentAnalyticsTrendPoint `json:"trend"`
	TopModels      []AgentAnalyticsModelItem  `json:"top_models"`
	TopUsers       []AgentAnalyticsUserItem   `json:"top_users"`
	RecentLogs     []AgentAnalyticsLogItem    `json:"recent_logs"`
}

type AgentAnalyticsLogFilters struct {
	StartTimestamp int64
	EndTimestamp   int64
	Type           int
	Username       string
	ModelName      string
	RequestId      string
}

func listAgentAnalyticsUserIDs(agentID int) ([]int, error) {
	var userIDs []int
	err := DB.Model(&AgentUser{}).
		Where("agent_id = ?", agentID).
		Order("user_id ASC").
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

func forEachAgentUserChunk(userIDs []int, fn func([]int) error) error {
	for start := 0; start < len(userIDs); start += agentAnalyticsUserChunkSize {
		end := start + agentAnalyticsUserChunkSize
		if end > len(userIDs) {
			end = len(userIDs)
		}
		if err := fn(userIDs[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func agentAnalyticsLogQuery(userIDs []int, startTimestamp, endTimestamp int64) *gorm.DB {
	return LOG_DB.Model(&Log{}).
		Where("user_id IN ?", userIDs).
		Where("created_at >= ? AND created_at <= ?", startTimestamp, endTimestamp).
		Where("type IN ?", []int{LogTypeConsume, LogTypeError})
}

func agentAnalyticsBucketExpression() string {
	if LOG_DB != nil && LOG_DB.Dialector.Name() == "mysql" {
		return "(created_at DIV ?) * ?"
	}
	return "(created_at / ?) * ?"
}

func GetAgentAnalytics(agentID int, startTimestamp, endTimestamp, bucketSeconds int64) (*AgentAnalytics, error) {
	result := &AgentAnalytics{
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		BucketSeconds:  bucketSeconds,
		Trend:          make([]AgentAnalyticsTrendPoint, 0),
		TopModels:      make([]AgentAnalyticsModelItem, 0),
		TopUsers:       make([]AgentAnalyticsUserItem, 0),
		RecentLogs:     make([]AgentAnalyticsLogItem, 0),
	}

	if err := DB.Model(&AgentUser{}).Where("agent_id = ?", agentID).Count(&result.Summary.TotalUsers).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&AgentUser{}).
		Where("agent_id = ? AND status = ?", agentID, AgentUserStatusEnabled).
		Count(&result.Summary.ActiveUsers).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&AgentUser{}).
		Where("agent_id = ? AND created_at >= ? AND created_at <= ?", agentID, startTimestamp, endTimestamp).
		Count(&result.Summary.NewUsers).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&AgentLedger{}).
		Where("agent_id = ? AND type = ? AND created_at >= ? AND created_at <= ?", agentID, AgentLedgerTypeConsumeProfit, startTimestamp, endTimestamp).
		Select("COALESCE(SUM(profit_quota), 0)").
		Scan(&result.Summary.ProfitQuota).Error; err != nil {
		return nil, err
	}

	userIDs, err := listAgentAnalyticsUserIDs(agentID)
	if err != nil || len(userIDs) == 0 {
		return result, err
	}

	type summaryRow struct {
		Type   int
		Count  int64
		Tokens int64
		Quota  int64
	}
	type trendRow struct {
		Bucket int64
		Type   int
		Count  int64
		Tokens int64
		Quota  int64
	}
	type modelRow struct {
		ModelName string
		Requests  int64
		Tokens    int64
		Quota     int64
	}
	type userRow struct {
		UserId   int
		Username string
		Requests int64
		Errors   int64
		Tokens   int64
		Quota    int64
	}

	trendMap := make(map[int64]*AgentAnalyticsTrendPoint)
	modelMap := make(map[string]*AgentAnalyticsModelItem)
	userMap := make(map[int]*AgentAnalyticsUserItem)
	recentLogs := make([]AgentAnalyticsLogItem, 0)

	err = forEachAgentUserChunk(userIDs, func(chunk []int) error {
		var summaries []summaryRow
		if err := agentAnalyticsLogQuery(chunk, startTimestamp, endTimestamp).
			Select("type, COUNT(*) AS count, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS tokens, COALESCE(SUM(quota), 0) AS quota").
			Group("type").Scan(&summaries).Error; err != nil {
			return err
		}
		for _, row := range summaries {
			if row.Type == LogTypeConsume {
				result.Summary.SuccessfulRequests += row.Count
				result.Summary.TotalTokens += row.Tokens
				result.Summary.TotalQuota += row.Quota
			} else if row.Type == LogTypeError {
				result.Summary.FailedRequests += row.Count
			}
		}

		var trends []trendRow
		if err := agentAnalyticsLogQuery(chunk, startTimestamp, endTimestamp).
			Select(agentAnalyticsBucketExpression()+" AS bucket, type, COUNT(*) AS count, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS tokens, COALESCE(SUM(quota), 0) AS quota", bucketSeconds, bucketSeconds).
			Group("bucket, type").Scan(&trends).Error; err != nil {
			return err
		}
		for _, row := range trends {
			point := trendMap[row.Bucket]
			if point == nil {
				point = &AgentAnalyticsTrendPoint{Timestamp: row.Bucket}
				trendMap[row.Bucket] = point
			}
			if row.Type == LogTypeConsume {
				point.Requests += row.Count
				point.Tokens += row.Tokens
				point.Quota += row.Quota
			} else if row.Type == LogTypeError {
				point.Errors += row.Count
			}
		}

		var models []modelRow
		if err := agentAnalyticsLogQuery(chunk, startTimestamp, endTimestamp).
			Where("type = ? AND model_name <> ''", LogTypeConsume).
			Select("model_name, COUNT(*) AS requests, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS tokens, COALESCE(SUM(quota), 0) AS quota").
			Group("model_name").Scan(&models).Error; err != nil {
			return err
		}
		for _, row := range models {
			item := modelMap[row.ModelName]
			if item == nil {
				item = &AgentAnalyticsModelItem{ModelName: row.ModelName}
				modelMap[row.ModelName] = item
			}
			item.Requests += row.Requests
			item.Tokens += row.Tokens
			item.Quota += row.Quota
		}

		var users []userRow
		if err := agentAnalyticsLogQuery(chunk, startTimestamp, endTimestamp).
			Select("user_id, MAX(username) AS username, SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) AS requests, SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) AS errors, COALESCE(SUM(CASE WHEN type = ? THEN prompt_tokens + completion_tokens ELSE 0 END), 0) AS tokens, COALESCE(SUM(CASE WHEN type = ? THEN quota ELSE 0 END), 0) AS quota", LogTypeConsume, LogTypeError, LogTypeConsume, LogTypeConsume).
			Group("user_id").Scan(&users).Error; err != nil {
			return err
		}
		for _, row := range users {
			item := userMap[row.UserId]
			if item == nil {
				item = &AgentAnalyticsUserItem{UserId: row.UserId, Username: row.Username}
				userMap[row.UserId] = item
			}
			if item.Username == "" {
				item.Username = row.Username
			}
			item.Requests += row.Requests
			item.Errors += row.Errors
			item.Tokens += row.Tokens
			item.Quota += row.Quota
		}

		var logs []AgentAnalyticsLogItem
		if err := agentAnalyticsLogQuery(chunk, startTimestamp, endTimestamp).
			Select("id, user_id, username, created_at, type, token_name, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, " + CommonLogGroupCol() + ", request_id, content AS error_message, other").
			Order("id DESC").Limit(10).Scan(&logs).Error; err != nil {
			return err
		}
		hydrateAgentAnalyticsLogFailures(logs)
		recentLogs = append(recentLogs, logs...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	result.Summary.TotalRequests = result.Summary.SuccessfulRequests + result.Summary.FailedRequests
	if result.Summary.TotalRequests > 0 {
		result.Summary.SuccessRate = float64(result.Summary.SuccessfulRequests) / float64(result.Summary.TotalRequests) * 100
	}
	result.Summary.UsageUsers = int64(len(userMap))

	firstBucket := (startTimestamp / bucketSeconds) * bucketSeconds
	lastBucket := (endTimestamp / bucketSeconds) * bucketSeconds
	for timestamp := firstBucket; timestamp <= lastBucket; timestamp += bucketSeconds {
		point := trendMap[timestamp]
		if point == nil {
			point = &AgentAnalyticsTrendPoint{Timestamp: timestamp}
		}
		result.Trend = append(result.Trend, *point)
	}
	for _, item := range modelMap {
		result.TopModels = append(result.TopModels, *item)
	}
	sort.Slice(result.TopModels, func(i, j int) bool {
		if result.TopModels[i].Requests == result.TopModels[j].Requests {
			return result.TopModels[i].Quota > result.TopModels[j].Quota
		}
		return result.TopModels[i].Requests > result.TopModels[j].Requests
	})
	if len(result.TopModels) > 10 {
		result.TopModels = result.TopModels[:10]
	}
	for _, item := range userMap {
		result.TopUsers = append(result.TopUsers, *item)
	}
	sort.Slice(result.TopUsers, func(i, j int) bool {
		iTotal := result.TopUsers[i].Requests + result.TopUsers[i].Errors
		jTotal := result.TopUsers[j].Requests + result.TopUsers[j].Errors
		if iTotal == jTotal {
			return result.TopUsers[i].Quota > result.TopUsers[j].Quota
		}
		return iTotal > jTotal
	})
	if len(result.TopUsers) > 10 {
		result.TopUsers = result.TopUsers[:10]
	}
	sort.Slice(recentLogs, func(i, j int) bool { return recentLogs[i].Id > recentLogs[j].Id })
	if len(recentLogs) > 10 {
		recentLogs = recentLogs[:10]
	}
	result.RecentLogs = recentLogs
	return result, nil
}

func applyAgentAnalyticsLogFilters(tx *gorm.DB, filters AgentAnalyticsLogFilters) *gorm.DB {
	if filters.Type == LogTypeConsume || filters.Type == LogTypeError {
		tx = tx.Where("type = ?", filters.Type)
	}
	if keyword := strings.TrimSpace(filters.Username); keyword != "" {
		tx = tx.Where("username LIKE ?", "%"+keyword+"%")
	}
	if modelName := strings.TrimSpace(filters.ModelName); modelName != "" {
		tx = tx.Where("model_name LIKE ?", "%"+modelName+"%")
	}
	if requestID := strings.TrimSpace(filters.RequestId); requestID != "" {
		tx = tx.Where("request_id = ?", requestID)
	}
	return tx
}

func GetAgentAnalyticsLogs(agentID int, filters AgentAnalyticsLogFilters, startIdx, pageSize int) ([]AgentAnalyticsLogItem, int64, error) {
	userIDs, err := listAgentAnalyticsUserIDs(agentID)
	if err != nil || len(userIDs) == 0 {
		return []AgentAnalyticsLogItem{}, 0, err
	}
	allLogs := make([]AgentAnalyticsLogItem, 0)
	var total int64
	limit := startIdx + pageSize
	err = forEachAgentUserChunk(userIDs, func(chunk []int) error {
		base := applyAgentAnalyticsLogFilters(
			agentAnalyticsLogQuery(chunk, filters.StartTimestamp, filters.EndTimestamp),
			filters,
		)
		var chunkTotal int64
		if err := base.Count(&chunkTotal).Error; err != nil {
			return err
		}
		total += chunkTotal
		var logs []AgentAnalyticsLogItem
		if err := base.Select("id, user_id, username, created_at, type, token_name, model_name, quota, prompt_tokens, completion_tokens, use_time, is_stream, " + CommonLogGroupCol() + ", request_id, content AS error_message, other").
			Order("id DESC").Limit(limit).Scan(&logs).Error; err != nil {
			return err
		}
		allLogs = append(allLogs, logs...)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	sort.Slice(allLogs, func(i, j int) bool { return allLogs[i].Id > allLogs[j].Id })
	if startIdx >= len(allLogs) {
		return []AgentAnalyticsLogItem{}, total, nil
	}
	end := startIdx + pageSize
	if end > len(allLogs) {
		end = len(allLogs)
	}
	pageLogs := allLogs[startIdx:end]
	hydrateAgentAnalyticsLogFailures(pageLogs)
	return pageLogs, total, nil
}
