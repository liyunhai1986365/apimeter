package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type NewAPISupplierChannelProfileSyncResult struct {
	Profiles []model.NewAPISupplierChannelProfile `json:"profiles"`
	Created  int                                  `json:"created"`
	Updated  int                                  `json:"updated"`
}

type NewAPISupplierChannelProfileUpdateRequest struct {
	LocalGroup          string   `json:"local_group"`
	ChannelType         int      `json:"channel_type"`
	ChannelNameTemplate string   `json:"channel_name_template"`
	Tag                 string   `json:"tag"`
	Weight              *uint    `json:"weight"`
	Priority            *int64   `json:"priority"`
	AutoBan             int      `json:"auto_ban"`
	BalanceThreshold    int      `json:"balance_threshold"`
	ChannelRatio        *float64 `json:"channel_ratio"`
	SyncMode            string   `json:"sync_mode"`
	SyncStatus          string   `json:"sync_status"`
	ChannelStatus       string   `json:"channel_status"`
}

type NewAPISupplierProfileModelTestResultRequest struct {
	ModelName      string `json:"model"`
	Success        bool   `json:"success"`
	ResponseTimeMS int    `json:"response_time_ms"`
	Message        string `json:"message"`
}

func SyncNewAPISupplierChannelProfile(ctx context.Context, profileID int) (*model.NewAPISupplierChannelProfile, error) {
	if profileID <= 0 {
		return nil, fmt.Errorf("缺少供应商渠道配置 ID")
	}
	var profile model.NewAPISupplierChannelProfile
	if err := model.DB.First(&profile, profileID).Error; err != nil {
		return nil, err
	}
	if profile.SyncMode == model.NewAPISupplierSyncModeIgnored {
		return nil, fmt.Errorf("供应商渠道已忽略，不能同步")
	}
	var supplier model.NewAPISupplier
	if err := model.DB.First(&supplier, profile.SupplierID).Error; err != nil {
		return nil, err
	}
	models, err := getNewAPISupplierProfileModelNames(profile.Id)
	if err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("供应商渠道没有可同步模型")
	}
	req := NewAPISupplierConfigureRequest{Items: []NewAPISupplierConfigureItem{{
		UpstreamGroup: profile.UpstreamGroup,
		LocalGroup:    profile.LocalGroup,
		Models:        models,
		ChannelType:   profile.ChannelType,
	}}}
	if err := ensureNewAPISupplierConfigureKeys(ctx, &supplier, req); err != nil {
		return nil, err
	}
	groupKeys := parseStringMap(supplier.GroupKeysJSON)
	key := strings.TrimSpace(groupKeys[profile.UpstreamGroup])
	if key == "" {
		key = strings.TrimSpace(supplier.APIKey)
	}
	if key == "" {
		key = strings.TrimSpace(supplier.AccessToken)
	}
	if key == "" {
		return nil, fmt.Errorf("供应商分组 %s 缺少可用令牌", profile.UpstreamGroup)
	}
	channelType := profile.ChannelType
	if channelType == 0 {
		channelType = supplier.ChannelType
	}
	if channelType == 0 {
		channelType = constant.ChannelTypeOpenAI
	}
	localGroup := strings.TrimSpace(profile.LocalGroup)
	if localGroup == "" {
		localGroup = profile.UpstreamGroup
	}
	baseURL := strings.TrimRight(strings.TrimSpace(firstNonEmpty(profile.BaseURLSnapshot, supplier.BaseURL)), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("供应商 Base URL 不能为空")
	}
	channelRatio := profile.ChannelRatio
	if channelRatio == nil {
		channelRatio = parseNewAPISupplierChannelRatio(profile.UpstreamGroupRatio)
	}
	tag := firstNonEmpty(profile.Tag, supplier.Tag, supplier.Name)
	channelStatus := common.ChannelStatusEnabled
	if profile.ChannelStatus == model.NewAPISupplierChannelStatusUnavailable {
		channelStatus = common.ChannelStatusManuallyDisabled
	}
	modelCSV := strings.Join(models, ",")
	channelName := renderNewAPISupplierProfileChannelName(profile, supplier)
	now := common.GetTimestamp()
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		channel, created, err := loadOrCreateNewAPISupplierProfileChannel(tx, profile, channelName)
		if err != nil {
			return err
		}
		channel.Type = channelType
		channel.Key = key
		channel.Status = channelStatus
		channel.BaseURL = common.GetPointer(baseURL)
		channel.Models = modelCSV
		channel.Group = localGroup
		channel.Balance = float64(supplier.Quota)
		channel.Tag = common.GetPointer(tag)
		channel.Weight = profile.Weight
		channel.ChannelRatio = channelRatio
		channel.Priority = profile.Priority
		channel.AutoBan = common.GetPointer(profile.AutoBan)
		channel.OtherInfo = fmt.Sprintf("newapi-supplier:%d;profile:%d;upstream_group:%s", supplier.Id, profile.Id, profile.UpstreamGroup)
		if created {
			channel.CreatedTime = now
			if err := tx.Create(channel).Error; err != nil {
				return err
			}
			if err := channel.AddAbilities(tx); err != nil {
				return err
			}
		} else {
			if err := tx.Save(channel).Error; err != nil {
				return err
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
		}
		if err := upsertNewAPISupplierChannelBinding(tx, supplier.Id, *channel, profile.UpstreamGroup, now); err != nil {
			return err
		}
		syncStatus := model.NewAPISupplierChannelSyncStatusSynced
		if created {
			syncStatus = model.NewAPISupplierChannelSyncStatusCreated
		}
		return tx.Model(&model.NewAPISupplierChannelProfile{}).Where("id = ?", profile.Id).Updates(map[string]any{
			"channel_id":     channel.Id,
			"channel_type":   channel.Type,
			"local_group":    channel.Group,
			"sync_status":    syncStatus,
			"last_synced_at": now,
			"last_error":     "",
			"updated_time":   now,
		}).Error
	})
	if err != nil {
		_ = model.DB.Model(&model.NewAPISupplierChannelProfile{}).Where("id = ?", profile.Id).Updates(map[string]any{
			"sync_status":  model.NewAPISupplierChannelSyncStatusFailed,
			"last_error":   err.Error(),
			"updated_time": common.GetTimestamp(),
		}).Error
		return nil, err
	}
	model.InitChannelCache()
	ResetProxyClientCache()
	var updated model.NewAPISupplierChannelProfile
	if err := model.DB.First(&updated, profile.Id).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func SyncNewAPISupplierChannelProfiles(supplier *model.NewAPISupplier, snapshots []NewAPISupplierGroupSnapshot) (*NewAPISupplierChannelProfileSyncResult, error) {
	if supplier == nil {
		return nil, fmt.Errorf("supplier is nil")
	}
	supplier.Normalize()
	result := &NewAPISupplierChannelProfileSyncResult{}
	now := common.GetTimestamp()
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.Group) == "" || strings.TrimSpace(snapshot.Source) != "pricing" {
			continue
		}
		models := normalizeStringList(snapshot.Models)
		if len(models) == 0 {
			continue
		}
		channelType := supplier.ChannelType
		if channelType == 0 {
			channelType = constant.ChannelTypeOpenAI
		}
		endpointType := string(firstEndpointType(channelType, firstString(models)))
		localGroup := strings.TrimSpace(supplier.DefaultLocalGroup)
		if localGroup == "" {
			localGroup = snapshot.Group
		}
		channelRatio := parseNewAPISupplierChannelRatio(snapshot.Ratio)
		profile := model.NewAPISupplierChannelProfile{
			SupplierID:           supplier.Id,
			SupplierNameSnapshot: supplier.Name,
			BaseURLSnapshot:      supplier.BaseURL,
			UpstreamGroup:        strings.TrimSpace(snapshot.Group),
			UpstreamGroupDesc:    strings.TrimSpace(snapshot.Desc),
			UpstreamGroupRatio:   strings.TrimSpace(snapshot.Ratio),
			ModelSource:          "pricing",
			ChannelType:          channelType,
			EndpointType:         endpointType,
			LocalGroup:           localGroup,
			ChannelNameTemplate:  "{group} - {ratio} - {channel_type} - {supplier}",
			Tag:                  firstNonEmpty(strings.TrimSpace(supplier.Tag), supplier.Name),
			Weight:               supplier.Weight,
			Priority:             supplier.Priority,
			AutoBan:              supplier.AutoBan,
			BalanceThreshold:     supplier.BalanceThreshold,
			ChannelRatio:         channelRatio,
			SyncMode:             model.NewAPISupplierSyncModeManaged,
			SyncStatus:           model.NewAPISupplierChannelSyncStatusPending,
			ChannelStatus:        model.NewAPISupplierChannelStatusAvailable,
			ConfigHash:           hashStrings(supplier.BaseURL, snapshot.Group, snapshot.Ratio, endpointType, localGroup, strconv.Itoa(channelType)),
			ModelSetHash:         hashStrings(models...),
			LastCheckedAt:        now,
			UpdatedTime:          now,
		}
		var existing model.NewAPISupplierChannelProfile
		err := model.DB.Where("supplier_id = ? AND upstream_group = ? AND channel_type = ? AND endpoint_type = ? AND local_group = ?",
			profile.SupplierID, profile.UpstreamGroup, profile.ChannelType, profile.EndpointType, profile.LocalGroup).
			First(&existing).Error
		switch {
		case err == nil:
			profile.Id = existing.Id
			profile.ChannelID = existing.ChannelID
			profile.CreatedTime = existing.CreatedTime
			profile.LastSyncedAt = existing.LastSyncedAt
			if existing.SyncMode != "" {
				profile.SyncMode = existing.SyncMode
			}
			if existing.SyncStatus != "" && existing.SyncStatus != model.NewAPISupplierChannelSyncStatusFailed {
				profile.SyncStatus = existing.SyncStatus
			}
			if existing.ChannelStatus != "" {
				profile.ChannelStatus = existing.ChannelStatus
			}
			if err := model.DB.Model(&model.NewAPISupplierChannelProfile{}).Where("id = ?", existing.Id).Updates(profile).Error; err != nil {
				return nil, err
			}
			result.Updated++
		case err == gorm.ErrRecordNotFound:
			profile.CreatedTime = now
			if err := model.DB.Create(&profile).Error; err != nil {
				return nil, err
			}
			result.Created++
		default:
			return nil, err
		}
		if err := syncNewAPISupplierChannelProfileModels(&profile, snapshot, models); err != nil {
			return nil, err
		}
		result.Profiles = append(result.Profiles, profile)
	}
	return result, nil
}

