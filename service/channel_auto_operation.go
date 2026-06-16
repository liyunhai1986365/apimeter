package service

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type ChannelModelErrorStats struct {
	ModelName     string  `json:"model_name"`
	TotalRequests int64   `json:"total_requests"`
	ErrorCount    int64   `json:"error_count"`
	ErrorRate     float64 `json:"error_rate"`
	LatestError   string  `json:"latest_error"`
	LatestErrorAt int64   `json:"latest_error_at"`
}

type ChannelModelErrorStatsQuery struct {
	ChannelID        int
	Threshold        int
	WindowSeconds    int64
	Now              int64
	MinRequests      int
	ErrorRatePercent float64
	Group            string
}

type AutoEnableCandidateQuery struct {
	FailedChannelID int
	ModelName       string
	Group           string
}

type AutoEnableOperationRecordCandidateQuery struct {
	Now             int64
	CooldownSeconds int64
	Limit           int
}

type AutoEnableOperationRecordCandidate struct {
	Channel *model.Channel
	Record  model.ChannelOperationRecord
}

func BuildChannelAutoOperationRecordMeta(stat ChannelModelErrorStats, operationGroupID string, targetChannelID int, windowSeconds int64) ChannelStatusChangeRecordMeta {
	return ChannelStatusChangeRecordMeta{
		TargetChannelID:  targetChannelID,
		OperationGroupID: operationGroupID,
		WindowSeconds:    windowSeconds,
		TotalRequests:    stat.TotalRequests,
		ErrorRequests:    stat.ErrorCount,
		ErrorRate:        stat.ErrorRate,
	}
}

func BuildChannelAutoOperationGroupID(channelID int, modelName string, now int64) string {
	modelName = strings.NewReplacer(" ", "_", "/", "_", "\\", "_").Replace(strings.TrimSpace(modelName))
	if len(modelName) > 32 {
		modelName = modelName[:32]
	}
	return fmt.Sprintf("auto-%d-%s-%d", channelID, modelName, now)
}

func GetChannelModelErrorStats(args ...interface{}) ([]ChannelModelErrorStats, error) {
	query := normalizeChannelModelErrorStatsQuery(args...)
	if query.ChannelID <= 0 || query.Threshold <= 0 || query.WindowSeconds <= 0 {
		return []ChannelModelErrorStats{}, nil
	}
	if query.Now <= 0 {
		query.Now = common.GetTimestamp()
	}
	if query.MinRequests <= 0 {
		query.MinRequests = query.Threshold
	}
	if query.ErrorRatePercent <= 0 {
		query.ErrorRatePercent = 0
	}
	since := query.Now - query.WindowSeconds
	if since < 0 {
		since = 0
	}

	var rows []struct {
		ModelName     string
		TotalRequests int64
		ErrorCount    int64
		LatestErrorAt int64
	}
	db := model.LOG_DB.Model(&model.Log{}).
		Select(
			"model_name, COUNT(*) AS total_requests, SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) AS error_count, MAX(CASE WHEN type = ? THEN created_at ELSE 0 END) AS latest_error_at",
			model.LogTypeError,
			model.LogTypeError,
		).
		Where("channel_id = ? AND type IN ? AND created_at >= ? AND model_name <> ?", query.ChannelID, []int{model.LogTypeConsume, model.LogTypeError}, since, "").
		Group("model_name")
	if strings.TrimSpace(query.Group) != "" {
		db = db.Where(model.CommonLogGroupCol()+" = ?", strings.TrimSpace(query.Group))
	}
	group := strings.TrimSpace(query.Group)
	db = onlyFinalRetryLogRows(db, func(tx *gorm.DB, alias string) *gorm.DB {
		prefix := alias + "."
		tx = tx.
			Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError}).
			Where(prefix+"created_at >= ?", since).
			Where(prefix+"model_name <> ?", "")
		if group != "" {
			tx = tx.Where(prefix+model.CommonLogGroupCol()+" = ?", group)
		}
		return tx
	})
	err := db.
		Having("SUM(CASE WHEN type = ? THEN 1 ELSE 0 END) >= ?", model.LogTypeError, query.Threshold).
		Having("COUNT(*) >= ?", query.MinRequests).
		Order("error_count DESC, latest_error_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	stats := make([]ChannelModelErrorStats, 0, len(rows))
	for _, row := range rows {
		errorRate := 0.0
		if row.TotalRequests > 0 {
			errorRate = math.Round((float64(row.ErrorCount)/float64(row.TotalRequests))*10000) / 100
		}
		if errorRate < query.ErrorRatePercent {
			continue
		}
		var latest model.Log
		latestQuery := model.LOG_DB.Model(&model.Log{}).
			Where("channel_id = ? AND type = ? AND model_name = ? AND created_at >= ?", query.ChannelID, model.LogTypeError, row.ModelName, since).
			Order("created_at DESC, id DESC")
		if group != "" {
			latestQuery = latestQuery.Where(model.CommonLogGroupCol()+" = ?", group)
		}
		latestQuery = onlyFinalRetryLogRows(latestQuery, func(tx *gorm.DB, alias string) *gorm.DB {
			prefix := alias + "."
			tx = tx.
				Where(prefix+"type IN ?", []int{model.LogTypeConsume, model.LogTypeError}).
				Where(prefix+"created_at >= ?", since).
				Where(prefix+"model_name <> ?", "")
			if group != "" {
				tx = tx.Where(prefix+model.CommonLogGroupCol()+" = ?", group)
			}
			return tx
		})
		latestErr := latestQuery.
			First(&latest).Error
		if latestErr != nil {
			return nil, latestErr
		}
		stats = append(stats, ChannelModelErrorStats{
			ModelName:     row.ModelName,
			TotalRequests: row.TotalRequests,
			ErrorCount:    row.ErrorCount,
			ErrorRate:     errorRate,
			LatestError:   strings.TrimSpace(latest.Content),
			LatestErrorAt: row.LatestErrorAt,
		})
	}
	return stats, nil
}

