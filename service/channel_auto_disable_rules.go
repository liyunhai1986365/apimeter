package service

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

type ChannelAutoDisableRule = operation_setting.ChannelAutoDisableRule

type ChannelAutoDisableRuleMatchQuery struct {
	ChannelID int
	Now       int64
	Group     string
	Rules     []ChannelAutoDisableRule
}

type ChannelAutoDisableRuleMatch struct {
	Rule                      ChannelAutoDisableRule `json:"rule"`
	Stats                     ChannelModelErrorStats `json:"stats"`
	PeakMinuteErrorCount      int64                  `json:"peak_minute_error_count"`
	FirstTokenSlowCount       int64                  `json:"first_token_slow_count"`
	PeakMinuteFirstTokenCount int64                  `json:"peak_minute_first_token_count"`
	MatchedConditions         []string               `json:"matched_conditions"`
	Extra                     map[string]interface{} `json:"extra"`
}

func BuildChannelAutoDisableRuleRecordMeta(match ChannelAutoDisableRuleMatch, operationGroupID string, targetChannelID int) ChannelStatusChangeRecordMeta {
	extra := common.GetJsonString(match.Extra)
	return ChannelStatusChangeRecordMeta{
		TargetChannelID:  targetChannelID,
		OperationGroupID: operationGroupID,
		WindowSeconds:    int64(match.Rule.WindowMinutes) * 60,
		TotalRequests:    match.Stats.TotalRequests,
		ErrorRequests:    match.Stats.ErrorCount,
		ErrorRate:        match.Stats.ErrorRate,
		Extra:            extra,
	}
}

func BuildChannelAutoDisableRuleReason(match ChannelAutoDisableRuleMatch) string {
	ruleName := strings.TrimSpace(match.Rule.Name)
	if ruleName == "" {
		ruleName = strings.TrimSpace(match.Rule.ID)
	}
	if ruleName == "" {
		ruleName = "未命名规则"
	}
	reason := fmt.Sprintf(
		"模型 %s 命中自动禁用规则「%s」：最近 %d 分钟内错误 %d 次，总请求 %d 次，错误率 %.2f%%",
		match.Stats.ModelName,
		ruleName,
		match.Rule.WindowMinutes,
		match.Stats.ErrorCount,
		match.Stats.TotalRequests,
		match.Stats.ErrorRate,
	)
	if match.Rule.FirstTokenCountThreshold > 0 {
		reason += fmt.Sprintf("，首 Token 慢响应 %d 次", match.FirstTokenSlowCount)
	}
	if match.Rule.PerMinuteErrorThreshold > 0 {
		reason += fmt.Sprintf("，单分钟最高错误 %d 次", match.PeakMinuteErrorCount)
	}
	if match.Rule.FirstTokenCountThreshold > 0 {
		reason += fmt.Sprintf("，单分钟最高首 Token 慢响应 %d 次", match.PeakMinuteFirstTokenCount)
	}
	if strings.TrimSpace(match.Stats.LatestError) != "" {
		reason += "；最近错误：" + match.Stats.LatestError
	}
	return reason
}

type autoDisableLogCandidate struct {
	ID            int
	CreatedAt     int64
	Type          int
	ModelName     string
	Content       string
	UseTime       int
	FirstToken    float64
	HasFirstToken bool
	Group         string
	Other         string
	RequestID     string
	StatusCode    int
	ErrorCode     string
}

func GetChannelAutoDisableRuleMatches(query ChannelAutoDisableRuleMatchQuery) ([]ChannelAutoDisableRuleMatch, error) {
	if query.ChannelID <= 0 || len(query.Rules) == 0 {
		return []ChannelAutoDisableRuleMatch{}, nil
	}
	if query.Now <= 0 {
		query.Now = common.GetTimestamp()
	}

	matches := make([]ChannelAutoDisableRuleMatch, 0)
	for _, rule := range query.Rules {
		rule = normalizeChannelAutoDisableRule(rule)
		if !rule.Enabled || rule.WindowMinutes <= 0 {
			continue
		}
		if strings.TrimSpace(rule.StatusCodes) != "" {
			if _, err := operation_setting.ParseHTTPStatusCodeRanges(rule.StatusCodes); err != nil {
				common.SysLog(fmt.Sprintf("skip invalid channel auto disable rule: rule_id=%s error=%v", rule.ID, err))
				continue
			}
		}
		logs, err := getAutoDisableRuleLogs(query, rule)
		if err != nil {
			return nil, err
		}
		ruleMatches := evaluateAutoDisableRule(rule, logs)
		matches = append(matches, ruleMatches...)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Stats.ErrorCount != matches[j].Stats.ErrorCount {
			return matches[i].Stats.ErrorCount > matches[j].Stats.ErrorCount
		}
		if matches[i].Stats.ErrorRate != matches[j].Stats.ErrorRate {
			return matches[i].Stats.ErrorRate > matches[j].Stats.ErrorRate
		}
		return matches[i].Stats.LatestErrorAt > matches[j].Stats.LatestErrorAt
	})
	return matches, nil
}