func UpdateNewAPISupplierChannelProfile(profileID int, req NewAPISupplierChannelProfileUpdateRequest) (*model.NewAPISupplierChannelProfile, error) {
	if profileID <= 0 {
		return nil, fmt.Errorf("缺少供应商渠道配置 ID")
	}
	var profile model.NewAPISupplierChannelProfile
	if err := model.DB.First(&profile, profileID).Error; err != nil {
		return nil, err
	}
	localGroup := firstNonEmpty(req.LocalGroup, profile.LocalGroup)
	channelType := req.ChannelType
	if channelType == 0 {
		channelType = profile.ChannelType
	}
	if channelType == 0 {
		channelType = constant.ChannelTypeOpenAI
	}
	endpointType := firstNonEmpty(profile.EndpointType, string(firstEndpointType(channelType, "")))
	channelNameTemplate := firstNonEmpty(req.ChannelNameTemplate, profile.ChannelNameTemplate, "{group} - {ratio} - {channel_type} - {supplier}")
	tag := strings.TrimSpace(req.Tag)
	syncMode := normalizeNewAPISupplierProfileSyncMode(firstNonEmpty(req.SyncMode, profile.SyncMode))
	syncStatus := normalizeNewAPISupplierProfileSyncStatus(firstNonEmpty(req.SyncStatus, profile.SyncStatus))
	channelStatus := normalizeNewAPISupplierChannelStatus(firstNonEmpty(req.ChannelStatus, profile.ChannelStatus))
	if syncMode == model.NewAPISupplierSyncModeIgnored {
		channelStatus = model.NewAPISupplierChannelStatusUnavailable
	}
	now := common.GetTimestamp()
	configHash := hashStrings(profile.BaseURLSnapshot, profile.UpstreamGroup, profile.UpstreamGroupRatio, endpointType, localGroup, strconv.Itoa(channelType), syncMode, syncStatus, channelStatus)
	if err := model.DB.Model(&model.NewAPISupplierChannelProfile{}).Where("id = ?", profileID).Updates(map[string]any{
		"channel_type":          channelType,
		"local_group":           localGroup,
		"channel_name_template": channelNameTemplate,
		"tag":                   tag,
		"weight":                req.Weight,
		"priority":              req.Priority,
		"auto_ban":              req.AutoBan,
		"balance_threshold":     req.BalanceThreshold,
		"channel_ratio":         req.ChannelRatio,
		"sync_mode":             syncMode,
		"sync_status":           syncStatus,
		"channel_status":        channelStatus,
		"config_hash":           configHash,
		"last_error":            "",
		"updated_time":          now,
	}).Error; err != nil {
		return nil, err
	}
	var updated model.NewAPISupplierChannelProfile
	if err := model.DB.First(&updated, profileID).Error; err != nil {
		return nil, err
	}
	return &updated, nil
}

