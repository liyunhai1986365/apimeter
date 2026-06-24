package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	ConfigurableResourceStateStatusActive  = "active"
	ConfigurableResourceStateStatusInvalid = "invalid"
)

type ConfigurableResourceState struct {
	Id           int    `json:"id"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
	UpdatedAt    int64  `json:"updated_at" gorm:"bigint;index"`
	LastUsedAt   int64  `json:"last_used_at" gorm:"bigint;index"`
	ChannelID    int    `json:"channel_id" gorm:"index;uniqueIndex:idx_configurable_resource_state_scope,priority:1"`
	ProfileID    string `json:"profile_id" gorm:"type:varchar(128);uniqueIndex:idx_configurable_resource_state_scope,priority:2"`
	ResourceID   string `json:"resource_id" gorm:"type:varchar(128);uniqueIndex:idx_configurable_resource_state_scope,priority:3"`
	PreRequestID string `json:"pre_request_id" gorm:"type:varchar(128);uniqueIndex:idx_configurable_resource_state_scope,priority:4"`
	UserID       int    `json:"user_id" gorm:"uniqueIndex:idx_configurable_resource_state_scope,priority:5"`
	TokenID      int    `json:"token_id" gorm:"uniqueIndex:idx_configurable_resource_state_scope,priority:6"`
	StateKey     string `json:"state_key" gorm:"type:varchar(128);uniqueIndex:idx_configurable_resource_state_scope,priority:7"`
	StateValue   string `json:"state_value" gorm:"type:varchar(512)"`
	Status       string `json:"status" gorm:"type:varchar(32);index"`
	FailCount    int    `json:"fail_count" gorm:"default:0"`
	Metadata     string `json:"metadata" gorm:"type:text"`
}

func FindActiveConfigurableResourceState(channelID int, profileID, resourceID, preRequestID string, userID, tokenID int, stateKey string) (*ConfigurableResourceState, error) {
	var state ConfigurableResourceState
	err := DB.Where(
		"channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = ? AND token_id = ? AND state_key = ? AND status = ?",
		channelID,
		strings.TrimSpace(profileID),
		strings.TrimSpace(resourceID),
		strings.TrimSpace(preRequestID),
		userID,
		tokenID,
		strings.TrimSpace(stateKey),
		ConfigurableResourceStateStatusActive,
	).First(&state).Error
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func UpsertConfigurableResourceState(state *ConfigurableResourceState) error {
	if state == nil {
		return nil
	}
	now := common.GetTimestamp()
	if state.CreatedAt == 0 {
		state.CreatedAt = now
	}
	state.UpdatedAt = now
	state.LastUsedAt = now
	state.ProfileID = strings.TrimSpace(state.ProfileID)
	state.ResourceID = strings.TrimSpace(state.ResourceID)
	state.PreRequestID = strings.TrimSpace(state.PreRequestID)
	state.StateKey = strings.TrimSpace(state.StateKey)
	if strings.TrimSpace(state.Status) == "" {
		state.Status = ConfigurableResourceStateStatusActive
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "profile_id"},
			{Name: "resource_id"},
			{Name: "pre_request_id"},
			{Name: "user_id"},
			{Name: "token_id"},
			{Name: "state_key"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"state_value", "status", "fail_count", "metadata", "updated_at", "last_used_at"}),
	}).Create(state).Error
}

func MarkConfigurableResourceStateInvalid(state *ConfigurableResourceState) error {
	if state == nil || state.Id <= 0 {
		return nil
	}
	return DB.Model(&ConfigurableResourceState{}).
		Where("id = ?", state.Id).
		Updates(map[string]any{
			"status":     ConfigurableResourceStateStatusInvalid,
			"fail_count": gorm.Expr("fail_count + ?", 1),
			"updated_at": common.GetTimestamp(),
		}).Error
}