func normalizeChannelAutoDisableRule(rule ChannelAutoDisableRule) ChannelAutoDisableRule {
	rule.ID = strings.TrimSpace(rule.ID)
	rule.Name = strings.TrimSpace(rule.Name)
	rule.StatusCodes = strings.TrimSpace(rule.StatusCodes)
	rule.ModelNames = normalizeAutoDisableStringList(rule.ModelNames)
	rule.ErrorTypes = normalizeAutoDisableStringList(rule.ErrorTypes)
	if rule.WindowMinutes <= 0 {
		rule.WindowMinutes = 10
	}
	return rule
}

func normalizeAutoDisableStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			key := strings.ToLower(part)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			normalized = append(normalized, part)
		}
	}
	return normalized
}

func getAutoDisableRuleLogs(query ChannelAutoDisableRuleMatchQuery, rule ChannelAutoDisableRule) ([]autoDisableLogCandidate, error) {
	since := query.Now - int64(rule.WindowMinutes)*60
	if since < 0 {
		since = 0
	}
	db := model.LOG_DB.Model(&model.Log{}).
		Where("channel_id = ? AND type IN ? AND created_at >= ? AND created_at <= ? AND model_name <> ?",
			query.ChannelID, []int{model.LogTypeConsume, model.LogTypeError}, since, query.Now, "")
	group := strings.TrimSpace(query.Group)
	if group != "" {
		db = db.Where(model.CommonLogGroupCol()+" = ?", group)
	}
	db = onlyFinalRetryLogRows(db, func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.
			Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError}).
			Where(prefix+"created_at >= ? AND "+prefix+"created_at <= ?", since, query.Now).
			Where(prefix+"model_name <> ?", "")
		if group != "" {
			tx = tx.Where(prefix+model.CommonLogGroupCol()+" = ?", group)
		}
		return tx
	})

	var rows []model.Log
	if err := db.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	logs := make([]autoDisableLogCandidate, 0, len(rows))
	for _, row := range rows {
		statusCode, errorCode := extractAutoDisableLogErrorMeta(row.Other)
		firstToken, hasFirstToken := extractMonitorTtftSeconds(row.Other)
		logs = append(logs, autoDisableLogCandidate{
			ID:            row.Id,
			CreatedAt:     row.CreatedAt,
			Type:          row.Type,
			ModelName:     row.ModelName,
			Content:       strings.TrimSpace(row.Content),
			UseTime:       row.UseTime,
			FirstToken:    firstToken,
			HasFirstToken: hasFirstToken,
			Group:         row.Group,
			Other:         row.Other,
			RequestID:     row.RequestId,
			StatusCode:    statusCode,
			ErrorCode:     errorCode,
		})
	}
	return logs, nil
}

func evaluateAutoDisableRule(rule ChannelAutoDisableRule, logs []autoDisableLogCandidate) []ChannelAutoDisableRuleMatch {
	byModel := map[string][]autoDisableLogCandidate{}
	for _, item := range logs {
		if len(rule.ModelNames) > 0 && !matchAutoDisableModel(rule.ModelNames, item.ModelName) {
			continue
		}
		byModel[item.ModelName] = append(byModel[item.ModelName], item)
	}

	matches := make([]ChannelAutoDisableRuleMatch, 0, len(byModel))
	for modelName, modelLogs := range byModel {
		match, ok := evaluateAutoDisableRuleForModel(rule, modelName, modelLogs)
		if ok {
			matches = append(matches, match)
		}
	}
	return matches
}

