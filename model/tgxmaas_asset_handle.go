package model

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Keep aliases in the existing persistent resource state store. They are scoped
// by gateway user, supplier channel and project; never resolve another user's
// original ID through a shared upstream credential.
func tgxMaasHandleKey(project, id string) string {
	if project == "" {
		project = "default"
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(project+"\x00"+id)))
}

func SaveTgxMaasAssetHandle(channelID, userID int, project, originalID, providerID string) error {
	return SaveTgxMaasAssetHandleContext(context.Background(), channelID, userID, project, originalID, providerID)
}

// Unchanged supplier mappings are reads, including mappings recorded before
// this optimization. Do not refresh their timestamps on every poll.
func SaveTgxMaasAssetHandleContext(ctx context.Context, channelID, userID int, project, originalID, providerID string) error {
	if originalID == "" || providerID == "" || originalID == providerID {
		return nil
	}
	db := DB.WithContext(ctx)
	key := tgxMaasHandleKey(project, originalID)
	var stored ConfigurableResourceState
	result := db.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = ? AND token_id = 0 AND state_key = ?",
		channelID, "seedance-tgxmaas", "asset_handle", "original_id", userID, key).Limit(1).Find(&stored)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 && stored.StateValue == providerID && stored.Status == ConfigurableResourceStateStatusActive {
		return nil
	}
	now := common.GetTimestamp()
	state := ConfigurableResourceState{
		ChannelID: channelID, ProfileID: "seedance-tgxmaas", ResourceID: "asset_handle",
		PreRequestID: "original_id", UserID: userID,
		StateKey: key, StateValue: providerID, Status: ConfigurableResourceStateStatusActive,
		CreatedAt: now, UpdatedAt: now, LastUsedAt: now,
	}
	// Preserve mapping changes returned by the supplier, scoped to this user
	// and account. Only a new/changed response reaches this write.
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "channel_id"}, {Name: "profile_id"}, {Name: "resource_id"}, {Name: "pre_request_id"}, {Name: "user_id"}, {Name: "token_id"}, {Name: "state_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"state_value", "status", "updated_at", "last_used_at"}),
	}).Create(&state).Error
	// Keep the transaction: closing a cancelled MySQL connection does not
	// guarantee an already submitted autocommit write will stop on the server.
	// GORM may append a rollback error that masks the original context error.
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func ResolveTgxMaasAssetHandle(channelID, userID int, project, id string) (string, error) {
	if !strings.HasPrefix(id, "asset-") && !strings.HasPrefix(id, "group-") {
		return id, nil
	}
	state, err := FindActiveConfigurableResourceState(channelID, "seedance-tgxmaas", "asset_handle", "original_id", userID, 0, tgxMaasHandleKey(project, id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return id, nil
	} // New supplier versions may accept original handles directly.
	if err != nil {
		return "", err
	}
	return state.StateValue, nil
}

// Explicit backends have independent account/endpoint scopes. Legacy channels
// retain their existing lookup keys. Replacing dedicated credentials invalidates
// aliases conservatively rather than applying an old account's IDs to a new one.
func (ch *Channel) AssetHandleProject(project string) string {
	p := ch.GetSetting().Protocol
	if p == nil || p.AssetLibrary == nil || p.AssetLibrary.Backend == "" || p.AssetLibrary.Backend == "inherit" {
		return project
	}
	if project == "" {
		project = "default"
	}
	cfg := p.AssetLibrary
	endpoint := cfg.BaseURL
	if endpoint == "" {
		endpoint = ch.GetBaseURL()
	}
	credential := ch.AssetSecret
	if cfg.AuthMode == "channel_key" {
		credential = ch.Key
	}
	scope := sha256.Sum256([]byte(cfg.Backend + "\x00" + strings.TrimRight(endpoint, "/") + "\x00" + cfg.AuthMode + "\x00" + credential))
	return fmt.Sprintf("%s\x00%x", project, scope)
}

func (ch *Channel) AssetStateKey(key string) string {
	scoped := ch.AssetHandleProject(key)
	if scoped == key {
		return key
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(scoped)))
}
