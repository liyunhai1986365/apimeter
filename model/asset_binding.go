package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const assetBindingProfile = "asset-access-v1"
const assetLibraryScopeProfile = "asset-library-scope-v1"
const assetBindingLockProfile = "asset-access-lock-v1"

var ErrAssetNotOwned = errors.New("asset resource is unknown or not owned by this user")

// AssetBinding reuses ConfigurableResourceState; it does not add a table or
// column. UserID/TokenID on the stored row are zero so its existing unique key
// enforces one owner per upstream handle, including concurrent registrations.
// StateValue holds the owner, allowing portable queries without JSON SQL.
type AssetBinding struct {
	ChannelID   int    `json:"channel_id"`
	UserID      int    `json:"user_id"`
	Backend     string `json:"backend"`
	Scope       string `json:"scope"`
	Kind        string `json:"kind"`
	ID          string `json:"id,omitempty"`
	CanonicalID string `json:"canonical_id,omitempty"`
	Project     string `json:"project,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	ExpiresAt   int64  `json:"expires_at,omitempty"`
}

func assetBindingKey(id string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(id))) }

// ResolveAssetLibraryScope assigns one persistent identity to a channel's asset
// endpoint. The first value is its legacy credential scope, preserving existing
// ownership without rewriting rows. Later credential rotations keep that value.
func ResolveAssetLibraryScope(channelID int, endpointKey, initialScope string) (string, error) {
	if channelID <= 0 || endpointKey == "" || initialScope == "" {
		return "", fmt.Errorf("invalid asset library scope")
	}
	var stored ConfigurableResourceState
	lookup := func() *gorm.DB {
		return DB.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key = ?",
			channelID, assetLibraryScopeProfile, "scope", "endpoint", endpointKey).Limit(1).Find(&stored)
	}
	result := lookup()
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		now := common.GetTimestamp()
		state := ConfigurableResourceState{
			ChannelID: channelID, ProfileID: assetLibraryScopeProfile,
			ResourceID: "scope", PreRequestID: "endpoint", StateKey: endpointKey,
			StateValue: initialScope, Status: ConfigurableResourceStateStatusActive,
			CreatedAt: now, UpdatedAt: now, LastUsedAt: now,
		}
		// Concurrent first use must agree on a single identity, never replace it.
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
			return "", err
		}
		if result = lookup(); result.Error != nil {
			return "", result.Error
		}
	}
	if stored.Status != ConfigurableResourceStateStatusActive || stored.StateValue == "" {
		return "", fmt.Errorf("asset library scope is unavailable")
	}
	return stored.StateValue, nil
}

func SaveAssetBinding(binding AssetBinding, handle string) error {
	binding.ID = handle
	return SaveAssetBindings([]AssetBinding{binding})
}

// SaveAssetBindings registers or hydrates a complete alias set atomically.
func SaveAssetBindings(bindings []AssetBinding) error {
	return registerAssetBindings(bindings, false)
}

// Lock persistent, global handle rows before reading any ownership snapshots.
// An asset and its asynchronous task handle share the same lock namespace.
// The existing ownership unique key alone is insufficient across channels.
func lockAssetBindings(tx *gorm.DB, bindings []AssetBinding) error {
	keys := make(map[string]bool)
	for _, binding := range bindings {
		kind := binding.Kind
		if kind == "task" {
			kind = "asset"
		}
		keys[kind+":"+assetBindingKey(binding.ID)] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	for _, key := range ordered {
		state := ConfigurableResourceState{ProfileID: assetBindingLockProfile,
			ResourceID: "handle", PreRequestID: "global", StateKey: key,
			Status: ConfigurableResourceStateStatusActive}
		// An upsert also acquires SQLite's write lock before any reads. Updating
		// an existing row avoids shared-lock upgrades under MySQL contention.
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "profile_id"}, {Name: "resource_id"}, {Name: "pre_request_id"}, {Name: "user_id"}, {Name: "token_id"}, {Name: "state_key"}},
			DoUpdates: clause.AssignmentColumns([]string{"status"}),
		}).Create(&state).Error; err != nil {
			return err
		}
		if err := lockForUpdate(tx).Where("channel_id = 0 AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key = ?",
			assetBindingLockProfile, "handle", "global", key).First(&ConfigurableResourceState{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func saveAssetBinding(tx *gorm.DB, binding AssetBinding, handle string) error {
	if binding.UserID <= 0 || binding.ChannelID <= 0 || binding.Scope == "" || handle == "" {
		return fmt.Errorf("invalid asset binding")
	}
	if binding.Kind == "session" {
		// BytedToken is a bearer credential. Persist only its digest and expiry.
		binding.ID, binding.CanonicalID = "", ""
	} else {
		binding.ID = handle
		if binding.CanonicalID == "" {
			binding.CanonicalID = handle
		}
	}
	metadata, err := common.Marshal(binding)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	state := ConfigurableResourceState{
		ChannelID: binding.ChannelID, ProfileID: assetBindingProfile,
		ResourceID: binding.Kind, PreRequestID: binding.Scope,
		StateKey: assetBindingKey(handle), StateValue: strconv.Itoa(binding.UserID),
		Status: ConfigurableResourceStateStatusActive, Metadata: string(metadata),
		CreatedAt: now, UpdatedAt: now, LastUsedAt: now,
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&state).Error; err != nil {
		return err
	}
	var stored ConfigurableResourceState
	if err := lockForUpdate(tx).Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key = ?",
		binding.ChannelID, assetBindingProfile, binding.Kind, binding.Scope, state.StateKey).First(&stored).Error; err != nil {
		return err
	}
	if stored.StateValue != state.StateValue || stored.Status != ConfigurableResourceStateStatusActive {
		return ErrAssetNotOwned
	}
	// Do not extend a session's expiry on a repeated query/registration.
	if binding.Kind == "session" {
		return nil
	}
	var existing AssetBinding
	if err := common.UnmarshalJsonStr(stored.Metadata, &existing); err != nil {
		return err
	}
	if (existing.Project != "" && binding.Project != "" && existing.Project != binding.Project) ||
		(existing.GroupID != "" && binding.GroupID != "" && existing.GroupID != binding.GroupID) {
		return ErrAssetNotOwned
	}
	if existing.Project == "" {
		existing.Project = binding.Project
	}
	if existing.GroupID == "" {
		existing.GroupID = binding.GroupID
	}
	// An async handle can remain canonical after its final asset ID arrives.
	// Hydrating metadata must never replace that established identity.
	metadata, err = common.Marshal(existing)
	if err != nil {
		return err
	}
	if string(metadata) == stored.Metadata {
		return nil
	}
	return tx.Model(&stored).Updates(map[string]any{"metadata": string(metadata), "updated_at": now}).Error
}

// HasAssetBinding includes other owners, revoked records and old account scopes.
// None of these are unregistered historical handles eligible for first use.
func HasAssetBinding(kind, handle string) (bool, error) {
	var count int64
	err := assetHandleStates(DB, kind, handle).Count(&count).Error
	return count > 0, err
}

func assetHandleStates(tx *gorm.DB, kind, handle string) *gorm.DB {
	kinds := []string{kind}
	if kind == "asset" || kind == "task" {
		kinds = []string{"asset", "task"}
	}
	return tx.Model(&ConfigurableResourceState{}).Where("profile_id = ? AND resource_id IN ? AND state_key = ?", assetBindingProfile, kinds, assetBindingKey(handle))
}

// ClaimLegacyAssetBindings saves all verified aliases in one transaction. A
// conflicting alias must roll back the entire claim, including the supplied ID.
func ClaimLegacyAssetBindings(bindings []AssetBinding) error {
	return registerAssetBindings(bindings, true)
}

func registerAssetBindings(bindings []AssetBinding, legacy bool) error {
	for _, binding := range bindings {
		if binding.UserID <= 0 || binding.ChannelID <= 0 || binding.Scope == "" || binding.ID == "" {
			return fmt.Errorf("invalid asset binding")
		}
		if binding.Kind != "asset" && binding.Kind != "group" && binding.Kind != "task" && (legacy || binding.Kind != "session") {
			return fmt.Errorf("unsupported asset kind")
		}
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := lockAssetBindings(tx, bindings); err != nil {
			return err
		}
		for _, binding := range bindings {
			var states []ConfigurableResourceState
			if err := assetHandleStates(tx, binding.Kind, binding.ID).Find(&states).Error; err != nil {
				return err
			}
			for _, state := range states {
				// New resources may reuse provider-local IDs in unrelated accounts.
				// Legacy claims require a globally unregistered handle; ordinary
				// writes still cannot add a second owner in the same account scope.
				if !legacy && state.PreRequestID != binding.Scope {
					continue
				}
				if state.ChannelID != binding.ChannelID || state.PreRequestID != binding.Scope || state.StateValue != strconv.Itoa(binding.UserID) || state.Status != ConfigurableResourceStateStatusActive {
					return ErrAssetNotOwned
				}
			}
			if err := saveAssetBinding(tx, binding, binding.ID); err != nil {
				return err
			}
		}
		return nil
	})
}

func FindAssetBindings(userID int, kind, handle string) ([]AssetBinding, error) {
	var states []ConfigurableResourceState
	err := DB.Where("profile_id = ? AND resource_id = ? AND state_key = ? AND state_value = ? AND status = ?",
		assetBindingProfile, kind, assetBindingKey(handle), strconv.Itoa(userID), ConfigurableResourceStateStatusActive).Find(&states).Error
	if err != nil {
		return nil, err
	}
	return decodeAssetBindings(states)
}

func ListAssetBindings(channelID, userID int, scope, kind string) ([]AssetBinding, error) {
	var states []ConfigurableResourceState
	err := DB.Where("channel_id = ? AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND state_value = ? AND status = ?",
		channelID, assetBindingProfile, kind, scope, strconv.Itoa(userID), ConfigurableResourceStateStatusActive).Find(&states).Error
	if err != nil {
		return nil, err
	}
	return decodeAssetBindings(states)
}

func decodeAssetBindings(states []ConfigurableResourceState) ([]AssetBinding, error) {
	bindings := make([]AssetBinding, 0, len(states))
	for _, state := range states {
		var binding AssetBinding
		if err := common.UnmarshalJsonStr(state.Metadata, &binding); err != nil {
			return nil, fmt.Errorf("invalid stored asset binding: %w", err)
		}
		if binding.ExpiresAt > 0 && binding.ExpiresAt <= common.GetTimestamp() {
			continue
		}
		bindings = append(bindings, binding)
	}
	return bindings, nil
}

func InvalidateAssetBindings(binding AssetBinding, deleteGroup bool) error {
	var states []ConfigurableResourceState
	if err := DB.Where("channel_id = ? AND profile_id = ? AND pre_request_id = ? AND state_value = ? AND status = ?",
		binding.ChannelID, assetBindingProfile, binding.Scope, strconv.Itoa(binding.UserID), ConfigurableResourceStateStatusActive).Find(&states).Error; err != nil {
		return err
	}
	var ids []int
	for _, state := range states {
		var item AssetBinding
		if err := common.UnmarshalJsonStr(state.Metadata, &item); err != nil {
			return err
		}
		sameKind := item.Kind == binding.Kind || (binding.Kind == "asset" && item.Kind == "task")
		if (sameKind && item.CanonicalID == binding.CanonicalID) || (deleteGroup && item.GroupID == binding.CanonicalID) {
			ids = append(ids, state.Id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return DB.Model(&ConfigurableResourceState{}).Where("id IN ?", ids).
		Updates(map[string]any{"status": ConfigurableResourceStateStatusInvalid, "updated_at": common.GetTimestamp()}).Error
}
