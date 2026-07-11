package model

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	NameRuleExact = iota
	NameRulePrefix
	NameRuleContains
	NameRuleSuffix
)

type BoundChannel struct {
	Name string `json:"name"`
	Type int    `json:"type"`
}

type Model struct {
	Id               int            `json:"id"`
	ModelName        string         `json:"model_name" gorm:"size:128;not null;uniqueIndex:uk_model_name_delete_at,priority:1"`
	Description      string         `json:"description,omitempty" gorm:"type:text"`
	Icon             string         `json:"icon,omitempty" gorm:"type:varchar(128)"`
	Tags             string         `json:"tags,omitempty" gorm:"type:varchar(255)"`
	Category         string         `json:"category,omitempty" gorm:"type:varchar(32);default:'text'"`
	VendorID         int            `json:"vendor_id,omitempty" gorm:"index"`
	AliasModels      string         `json:"alias_models,omitempty" gorm:"type:text"`
	Endpoints        string         `json:"endpoints,omitempty" gorm:"type:text"`
	SortOrder        int            `json:"sort_order" gorm:"type:int;default:100;index"`
	ContextLength    int            `json:"context_length,omitempty" gorm:"type:int;default:0"`
	MaxOutputTokens  int            `json:"max_output_tokens,omitempty" gorm:"type:int;default:0"`
	KnowledgeCutoff  string         `json:"knowledge_cutoff,omitempty" gorm:"type:varchar(32)"`
	ReleaseDate      string         `json:"release_date,omitempty" gorm:"type:varchar(32)"`
	ParameterCount   string         `json:"parameter_count,omitempty" gorm:"type:varchar(64)"`
	InputModalities  string         `json:"input_modalities,omitempty" gorm:"type:varchar(255)"`
	OutputModalities string         `json:"output_modalities,omitempty" gorm:"type:varchar(255)"`
	Capabilities     string         `json:"capabilities,omitempty" gorm:"type:varchar(255)"`
	Status           int            `json:"status" gorm:"default:1"`
	SyncOfficial     int            `json:"sync_official" gorm:"default:1"`
	CreatedTime      int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime      int64          `json:"updated_time" gorm:"bigint"`
	DeletedAt        gorm.DeletedAt `json:"-" gorm:"index;uniqueIndex:uk_model_name_delete_at,priority:2"`

	BoundChannels []BoundChannel `json:"bound_channels,omitempty" gorm:"-"`
	EnableGroups  []string       `json:"enable_groups,omitempty" gorm:"-"`
	QuotaTypes    []int          `json:"quota_types,omitempty" gorm:"-"`
	NameRule      int            `json:"name_rule" gorm:"default:0"`

	MatchedModels []string `json:"matched_models,omitempty" gorm:"-"`
	MatchedCount  int      `json:"matched_count,omitempty" gorm:"-"`
}

func (mi *Model) Insert() error {
	now := common.GetTimestamp()
	mi.CreatedTime = now
	mi.UpdatedTime = now
	if mi.SortOrder == 0 {
		mi.SortOrder = DefaultSortOrder
	}

	// 保存原始值（因为 Create 后可能被 GORM 的 default 标签覆盖为 1）
	originalStatus := mi.Status
	originalSyncOfficial := mi.SyncOfficial

	// 先创建记录（GORM 会对零值字段应用默认值）
	if err := DB.Create(mi).Error; err != nil {
		return err
	}

	// 使用保存的原始值进行更新，确保零值能正确保存
	return DB.Model(&Model{}).Where("id = ?", mi.Id).Updates(map[string]interface{}{
		"status":        originalStatus,
		"sync_official": originalSyncOfficial,
	}).Error
}

func IsModelNameDuplicated(id int, name string) (bool, error) {
	if name == "" {
		return false, nil
	}
	var cnt int64
	err := DB.Model(&Model{}).Where("model_name = ? AND id <> ?", name, id).Count(&cnt).Error
	return cnt > 0, err
}

func (mi *Model) Update() error {
	mi.UpdatedTime = common.GetTimestamp()
	// 使用 Select 强制更新所有字段，包括零值
	return DB.Model(&Model{}).Where("id = ?", mi.Id).
		Select("model_name", "description", "icon", "tags", "category", "vendor_id", "alias_models", "endpoints", "sort_order", "context_length", "max_output_tokens", "knowledge_cutoff", "release_date", "parameter_count", "input_modalities", "output_modalities", "capabilities", "status", "sync_official", "name_rule", "updated_time").
		Updates(mi).Error
}

func (mi *Model) Delete() error {
	return DB.Delete(mi).Error
}

func GetVendorModelCounts() (map[int64]int64, error) {
	var stats []struct {
		VendorID int64
		Count    int64
	}
	if err := DB.Model(&Model{}).
		Select("vendor_id as vendor_id, count(*) as count").
		Group("vendor_id").
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	m := make(map[int64]int64, len(stats))
	for _, s := range stats {
		m[s.VendorID] = s.Count
	}
	return m, nil
}