func evaluateAutoDisableRuleForModel(rule ChannelAutoDisableRule, modelName string, logs []autoDisableLogCandidate) (ChannelAutoDisableRuleMatch, bool) {
	totalRequests := int64(len(logs))
	matchingErrors := make([]autoDisableLogCandidate, 0)
	for _, item := range logs {
		if item.Type != model.LogTypeError {
			continue
		}
		if len(rule.ErrorTypes) > 0 && !matchAutoDisableErrorType(rule.ErrorTypes, item) {
			continue
		}
		if strings.TrimSpace(rule.StatusCodes) != "" && !matchAutoDisableStatusCode(rule.StatusCodes, item.StatusCode) {
			continue
		}
		matchingErrors = append(matchingErrors, item)
	}

	errorCount := int64(len(matchingErrors))
	errorRate := 0.0
	if totalRequests > 0 {
		errorRate = math.Round(float64(errorCount)*10000/float64(totalRequests)) / 100
	}
	peakMinute := peakMinuteErrorCount(matchingErrors)
	firstTokenSlowLogs := matchingFirstTokenSlowLogs(rule, logs)
	firstTokenSlowCount := int64(len(firstTokenSlowLogs))
	peakMinuteFirstToken := peakMinuteErrorCount(firstTokenSlowLogs)

	if rule.MinRequests > 0 && totalRequests < int64(rule.MinRequests) {
		return ChannelAutoDisableRuleMatch{}, false
	}
	if rule.ErrorCountThreshold > 0 && errorCount < int64(rule.ErrorCountThreshold) {
		return ChannelAutoDisableRuleMatch{}, false
	}
	if rule.ErrorRate > 0 && errorRate < rule.ErrorRate {
		return ChannelAutoDisableRuleMatch{}, false
	}
	if rule.PerMinuteErrorThreshold > 0 && peakMinute < int64(rule.PerMinuteErrorThreshold) {
		return ChannelAutoDisableRuleMatch{}, false
	}
	if rule.FirstTokenCountThreshold > 0 {
		if rule.FirstTokenSeconds <= 0 || firstTokenSlowCount < int64(rule.FirstTokenCountThreshold) {
			return ChannelAutoDisableRuleMatch{}, false
		}
	}
	if rule.ErrorCountThreshold == 0 && rule.ErrorRate == 0 && rule.PerMinuteErrorThreshold == 0 && rule.FirstTokenCountThreshold == 0 {
		return ChannelAutoDisableRuleMatch{}, false
	}

	var latest autoDisableLogCandidate
	if len(matchingErrors) > 0 {
		latest = latestAutoDisableError(matchingErrors)
	}
	latestFirstToken := latestAutoDisableLog(firstTokenSlowLogs)
	stats := ChannelModelErrorStats{
		ModelName:     modelName,
		TotalRequests: totalRequests,
		ErrorCount:    errorCount,
		ErrorRate:     errorRate,
		LatestError:   latest.Content,
		LatestErrorAt: latest.CreatedAt,
	}
	matchedConditions := autoDisableMatchedConditions(rule)
	extra := map[string]interface{}{
		"rule_id":                       rule.ID,
		"rule_name":                     rule.Name,
		"matched_conditions":            matchedConditions,
		"peak_minute_error_count":       peakMinute,
		"first_token_slow_count":        firstTokenSlowCount,
		"peak_minute_first_token_count": peakMinuteFirstToken,
		"window_minutes":                rule.WindowMinutes,
		"latest_error_code":             latest.ErrorCode,
		"latest_status_code":            latest.StatusCode,
		"latest_first_token":            latestFirstToken.FirstToken,
	}
	return ChannelAutoDisableRuleMatch{
		Rule:                      rule,
		Stats:                     stats,
		PeakMinuteErrorCount:      peakMinute,
		FirstTokenSlowCount:       firstTokenSlowCount,
		PeakMinuteFirstTokenCount: peakMinuteFirstToken,
		MatchedConditions:         matchedConditions,
		Extra:                     extra,
	}, true
}

func matchingFirstTokenSlowLogs(rule ChannelAutoDisableRule, logs []autoDisableLogCandidate) []autoDisableLogCandidate {
	if rule.FirstTokenSeconds <= 0 {
		return nil
	}
	matched := make([]autoDisableLogCandidate, 0)
	for _, item := range logs {
		if !item.HasFirstToken {
			continue
		}
		if item.FirstToken < float64(rule.FirstTokenSeconds) {
			continue
		}
		matched = append(matched, item)
	}
	return matched
}