func UpdateNewAPISupplierProfileModelTestResult(profileID int, req NewAPISupplierProfileModelTestResultRequest) (*model.NewAPISupplierChannelProfileModel, error) {
	if profileID <= 0 {
		return nil, fmt.Errorf("缺少供应商渠道配置 ID")
	}
	modelName := strings.TrimSpace(req.ModelName)
	if modelName == "" {
		return nil, fmt.Errorf("模型名称不能为空")
	}
	var profile model.NewAPISupplierChannelProfile
	if err := model.DB.First(&profile, profileID).Error; err != nil {
		return nil, err
	}
	status := model.NewAPISupplierProfileModelStatusUnavailable
	lastError := strings.TrimSpace(req.Message)
	if req.Success {
		status = model.NewAPISupplierProfileModelStatusAvailable
		lastError = ""
	}
	now := common.GetTimestamp()
	var item model.NewAPISupplierChannelProfileModel
	err := model.DB.Where("profile_id = ? AND model_name = ?", profileID, modelName).First(&item).Error
	switch {
	case err == nil:
		if err := model.DB.Model(&model.NewAPISupplierChannelProfileModel{}).Where("id = ?", item.Id).Updates(map[string]any{
			"available_status":   status,
			"last_test_success":  req.Success,
			"last_test_time":     now,
			"last_response_time": req.ResponseTimeMS,
			"last_error":         lastError,
			"updated_time":       now,
		}).Error; err != nil {
			return nil, err
		}
	case err == gorm.ErrRecordNotFound:
		item = model.NewAPISupplierChannelProfileModel{
			ProfileID:        profile.Id,
			SupplierID:       profile.SupplierID,
			UpstreamGroup:    profile.UpstreamGroup,
			ModelName:        modelName,
			AvailableStatus:  status,
			LastTestSuccess:  req.Success,
			LastTestTime:     now,
			LastResponseTime: req.ResponseTimeMS,
			LastError:        lastError,
			CreatedTime:      now,
			UpdatedTime:      now,
		}
		if err := model.DB.Create(&item).Error; err != nil {
			return nil, err
		}
	default:
		return nil, err
	}
	var updated model.NewAPISupplierChannelProfileModel
	if err := model.DB.Where("profile_id = ? AND model_name = ?", profileID, modelName).First(&updated).Error; err != nil {
		return nil, err
	}
	if err := RefreshNewAPISupplierChannelProfileStatus(profileID); err != nil {
		return nil, err
	}
	return &updated, nil
}

