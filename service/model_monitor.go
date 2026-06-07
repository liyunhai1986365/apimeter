package service

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

const (
	DefaultModelMonitorWindowSeconds = int64(10 * 60)
	MaxModelMonitorWindowSeconds     = int64(24 * 60 * 60)
	modelMonitorCacheTTL             = 30 * time.Second
	modelMonitorRecentErrorLimit     = 20
)

type ModelMonitorQuery struct {
	Window         string
	StartTimestamp int64
	EndTimestamp   int64
	Group          string
}

type ModelMonitorWindow struct {
	Window         string `json:"window"`
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	WindowSeconds  int64  `json:"window_seconds"`
}

type ModelMonitorSummary struct {
	TotalRequests    int64   `json:"total_requests"`
	SuccessRequests  int64   `json:"success_requests"`
	ErrorRequests    int64   `json:"error_requests"`
	SuccessRate      float64 `json:"success_rate"`
	ErrorRate        float64 `json:"error_rate"`
	HealthScore      int     `json:"health_score"`
	AvgUseTime       float64 `json:"avg_use_time"`
	P95UseTime       int     `json:"p95_use_time"`
	Quota            int64   `json:"quota"`
	Tokens           int64   `json:"tokens"`
	AffectedChannels int64   `json:"affected_channels"`
	LatestError      string  `json:"latest_error,omitempty"`
	LatestErrorCode  string  `json:"latest_error_code,omitempty"`
	LatestErrorAt    int64   `json:"latest_error_at,omitempty"`
}

type ModelMonitorChannel struct {
	ChannelID       int     `json:"channel_id"`
	ChannelName     string  `json:"channel_name"`
	ChannelType     int     `json:"channel_type"`
	ChannelStatus   int     `json:"channel_status"`
	Group           string  `json:"group"`
	Priority        int64   `json:"priority"`
	Weight          uint    `json:"weight"`
	TotalRequests   int64   `json:"total_requests"`
	SuccessRequests int64   `json:"success_requests"`
	ErrorRequests   int64   `json:"error_requests"`
	SuccessRate     float64 `json:"success_rate"`
	ErrorRate       float64 `json:"error_rate"`
	HealthScore     int     `json:"health_score"`
	AvgUseTime      float64 `json:"avg_use_time"`
	P95UseTime      int     `json:"p95_use_time"`
	Quota           int64   `json:"quota"`
	Tokens          int64   `json:"tokens"`
	LatestError     string  `json:"latest_error,omitempty"`
	LatestErrorCode string  `json:"latest_error_code,omitempty"`
	LatestErrorAt   int64   `json:"latest_error_at,omitempty"`
	SampleStatus    string  `json:"sample_status"`
}