func matchAutoDisableModel(patterns []string, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" || pattern == "*" {
			return true
		}
		if strings.EqualFold(pattern, modelName) {
			return true
		}
		if strings.HasSuffix(pattern, "*") && strings.HasPrefix(strings.ToLower(modelName), strings.ToLower(strings.TrimSuffix(pattern, "*"))) {
			return true
		}
	}
	return false
}

func matchAutoDisableErrorType(errorTypes []string, item autoDisableLogCandidate) bool {
	code := strings.ToLower(strings.TrimSpace(item.ErrorCode))
	content := strings.ToLower(strings.TrimSpace(item.Content))
	for _, errorType := range errorTypes {
		errorType = strings.ToLower(strings.TrimSpace(errorType))
		if errorType == "" {
			continue
		}
		if code == errorType || strings.Contains(content, errorType) {
			return true
		}
	}
	return false
}

func matchAutoDisableStatusCode(rule string, code int) bool {
	if code < 100 || code > 599 {
		return false
	}
	ranges, err := operation_setting.ParseHTTPStatusCodeRanges(rule)
	if err != nil {
		return false
	}
	for _, r := range ranges {
		if code >= r.Start && code <= r.End {
			return true
		}
	}
	return false
}

func peakMinuteErrorCount(errors []autoDisableLogCandidate) int64 {
	buckets := map[int64]int64{}
	var peak int64
	for _, item := range errors {
		minute := item.CreatedAt / 60
		buckets[minute]++
		if buckets[minute] > peak {
			peak = buckets[minute]
		}
	}
	return peak
}

func latestAutoDisableError(errors []autoDisableLogCandidate) autoDisableLogCandidate {
	latest := errors[0]
	for _, item := range errors[1:] {
		if item.CreatedAt > latest.CreatedAt || (item.CreatedAt == latest.CreatedAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return latest
}

func latestAutoDisableLog(logs []autoDisableLogCandidate) autoDisableLogCandidate {
	if len(logs) == 0 {
		return autoDisableLogCandidate{}
	}
	latest := logs[0]
	for _, item := range logs[1:] {
		if item.CreatedAt > latest.CreatedAt || (item.CreatedAt == latest.CreatedAt && item.ID > latest.ID) {
			latest = item
		}
	}
	return latest
}

func autoDisableMatchedConditions(rule ChannelAutoDisableRule) []string {
	conditions := make([]string, 0, 8)
	if len(rule.ModelNames) > 0 {
		conditions = append(conditions, "model")
	}
	if len(rule.ErrorTypes) > 0 {
		conditions = append(conditions, "error_type")
	}
	if strings.TrimSpace(rule.StatusCodes) != "" {
		conditions = append(conditions, "status_code")
	}
	if rule.FirstTokenSeconds > 0 && rule.FirstTokenCountThreshold > 0 {
		conditions = append(conditions, "first_token")
	}
	if rule.FirstTokenCountThreshold > 0 {
		conditions = append(conditions, "first_token_count")
	}
	if rule.ErrorCountThreshold > 0 {
		conditions = append(conditions, "error_count")
	}
	if rule.ErrorRate > 0 {
		conditions = append(conditions, "error_rate")
	}
	if rule.MinRequests > 0 {
		conditions = append(conditions, "min_requests")
	}
	if rule.PerMinuteErrorThreshold > 0 {
		conditions = append(conditions, "per_minute_error_count")
	}
	return conditions
}

func extractAutoDisableLogErrorMeta(other string) (int, string) {
	if strings.TrimSpace(other) == "" {
		return 0, ""
	}
	var otherMap map[string]interface{}
	if err := common.Unmarshal([]byte(other), &otherMap); err != nil || otherMap == nil {
		return 0, ""
	}
	statusCode := extractAutoDisableStatusCode(otherMap)
	errorCode := ""
	for _, key := range []string{"code", "error_code", "type", "error_type"} {
		if value, ok := otherMap[key]; ok {
			errorCode = strings.TrimSpace(fmt.Sprint(value))
			if errorCode != "" {
				break
			}
		}
	}
	if errorCode == "" && statusCode > 0 {
		errorCode = strconv.Itoa(statusCode)
	}
	return statusCode, errorCode
}

func extractAutoDisableStatusCode(otherMap map[string]interface{}) int {
	for _, key := range []string{"status_code", "status", "http_status", "http_status_code"} {
		value, ok := otherMap[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case float64:
			return int(v)
		case int:
			return v
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(v))
			if err == nil {
				return parsed
			}
		default:
			parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
			if err == nil {
				return parsed
			}
		}
	}
	return 0
}