func RefreshNewAPISupplierChannelProfileStatus(profileID int) error {
	if profileID <= 0 {
		return fmt.Errorf("缺少供应商渠道配置 ID")
	}
	var total int64
	if err := model.DB.Model(&model.NewAPISupplierChannelProfileModel{}).Where("profile_id = ?", profileID).Count(&total).Error; err != nil {
		return err
	}
	status := model.NewAPISupplierChannelStatusUnavailable
	if total > 0 {
		var unavailable int64
		if err := model.DB.Model(&model.NewAPISupplierChannelProfileModel{}).
			Where("profile_id = ? AND available_status <> ?", profileID, model.NewAPISupplierProfileModelStatusAvailable).
			Count(&unavailable).Error; err != nil {
			return err
		}
		if unavailable == 0 {
			status = model.NewAPISupplierChannelStatusAvailable
		}
	}
	now := common.GetTimestamp()
	return model.DB.Model(&model.NewAPISupplierChannelProfile{}).Where("id = ?", profileID).Updates(map[string]any{
		"channel_status":  status,
		"last_checked_at": now,
		"updated_time":    now,
	}).Error
}

func getNewAPISupplierProfileModelNames(profileID int) ([]string, error) {
	var profileModels []model.NewAPISupplierChannelProfileModel
	if err := model.DB.Where("profile_id = ?", profileID).Order("model_name ASC").Find(&profileModels).Error; err != nil {
		return nil, err
	}
	models := make([]string, 0, len(profileModels))
	for _, profileModel := range profileModels {
		models = append(models, profileModel.ModelName)
	}
	return normalizeStringList(models), nil
}