type ModelMonitorErrorItem struct {
	ChannelID   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	CreatedAt   int64  `json:"created_at"`
	Content     string `json:"content"`
	ErrorCode   string `json:"error_code,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
}

type ModelMonitorResult struct {
	ModelID      int                     `json:"model_id"`
	ModelName    string                  `json:"model_name"`
	Window       ModelMonitorWindow      `json:"window"`
	Summary      ModelMonitorSummary     `json:"summary"`
	Channels     []ModelMonitorChannel   `json:"channels"`
	RecentErrors []ModelMonitorErrorItem `json:"recent_errors"`
	CacheHit     bool                    `json:"cache_hit"`
}

type ModelMonitorListItem struct {
	ModelID            int                            `json:"model_id"`
	ModelName          string                         `json:"model_name"`
	Description        string                         `json:"description,omitempty"`
	Icon               string                         `json:"icon,omitempty"`
	Tags               string                         `json:"tags,omitempty"`
	Status             int                            `json:"status"`
	Summary            ModelMonitorSummary            `json:"summary"`
	ConfiguredChannels int                            `json:"configured_channels"`
	Channels           []ModelMonitorChannel          `json:"channels"`
	LatestTests        []model.ModelChannelTestResult `json:"latest_tests"`
}

type ModelMonitorListResult struct {
	Items     []ModelMonitorListItem `json:"items"`
	Total     int64                  `json:"total"`
	Page      int                    `json:"page"`
	PageSize  int                    `json:"page_size"`
	Window    ModelMonitorWindow     `json:"window"`
	CacheHint string                 `json:"cache_hint"`
}

type modelMonitorCacheEntry struct {
	expiresAt time.Time
	result    *ModelMonitorResult
}

var modelMonitorCache = struct {
	sync.Mutex
	items map[string]modelMonitorCacheEntry
}{
	items: map[string]modelMonitorCacheEntry{},
}

func NormalizeModelMonitorWindow(query ModelMonitorQuery, now int64) ModelMonitorWindow {
	if now <= 0 {
		now = time.Now().Unix()
	}

	end := query.EndTimestamp
	if end <= 0 || end > now {
		end = now
	}

	windowSeconds := parseModelMonitorWindow(query.Window)
	start := query.StartTimestamp
	if start <= 0 {
		start = end - windowSeconds
	} else {
		windowSeconds = end - start
	}
	if windowSeconds <= 0 {
		windowSeconds = DefaultModelMonitorWindowSeconds
		start = end - windowSeconds
	}
	if windowSeconds > MaxModelMonitorWindowSeconds {
		windowSeconds = MaxModelMonitorWindowSeconds
		start = end - windowSeconds
	}
	if start < 0 {
		start = 0
		windowSeconds = end - start
	}

	return ModelMonitorWindow{
		Window:         formatModelMonitorWindow(windowSeconds),
		StartTimestamp: start,
		EndTimestamp:   end,
		WindowSeconds:  windowSeconds,
	}
}

func GetModelMonitor(modelID int, query ModelMonitorQuery, now int64) (*ModelMonitorResult, error) {
	window := NormalizeModelMonitorWindow(query, now)
	cacheKey := fmt.Sprintf("%d:%d:%d:%s", modelID, window.StartTimestamp, window.EndTimestamp, strings.TrimSpace(query.Group))
	if cached, ok := getModelMonitorCache(cacheKey); ok {
		copy := *cached
		copy.CacheHit = true
		return &copy, nil
	}

	var modelMeta model.Model
	if err := model.DB.First(&modelMeta, "id = ?", modelID).Error; err != nil {
		return nil, err
	}

	channels, err := getModelMonitorChannels(modelMeta.ModelName, query.Group)
	if err != nil {
		return nil, err
	}

	statsByChannel, summary, err := aggregateModelMonitorLogs(modelMeta.ModelName, window, query.Group)
	if err != nil {
		return nil, err
	}

	channelItems := make([]ModelMonitorChannel, 0, len(channels))
	channelNameByID := make(map[int]string, len(channels))
	for _, ch := range channels {
		channelNameByID[ch.ChannelID] = ch.ChannelName
		item := statsByChannel[ch.ChannelID]
		item.ChannelID = ch.ChannelID
		item.ChannelName = ch.ChannelName
		item.ChannelType = ch.ChannelType
		item.ChannelStatus = ch.ChannelStatus
		item.Group = ch.Group
		item.Priority = ch.Priority
		item.Weight = ch.Weight
		item.recalculate()
		channelItems = append(channelItems, item)
	}
	sort.Slice(channelItems, func(i, j int) bool {
		if channelItems[i].TotalRequests != channelItems[j].TotalRequests {
			return channelItems[i].TotalRequests > channelItems[j].TotalRequests
		}
		return channelItems[i].ChannelID < channelItems[j].ChannelID
	})

	recentErrors, err := getModelMonitorRecentErrors(modelMeta.ModelName, window, query.Group, channelNameByID)
	if err != nil {
		return nil, err
	}
	if len(recentErrors) > 0 {
		summary.LatestError = recentErrors[0].Content
		summary.LatestErrorCode = recentErrors[0].ErrorCode
		summary.LatestErrorAt = recentErrors[0].CreatedAt
	}
	summary.recalculate()

	result := &ModelMonitorResult{
		ModelID:      modelMeta.Id,
		ModelName:    modelMeta.ModelName,
		Window:       window,
		Summary:      summary,
		Channels:     channelItems,
		RecentErrors: recentErrors,
	}
	setModelMonitorCache(cacheKey, result)
	return result, nil
}

func ListModelMonitorModels(startIdx int, pageSize int, page int, keyword string, query ModelMonitorQuery, now int64) (*ModelMonitorListResult, error) {
	var modelsMeta []*model.Model
	var total int64
	db := model.DB.Model(&model.Model{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("model_name LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like)
	}
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	if err := db.Order("id DESC").Find(&modelsMeta).Error; err != nil {
		return nil, err
	}

	window := NormalizeModelMonitorWindow(query, now)
	items := make([]ModelMonitorListItem, 0, len(modelsMeta))
	for _, m := range modelsMeta {
		monitor, err := GetModelMonitor(m.Id, query, now)
		if err != nil {
			return nil, err
		}
		items = append(items, ModelMonitorListItem{
			ModelID:            m.Id,
			ModelName:          m.ModelName,
			Description:        m.Description,
			Icon:               m.Icon,
			Tags:               m.Tags,
			Status:             m.Status,
			Summary:            monitor.Summary,
			ConfiguredChannels: len(monitor.Channels),
			Channels:           monitor.Channels,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		left := items[i].Summary
		right := items[j].Summary
		if left.ErrorRate != right.ErrorRate {
			return left.ErrorRate > right.ErrorRate
		}
		if left.ErrorRequests != right.ErrorRequests {
			return left.ErrorRequests > right.ErrorRequests
		}
		if left.TotalRequests != right.TotalRequests {
			return left.TotalRequests > right.TotalRequests
		}
		if left.HealthScore != right.HealthScore {
			return left.HealthScore < right.HealthScore
		}
		return items[i].ModelID > items[j].ModelID
	})

	endIdx := startIdx + pageSize
	if startIdx > len(items) {
		startIdx = len(items)
	}
	if endIdx > len(items) {
		endIdx = len(items)
	}
	pageItems := items[startIdx:endIdx]
	for i := range pageItems {
		latestTests, err := model.GetLatestModelChannelTestResults(pageItems[i].ModelID)
		if err != nil {
			return nil, err
		}
		pageItems[i].LatestTests = filterModelMonitorLatestTestsByChannels(latestTests, pageItems[i].Channels)
	}

	return &ModelMonitorListResult{
		Items:     pageItems,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
		Window:    window,
		CacheHint: "30s",
	}, nil
}

func filterModelMonitorLatestTestsByChannels(latestTests []model.ModelChannelTestResult, channels []ModelMonitorChannel) []model.ModelChannelTestResult {
	if len(latestTests) == 0 || len(channels) == 0 {
		return []model.ModelChannelTestResult{}
	}
	channelIDs := make(map[int]struct{}, len(channels))
	for _, channel := range channels {
		channelIDs[channel.ChannelID] = struct{}{}
	}
	filtered := make([]model.ModelChannelTestResult, 0, len(latestTests))
	for _, test := range latestTests {
		if _, ok := channelIDs[test.ChannelID]; ok {
			filtered = append(filtered, test)
		}
	}
	return filtered
}

type modelMonitorChannelConfig struct {
	ChannelID     int
	ChannelName   string
	ChannelType   int
	ChannelStatus int
	Group         string
	Priority      int64
	Weight        uint
}

func getModelMonitorChannels(modelName string, group string) ([]modelMonitorChannelConfig, error) {
	var rows []struct {
		ChannelID     int
		ChannelName   string
		ChannelType   int
		ChannelStatus int
		AbilityGroup  string
		Priority      int64
		Weight        uint
	}
	query := model.DB.Table("abilities").
		Select("abilities.channel_id as channel_id, channels.name as channel_name, channels.type as channel_type, channels.status as channel_status, abilities."+modelCommonGroupCol()+" as ability_group, abilities.priority as priority, abilities.weight as weight").
		Joins("JOIN channels ON channels.id = abilities.channel_id").
		Where("abilities.model = ? AND abilities.enabled = ?", modelName, true)
	if group = strings.TrimSpace(group); group != "" {
		query = query.Where("abilities."+modelCommonGroupCol()+" = ?", group)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	channels := make([]modelMonitorChannelConfig, 0, len(rows))
	seen := map[int]bool{}
	for _, row := range rows {
		if seen[row.ChannelID] {
			continue
		}
		seen[row.ChannelID] = true
		channels = append(channels, modelMonitorChannelConfig{
			ChannelID:     row.ChannelID,
			ChannelName:   row.ChannelName,
			ChannelType:   row.ChannelType,
			ChannelStatus: row.ChannelStatus,
			Group:         row.AbilityGroup,
			Priority:      row.Priority,
			Weight:        row.Weight,
		})
	}
	sort.Slice(channels, func(i, j int) bool {
		if channels[i].Priority != channels[j].Priority {
			return channels[i].Priority > channels[j].Priority
		}
		return channels[i].ChannelID < channels[j].ChannelID
	})
	return channels, nil
}

func aggregateModelMonitorLogs(modelName string, window ModelMonitorWindow, group string) (map[int]ModelMonitorChannel, ModelMonitorSummary, error) {
	var rows []struct {
		ChannelID        int
		Type             int
		Count            int64
		Quota            int64
		PromptTokens     int64
		CompletionTokens int64
		UseTimeSum       int64
	}
	query := model.LOG_DB.Table("logs").
		Select("channel_id as channel_id, type as type, count(*) as count, COALESCE(sum(quota), 0) as quota, COALESCE(sum(prompt_tokens), 0) as prompt_tokens, COALESCE(sum(completion_tokens), 0) as completion_tokens, COALESCE(sum(use_time), 0) as use_time_sum").
		Where("model_name = ? AND created_at >= ? AND created_at <= ? AND type IN ?", modelName, window.StartTimestamp, window.EndTimestamp, []int{model.LogTypeConsume, model.LogTypeError}).
		Group("channel_id, type")
	if group = strings.TrimSpace(group); group != "" {
		query = query.Where(modelLogGroupCol()+" = ?", group)
	}
	query = onlyFinalRetryLogRows(query, func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.
			Where(prefix+"model_name = ?", modelName).
			Where(prefix+"created_at >= ? AND "+prefix+"created_at <= ?", window.StartTimestamp, window.EndTimestamp).
			Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError})
		if group != "" {
			tx = tx.Where(prefix+modelLogGroupCol()+" = ?", group)
		}
		return tx
	})
	if err := query.Scan(&rows).Error; err != nil {
		return nil, ModelMonitorSummary{}, err
	}

	statsByChannel := map[int]ModelMonitorChannel{}
	summary := ModelMonitorSummary{}
	for _, row := range rows {
		item := statsByChannel[row.ChannelID]
		item.ChannelID = row.ChannelID
		item.TotalRequests += row.Count
		summary.TotalRequests += row.Count
		if row.Type == model.LogTypeConsume {
			item.SuccessRequests += row.Count
			item.Quota += row.Quota
			item.Tokens += row.PromptTokens + row.CompletionTokens
			item.AvgUseTime += float64(row.UseTimeSum)
			summary.SuccessRequests += row.Count
			summary.Quota += row.Quota
			summary.Tokens += row.PromptTokens + row.CompletionTokens
			summary.AvgUseTime += float64(row.UseTimeSum)
		}
		if row.Type == model.LogTypeError {
			item.ErrorRequests += row.Count
			summary.ErrorRequests += row.Count
		}
		statsByChannel[row.ChannelID] = item
	}

	useTimesByChannel, allUseTimes, err := getModelMonitorUseTimes(modelName, window, group)
	if err != nil {
		return nil, ModelMonitorSummary{}, err
	}
	for channelID, useTimes := range useTimesByChannel {
		item := statsByChannel[channelID]
		item.P95UseTime = percentileInt(useTimes, 0.95)
		if item.SuccessRequests > 0 {
			item.AvgUseTime = round2(item.AvgUseTime / float64(item.SuccessRequests))
		}
		statsByChannel[channelID] = item
	}
	summary.P95UseTime = percentileInt(allUseTimes, 0.95)
	if summary.SuccessRequests > 0 {
		summary.AvgUseTime = round2(summary.AvgUseTime / float64(summary.SuccessRequests))
	}

	latestByChannel, err := getModelMonitorLatestErrorsByChannel(modelName, window, group)
	if err != nil {
		return nil, ModelMonitorSummary{}, err
	}
	for channelID, latest := range latestByChannel {
		item := statsByChannel[channelID]
		item.LatestError = latest.Content
		item.LatestErrorCode = latest.ErrorCode
		item.LatestErrorAt = latest.CreatedAt
		statsByChannel[channelID] = item
	}

	summary.AffectedChannels = int64(len(statsByChannel))
	return statsByChannel, summary, nil
}

func getModelMonitorUseTimes(modelName string, window ModelMonitorWindow, group string) (map[int][]int, []int, error) {
	var logs []model.Log
	query := model.LOG_DB.Select("channel_id", "use_time").
		Where("model_name = ? AND created_at >= ? AND created_at <= ? AND type = ?", modelName, window.StartTimestamp, window.EndTimestamp, model.LogTypeConsume)
	if group = strings.TrimSpace(group); group != "" {
		query = query.Where(modelLogGroupCol()+" = ?", group)
	}
	query = onlyFinalRetryLogRows(query, func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.
			Where(prefix+"model_name = ?", modelName).
			Where(prefix+"created_at >= ? AND "+prefix+"created_at <= ?", window.StartTimestamp, window.EndTimestamp).
			Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError})
		if group != "" {
			tx = tx.Where(prefix+modelLogGroupCol()+" = ?", group)
		}
		return tx
	})
	if err := query.Find(&logs).Error; err != nil {
		return nil, nil, err
	}
	byChannel := map[int][]int{}
	all := make([]int, 0, len(logs))
	for _, log := range logs {
		byChannel[log.ChannelId] = append(byChannel[log.ChannelId], log.UseTime)
		all = append(all, log.UseTime)
	}
	return byChannel, all, nil
}

func getModelMonitorLatestErrorsByChannel(modelName string, window ModelMonitorWindow, group string) (map[int]ModelMonitorErrorItem, error) {
	var logs []model.Log
	query := model.LOG_DB.
		Where("model_name = ? AND created_at >= ? AND created_at <= ? AND type = ?", modelName, window.StartTimestamp, window.EndTimestamp, model.LogTypeError)
	if group = strings.TrimSpace(group); group != "" {
		query = query.Where(modelLogGroupCol()+" = ?", group)
	}
	query = onlyFinalRetryLogRows(query, func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.
			Where(prefix+"model_name = ?", modelName).
			Where(prefix+"created_at >= ? AND "+prefix+"created_at <= ?", window.StartTimestamp, window.EndTimestamp).
			Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError})
		if group != "" {
			tx = tx.Where(prefix+modelLogGroupCol()+" = ?", group)
		}
		return tx
	})
	if err := query.Order("created_at desc, id desc").Limit(200).Find(&logs).Error; err != nil {
		return nil, err
	}
	result := map[int]ModelMonitorErrorItem{}
	for _, log := range logs {
		if _, ok := result[log.ChannelId]; ok {
			continue
		}
		result[log.ChannelId] = ModelMonitorErrorItem{
			ChannelID: log.ChannelId,
			CreatedAt: log.CreatedAt,
			Content:   log.Content,
			ErrorCode: extractMonitorErrorCode(log.Other),
			RequestID: log.RequestId,
		}
	}
	return result, nil
}

func getModelMonitorRecentErrors(modelName string, window ModelMonitorWindow, group string, channelNameByID map[int]string) ([]ModelMonitorErrorItem, error) {
	var logs []model.Log
	query := model.LOG_DB.
		Where("model_name = ? AND created_at >= ? AND created_at <= ? AND type = ?", modelName, window.StartTimestamp, window.EndTimestamp, model.LogTypeError)
	if group = strings.TrimSpace(group); group != "" {
		query = query.Where(modelLogGroupCol()+" = ?", group)
	}
	query = onlyFinalRetryLogRows(query, func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.
			Where(prefix+"model_name = ?", modelName).
			Where(prefix+"created_at >= ? AND "+prefix+"created_at <= ?", window.StartTimestamp, window.EndTimestamp).
			Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError})
		if group != "" {
			tx = tx.Where(prefix+modelLogGroupCol()+" = ?", group)
		}
		return tx
	})
	if err := query.Order("created_at desc, id desc").Limit(modelMonitorRecentErrorLimit).Find(&logs).Error; err != nil {
		return nil, err
	}
	items := make([]ModelMonitorErrorItem, 0, len(logs))
	for _, log := range logs {
		items = append(items, ModelMonitorErrorItem{
			ChannelID:   log.ChannelId,
			ChannelName: channelNameByID[log.ChannelId],
			CreatedAt:   log.CreatedAt,
			Content:     log.Content,
			ErrorCode:   extractMonitorErrorCode(log.Other),
			RequestID:   log.RequestId,
		})
	}
	return items, nil
}

func (item *ModelMonitorChannel) recalculate() {
	item.SuccessRate, item.ErrorRate = calculateRates(item.SuccessRequests, item.ErrorRequests)
	item.HealthScore = calculateHealthScore(item.SuccessRate, item.TotalRequests, item.P95UseTime, item.LatestErrorAt)
	if item.TotalRequests == 0 {
		item.SampleStatus = "no_data"
		return
	}
	if item.TotalRequests < 5 {
		item.SampleStatus = "insufficient"
		return
	}
	item.SampleStatus = "enough"
}

func (summary *ModelMonitorSummary) recalculate() {
	summary.SuccessRate, summary.ErrorRate = calculateRates(summary.SuccessRequests, summary.ErrorRequests)
	summary.HealthScore = calculateHealthScore(summary.SuccessRate, summary.TotalRequests, summary.P95UseTime, summary.LatestErrorAt)
}

func calculateRates(success int64, errors int64) (float64, float64) {
	total := success + errors
	if total <= 0 {
		return 0, 0
	}
	successRate := round2(float64(success) * 100 / float64(total))
	return successRate, round2(100 - successRate)
}

func onlyFinalRetryLogRows(tx *gorm.DB, applyFilters func(*gorm.DB, string) *gorm.DB) *gorm.DB {
	newer := model.LOG_DB.Table("logs AS newer").Select("1")
	newer = applyFilters(newer, "newer")
	newer = newer.
		Where("newer.request_id = logs.request_id").
		Where("newer.request_id <> ''").
		Where("newer.model_name = logs.model_name").
		Where("(newer.created_at > logs.created_at OR (newer.created_at = logs.created_at AND newer.id > logs.id))").
		Limit(1)

	return tx.Where("NOT (logs.request_id <> '' AND EXISTS (?))", newer)
}

func calculateHealthScore(successRate float64, total int64, p95UseTime int, latestErrorAt int64) int {
	if total <= 0 {
		return 0
	}
	score := successRate
	if total < 5 {
		score *= 0.7
	}
	if p95UseTime > 0 {
		switch {
		case p95UseTime <= 3:
			score += 5
		case p95UseTime >= 20:
			score -= 15
		case p95UseTime >= 10:
			score -= 8
		}
	}
	if latestErrorAt > 0 {
		score -= 5
	}
	if score < 0 {
		return 0
	}
	if score > 100 {
		return 100
	}
	return int(math.Round(score))
}

func parseModelMonitorWindow(window string) int64 {
	switch strings.TrimSpace(strings.ToLower(window)) {
	case "30m":
		return 30 * 60
	case "1h":
		return 60 * 60
	case "6h":
		return 6 * 60 * 60
	case "24h":
		return 24 * 60 * 60
	default:
		return DefaultModelMonitorWindowSeconds
	}
}

func formatModelMonitorWindow(seconds int64) string {
	switch seconds {
	case 10 * 60:
		return "10m"
	case 30 * 60:
		return "30m"
	case 60 * 60:
		return "1h"
	case 6 * 60 * 60:
		return "6h"
	case 24 * 60 * 60:
		return "24h"
	default:
		return strconv.FormatInt(seconds, 10) + "s"
	}
}

func percentileInt(values []int, percentile float64) int {
	if len(values) == 0 {
		return 0
	}
	copied := append([]int(nil), values...)
	sort.Ints(copied)
	index := int(math.Ceil(float64(len(copied))*percentile)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(copied) {
		index = len(copied) - 1
	}
	return copied[index]
}

func extractMonitorErrorCode(other string) string {
	if strings.TrimSpace(other) == "" {
		return ""
	}
	otherMap, err := common.StrToMap(other)
	if err != nil || otherMap == nil {
		return ""
	}
	for _, key := range []string{"code", "error_code", "status_code"} {
		if value, ok := otherMap[key]; ok {
			return strings.TrimSpace(fmt.Sprint(value))
		}
	}
	return ""
}

func getModelMonitorCache(key string) (*ModelMonitorResult, bool) {
	modelMonitorCache.Lock()
	defer modelMonitorCache.Unlock()
	entry, ok := modelMonitorCache.items[key]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(modelMonitorCache.items, key)
		return nil, false
	}
	return entry.result, true
}

func setModelMonitorCache(key string, result *ModelMonitorResult) {
	modelMonitorCache.Lock()
	defer modelMonitorCache.Unlock()
	modelMonitorCache.items[key] = modelMonitorCacheEntry{
		expiresAt: time.Now().Add(modelMonitorCacheTTL),
		result:    result,
	}
}

func ClearModelMonitorCache() {
	modelMonitorCache.Lock()
	defer modelMonitorCache.Unlock()
	modelMonitorCache.items = map[string]modelMonitorCacheEntry{}
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}

func modelCommonGroupCol() string {
	if common.UsingPostgreSQL {
		return `"group"`
	}
	return "`group`"
}

func modelLogGroupCol() string {
	if common.LogSqlType == common.DatabaseTypePostgreSQL {
		return `"group"`
	}
	return "`group`"
}