func GetAllModels(offset int, limit int) ([]*Model, error) {
	models, _, err := SearchModels("", "", "", "", offset, limit)
	return models, err
}

func GetBoundChannelsByModelsMap(modelNames []string) (map[string][]BoundChannel, error) {
	result := make(map[string][]BoundChannel)
	if len(modelNames) == 0 {
		return result, nil
	}
	type row struct {
		Model string
		Name  string
		Type  int
	}
	var rows []row
	err := DB.Table("channels").
		Select("abilities.model as model, channels.name as name, channels.type as type").
		Joins("JOIN abilities ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ?", modelNames, true).
		Distinct().
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.Model] = append(result[r.Model], BoundChannel{Name: r.Name, Type: r.Type})
	}
	return result, nil
}

func normalizeLookupValues(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func GetModelPrimaryNameMap(candidateNames []string) (map[string]string, error) {
	candidateNames = normalizeLookupValues(candidateNames)
	result := make(map[string]string, len(candidateNames))

	var models []Model
	if err := DB.Order("sort_order ASC").Order("id ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	for _, modelItem := range models {
		primary := strings.TrimSpace(modelItem.ModelName)
		if primary == "" {
			continue
		}
		if _, exists := result[primary]; !exists {
			result[primary] = primary
		}
		for _, alias := range parseAliasModels(modelItem.AliasModels) {
			if alias == primary {
				continue
			}
			if _, exists := result[alias]; !exists {
				result[alias] = primary
			}
		}
	}

	for _, candidate := range candidateNames {
		if candidate == "" {
			continue
		}
		if _, exists := result[candidate]; exists {
			continue
		}
		for _, modelItem := range models {
			primary := strings.TrimSpace(modelItem.ModelName)
			if primary == "" || candidate == primary || modelItem.NameRule == NameRuleExact {
				continue
			}
			if modelNameRuleMatches(candidate, primary, modelItem.NameRule) {
				result[candidate] = primary
				break
			}
		}
	}

	return result, nil
}

func modelNameRuleMatches(candidate string, primary string, rule int) bool {
	switch rule {
	case NameRulePrefix:
		return strings.HasPrefix(candidate, primary)
	case NameRuleSuffix:
		return strings.HasSuffix(candidate, primary)
	case NameRuleContains:
		return strings.Contains(candidate, primary)
	default:
		return candidate == primary
	}
}

func GetPreferredModelOwnerChannelTypes(modelNames []string, groups []string) (map[string]int, error) {
	result := make(map[string]int)
	modelNames = normalizeLookupValues(modelNames)
	if len(modelNames) == 0 {
		return result, nil
	}

	type row struct {
		Model       string
		ChannelType int
	}
	var rows []row

	query := DB.Table("abilities").
		Select("abilities.model as model, channels.type as channel_type").
		Joins("JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities.model IN ? AND abilities.enabled = ? AND channels.status = ?", modelNames, true, common.ChannelStatusEnabled).
		Order("COALESCE(abilities.priority, 0) DESC").
		Order("abilities.weight DESC").
		Order("abilities.channel_id ASC")

	groups = normalizeLookupValues(groups)
	if len(groups) > 0 {
		query = query.Where("abilities."+commonGroupCol+" IN ?", groups)
	}

	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, r := range rows {
		if _, ok := result[r.Model]; ok {
			continue
		}
		result[r.Model] = r.ChannelType
	}
	return result, nil
}

func SearchModels(keyword string, vendor string, status string, syncOfficial string, offset int, limit int) ([]*Model, int64, error) {
	var models []*Model
	db := DB.Model(&Model{})
	if keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where("model_name LIKE ? OR description LIKE ? OR tags LIKE ?", like, like, like)
	}
	if vendor != "" {
		if vid, err := strconv.Atoi(vendor); err == nil {
			db = db.Where("models.vendor_id = ?", vid)
		} else {
			db = db.Joins("JOIN vendors ON vendors.id = models.vendor_id").Where("vendors.name LIKE ?", "%"+vendor+"%")
		}
	}
	if statusValue, ok := parseModelStatusFilter(status); ok {
		db = db.Where("models.status = ?", statusValue)
	}
	if syncValue, ok := parseModelSyncFilter(syncOfficial); ok {
		db = db.Where("models.sync_official = ?", syncValue)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := db.Order("models.sort_order ASC").Order("models.id ASC").Offset(offset).Limit(limit).Find(&models).Error; err != nil {
		return nil, 0, err
	}
	return models, total, nil
}

func parseModelStatusFilter(status string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "all":
		return 0, false
	case "enabled", "1":
		return 1, true
	case "disabled", "0":
		return 0, true
	default:
		value, err := strconv.Atoi(status)
		return value, err == nil
	}
}

func parseModelSyncFilter(syncOfficial string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(syncOfficial)) {
	case "", "all":
		return 0, false
	case "yes", "1":
		return 1, true
	case "no", "0":
		return 0, true
	default:
		value, err := strconv.Atoi(syncOfficial)
		return value, err == nil
	}
}