func loadOrCreateNewAPISupplierProfileChannel(tx *gorm.DB, profile model.NewAPISupplierChannelProfile, channelName string) (*model.Channel, bool, error) {
	if profile.ChannelID != nil && *profile.ChannelID > 0 {
		var channel model.Channel
		if err := tx.First(&channel, *profile.ChannelID).Error; err == nil {
			return &channel, false, nil
		} else if err != gorm.ErrRecordNotFound {
			return nil, false, err
		}
	}
	return &model.Channel{
		Name: nextNewAPISupplierChannelName(tx, channelName),
	}, true, nil
}

func renderNewAPISupplierProfileChannelName(profile model.NewAPISupplierChannelProfile, supplier model.NewAPISupplier) string {
	template := strings.TrimSpace(profile.ChannelNameTemplate)
	if template == "" {
		return formatNewAPISupplierChannelName(profile.UpstreamGroup, profile.UpstreamGroupRatio, supplier.Name, profile.ChannelType)
	}
	channelTypeName := strings.TrimSpace(constant.GetChannelTypeName(profile.ChannelType))
	if channelTypeName == "" || channelTypeName == "Unknown" {
		channelTypeName = fmt.Sprintf("类型%d", profile.ChannelType)
	}
	replacer := strings.NewReplacer(
		"{supplier}", supplier.Name,
		"{group}", profile.UpstreamGroup,
		"{ratio}", firstNonEmpty(profile.UpstreamGroupRatio, "未配置倍率"),
		"{channel_type}", channelTypeName,
		"{local_group}", profile.LocalGroup,
	)
	return strings.TrimSpace(replacer.Replace(template))
}

func upsertNewAPISupplierChannelBinding(tx *gorm.DB, supplierID int, channel model.Channel, upstreamGroup string, now int64) error {
	var binding model.NewAPISupplierChannel
	err := tx.Where("supplier_id = ? AND channel_id = ?", supplierID, channel.Id).First(&binding).Error
	switch {
	case err == nil:
		return tx.Model(&model.NewAPISupplierChannel{}).Where("id = ?", binding.Id).Updates(map[string]any{
			"upstream_group": upstreamGroup,
			"channel_type":   channel.Type,
			"local_group":    channel.Group,
			"models":         channel.Models,
			"sync_mode":      model.NewAPISupplierSyncModeManaged,
			"updated_time":   now,
		}).Error
	case err == gorm.ErrRecordNotFound:
		binding = model.NewAPISupplierChannel{
			SupplierID:    supplierID,
			ChannelID:     channel.Id,
			UpstreamGroup: upstreamGroup,
			ChannelType:   channel.Type,
			LocalGroup:    channel.Group,
			Models:        channel.Models,
			SyncMode:      model.NewAPISupplierSyncModeManaged,
			CreatedTime:   now,
			UpdatedTime:   now,
		}
		return tx.Create(&binding).Error
	default:
		return err
	}
}