func normalizeChannelModelErrorStatsQuery(args ...interface{}) ChannelModelErrorStatsQuery {
	if len(args) == 1 {
		if query, ok := args[0].(ChannelModelErrorStatsQuery); ok {
			return query
		}
	}
	if len(args) == 4 {
		return ChannelModelErrorStatsQuery{
			ChannelID:     toInt(args[0]),
			Threshold:     toInt(args[1]),
			WindowSeconds: toInt64(args[2]),
			Now:           toInt64(args[3]),
		}
	}
	return ChannelModelErrorStatsQuery{}
}

func toInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	default:
		return 0
	}
}

func toInt64(value interface{}) int64 {
	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func CanDisableChannelForModel(channelID int, modelName string, group string) (bool, error) {
	modelName = strings.TrimSpace(modelName)
	group = strings.TrimSpace(group)
	if channelID <= 0 || modelName == "" {
		return false, nil
	}
	query := model.DB.Model(&model.Ability{}).
		Where("model = ? AND channel_id <> ? AND enabled = ?", modelName, channelID, true)
	if group != "" {
		query = query.Where(model.CommonGroupCol()+" = ?", group)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func CanDisableWholeChannelPreservingModels(channelID int) (bool, []string, error) {
	if channelID <= 0 {
		return false, nil, nil
	}
	var abilities []model.Ability
	if err := model.DB.Model(&model.Ability{}).
		Where("channel_id = ? AND enabled = ?", channelID, true).
		Find(&abilities).Error; err != nil {
		return false, nil, err
	}
	missing := make([]string, 0)
	for _, ability := range abilities {
		ok, err := CanDisableChannelForModel(channelID, ability.Model, ability.Group)
		if err != nil {
			return false, nil, err
		}
		if !ok {
			missing = append(missing, ability.Group+"/"+ability.Model)
		}
	}
	sort.Strings(missing)
	return len(missing) == 0, missing, nil
}

func MissingModelKeysForChannel(channelID int) ([]string, error) {
	ok, missing, err := CanDisableWholeChannelPreservingModels(channelID)
	if err != nil || ok {
		return missing, err
	}
	return missing, nil
}

func FindAutoEnableCandidateChannels(query AutoEnableCandidateQuery) ([]*model.Channel, error) {
	query.ModelName = strings.TrimSpace(query.ModelName)
	query.Group = strings.TrimSpace(query.Group)
	if query.ModelName == "" {
		return []*model.Channel{}, nil
	}
	var channels []*model.Channel
	err := model.DB.Model(&model.Channel{}).
		Where("id <> ? AND status = ?", query.FailedChannelID, common.ChannelStatusAutoDisabled).
		Order("priority DESC, id ASC").
		Find(&channels).Error
	if err != nil {
		return nil, err
	}
	candidates := make([]*model.Channel, 0, len(channels))
	for _, channel := range channels {
		if !channelSupportsModel(channel, query.ModelName) || !channelSupportsGroup(channel, query.Group) {
			continue
		}
		candidates = append(candidates, channel)
	}
	return candidates, nil
}

func FindAutoEnableOperationRecordCandidates(query AutoEnableOperationRecordCandidateQuery) ([]AutoEnableOperationRecordCandidate, error) {
	if query.Now <= 0 {
		query.Now = common.GetTimestamp()
	}
	if query.CooldownSeconds < 0 {
		query.CooldownSeconds = 0
	}
	if query.Limit <= 0 {
		query.Limit = 200
	}

	cutoff := query.Now - query.CooldownSeconds
	var records []model.ChannelOperationRecord
	err := model.DB.Model(&model.ChannelOperationRecord{}).
		Where("action = ? AND source = ? AND status = ? AND success = ?",
			model.ChannelOperationActionDisable,
			model.ChannelOperationSourceAuto,
			common.ChannelStatusAutoDisabled,
			true,
		).
		Order("id DESC").
		Limit(query.Limit).
		Find(&records).Error
	if err != nil {
		return nil, err
	}

	seenChannels := map[int]struct{}{}
	candidates := make([]AutoEnableOperationRecordCandidate, 0, len(records))
	for _, record := range records {
		if record.ChannelID <= 0 {
			continue
		}
		if _, exists := seenChannels[record.ChannelID]; exists {
			continue
		}
		seenChannels[record.ChannelID] = struct{}{}
		if record.CreatedAt > cutoff {
			continue
		}

		var channel model.Channel
		if err := model.DB.Where("id = ?", record.ChannelID).First(&channel).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				continue
			}
			return nil, err
		}
		if channel.Status != common.ChannelStatusAutoDisabled || !channel.GetAutoBan() {
			continue
		}

		enabled, err := hasSuccessfulAutoEnableAfter(record)
		if err != nil {
			return nil, err
		}
		if enabled {
			continue
		}
		candidateChannel := channel
		candidates = append(candidates, AutoEnableOperationRecordCandidate{
			Channel: &candidateChannel,
			Record:  record,
		})
	}
	return candidates, nil
}

func hasSuccessfulAutoEnableAfter(record model.ChannelOperationRecord) (bool, error) {
	tx := model.DB.Model(&model.ChannelOperationRecord{}).
		Where("channel_id = ? AND action = ? AND source = ? AND success = ?",
			record.ChannelID,
			model.ChannelOperationActionEnable,
			model.ChannelOperationSourceAuto,
			true,
		)
	if record.Id > 0 {
		tx = tx.Where("id > ?", record.Id)
	} else {
		tx = tx.Where("created_at > ?", record.CreatedAt)
	}
	var count int64
	if err := tx.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func ParseModelGroupKey(key string) (string, string) {
	parts := strings.SplitN(strings.TrimSpace(key), "/", 2)
	if len(parts) != 2 {
		return "", strings.TrimSpace(key)
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func BuildChannelAutoDisableReason(stat ChannelModelErrorStats, windowMinutes int, threshold int) string {
	return fmt.Sprintf(
		"模型 %s 在最近 %d 分钟内错误 %d 次，总请求 %d 次，错误率 %.2f%%，达到自动运营阈值 %d；最近错误：%s",
		stat.ModelName,
		windowMinutes,
		stat.ErrorCount,
		stat.TotalRequests,
		stat.ErrorRate,
		threshold,
		stat.LatestError,
	)
}

func channelSupportsModel(channel *model.Channel, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	for _, configured := range channel.GetModels() {
		if strings.TrimSpace(configured) == modelName {
			return true
		}
	}
	return false
}

func channelSupportsGroup(channel *model.Channel, group string) bool {
	group = strings.TrimSpace(group)
	if group == "" {
		return true
	}
	return slices.Contains(channel.GetGroups(), group)
}