func syncNewAPISupplierChannelProfileModels(profile *model.NewAPISupplierChannelProfile, snapshot NewAPISupplierGroupSnapshot, models []string) error {
	now := common.GetTimestamp()
	seen := make([]string, 0, len(models))
	for _, modelName := range models {
		seen = append(seen, modelName)
		providers := normalizeStringList(snapshot.ModelProviders[modelName])
		item := model.NewAPISupplierChannelProfileModel{
			ProfileID:       profile.Id,
			SupplierID:      profile.SupplierID,
			UpstreamGroup:   profile.UpstreamGroup,
			ModelName:       modelName,
			ModelProvider:   strings.Join(providers, ", "),
			AvailableStatus: model.NewAPISupplierProfileModelStatusAvailable,
			LastTestSuccess: true,
			LastTestTime:    now,
			UpdatedTime:     now,
		}
		var existing model.NewAPISupplierChannelProfileModel
		err := model.DB.Where("profile_id = ? AND model_name = ?", profile.Id, modelName).First(&existing).Error
		switch {
		case err == nil:
			item.Id = existing.Id
			item.CreatedTime = existing.CreatedTime
			if err := model.DB.Model(&model.NewAPISupplierChannelProfileModel{}).Where("id = ?", existing.Id).Updates(item).Error; err != nil {
				return err
			}
		case err == gorm.ErrRecordNotFound:
			item.CreatedTime = now
			if err := model.DB.Create(&item).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	if len(seen) == 0 {
		return nil
	}
	return model.DB.Model(&model.NewAPISupplierChannelProfileModel{}).
		Where("profile_id = ? AND model_name NOT IN ?", profile.Id, seen).
		Updates(map[string]any{
			"available_status":  model.NewAPISupplierProfileModelStatusUnavailable,
			"last_test_success": false,
			"updated_time":      now,
		}).Error
}

func normalizeNewAPISupplierProfileSyncMode(syncMode string) string {
	switch strings.TrimSpace(syncMode) {
	case model.NewAPISupplierSyncModeManual:
		return model.NewAPISupplierSyncModeManual
	case model.NewAPISupplierSyncModeIgnored:
		return model.NewAPISupplierSyncModeIgnored
	default:
		return model.NewAPISupplierSyncModeManaged
	}
}

func normalizeNewAPISupplierProfileSyncStatus(syncStatus string) string {
	switch strings.TrimSpace(syncStatus) {
	case model.NewAPISupplierChannelSyncStatusCreated:
		return model.NewAPISupplierChannelSyncStatusCreated
	case model.NewAPISupplierChannelSyncStatusSynced:
		return model.NewAPISupplierChannelSyncStatusSynced
	case model.NewAPISupplierChannelSyncStatusFailed:
		return model.NewAPISupplierChannelSyncStatusFailed
	case model.NewAPISupplierChannelSyncStatusDisabled:
		return model.NewAPISupplierChannelSyncStatusDisabled
	default:
		return model.NewAPISupplierChannelSyncStatusPending
	}
}

func normalizeNewAPISupplierChannelStatus(channelStatus string) string {
	switch strings.TrimSpace(channelStatus) {
	case model.NewAPISupplierChannelStatusUnavailable:
		return model.NewAPISupplierChannelStatusUnavailable
	default:
		return model.NewAPISupplierChannelStatusAvailable
	}
}

func firstEndpointType(channelType int, modelName string) constant.EndpointType {
	endpoints := common.GetEndpointTypesByChannelType(channelType, modelName)
	if len(endpoints) == 0 {
		return constant.EndpointTypeOpenAI
	}
	return endpoints[0]
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func hashStrings(values ...string) string {
	normalized := normalizeStringList(values)
	sum := sha256.Sum256([]byte(strings.Join(normalized, "\x00")))
	return fmt.Sprintf("%x", sum[:])
}
