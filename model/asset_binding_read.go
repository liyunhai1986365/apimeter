package model

import (
	"context"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Read bounded pages without holding a transaction or a connection across the
// consumer. Include id in the projection; callers retain their original scope.
func walkAssetStateRows(query *gorm.DB, visit func([]ConfigurableResourceState) error) error {
	ctx := query.Statement.Context
	lastID := 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var states []ConfigurableResourceState
		if err := query.Session(&gorm.Session{}).Where("id > ?", lastID).
			Order("id").Limit(assetBindingBatchSize).Find(&states).Error; err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := visit(states); err != nil {
			return err
		}
		if len(states) < assetBindingBatchSize {
			return ctx.Err()
		}
		lastID = states[len(states)-1].Id
	}
}

// Resolve canonical identities using the existing complete scope index;
// querying by profile + handle alone cannot seek into that index.
func scopedAssetHandleStates(db *gorm.DB, binding AssetBinding, handles []string) *gorm.DB {
	kinds := []string{binding.Kind}
	if assetBindingKind(binding.Kind) == "asset" {
		kinds = []string{"asset", "task"}
	}
	query := assetHandleCandidateQueryScoped(db, kinds, handles, &binding)
	if db.Dialector.Name() == common.DatabaseTypeMySQL {
		query = query.Select("asset_candidates.*")
	}
	return query
}

// Enumerate distinct prefixes of the existing unique index with MySQL's loose
// index scan, then seek each complete scope + digest. Filtering only by handle
// scans every asset and mutex entry because channel/account are not yet known.
// This remains a live query across all accounts, not an authorization cache.
func assetHandleCandidateQuery(db *gorm.DB, kinds, digests []string) *gorm.DB {
	return assetHandleCandidateQueryScoped(db, kinds, digests, nil)
}

func assetHandleCandidateQueryScoped(db *gorm.DB, kinds, digests []string, scope *AssetBinding) *gorm.DB {
	query := db.Model(&ConfigurableResourceState{})
	if len(kinds) == 0 || len(digests) == 0 {
		return query.Where("1 = 0")
	}
	if db.Dialector.Name() != common.DatabaseTypeMySQL {
		query = query.Where("channel_id > 0 AND profile_id = ? AND resource_id IN ? AND state_key IN ?", assetBindingProfile, kinds, digests)
		if scope != nil {
			query = query.Where("channel_id = ? AND pre_request_id = ?", scope.ChannelID, scope.Scope)
		}
		return query
	}
	const prefix = "channel_id, profile_id, resource_id, pre_request_id, user_id, token_id"
	scopes := db.Session(&gorm.Session{NewDB: true}).Model(&ConfigurableResourceState{}).
		Select(prefix).Where("channel_id > 0 AND profile_id = ? AND resource_id IN ?", assetBindingProfile, kinds).Group(prefix)
	if scope != nil {
		scopes = scopes.Where("channel_id = ? AND pre_request_id = ?", scope.ChannelID, scope.Scope)
	}
	selects, args := make([]string, len(digests)), make([]any, len(digests))
	for i, digest := range digests {
		selects[i], args[i] = "SELECT ? AS digest", digest
	}
	wanted := clause.Expr{SQL: strings.Join(selects, " UNION ALL "), Vars: args}
	// An IN list on state_key after the scope join only uses the first six
	// columns on MySQL 5.7. Equality to a derived digest row uses all seven,
	// including when discovering several aliases in one registration.
	table := db.NamingStrategy.TableName("ConfigurableResourceState")
	return query.Table("(?) AS asset_handles STRAIGHT_JOIN (?) AS asset_scopes STRAIGHT_JOIN ? AS asset_candidates FORCE INDEX (idx_configurable_resource_state_scope) ON "+
		"asset_candidates.channel_id = asset_scopes.channel_id AND asset_candidates.profile_id = asset_scopes.profile_id AND "+
		"asset_candidates.resource_id = asset_scopes.resource_id AND asset_candidates.pre_request_id = asset_scopes.pre_request_id AND "+
		"asset_candidates.user_id = asset_scopes.user_id AND asset_candidates.token_id = asset_scopes.token_id AND asset_candidates.state_key = asset_handles.digest",
		wanted, scopes, clause.Table{Name: table})
}

func assetHandleCandidateIDs(db *gorm.DB, kinds, digests []string) ([]int, error) {
	var ids []int
	err := assetHandleCandidateQuery(db, kinds, digests).Pluck("id", &ids).Error
	return ids, err
}

// Enumerating existing scope tuples also includes historical nonzero user/token
// columns, without losing full-key seeks on MySQL or scanning other accounts.
func scopedOwnedAssetStates(db *gorm.DB, binding AssetBinding, activeOnly bool) ([]ConfigurableResourceState, error) {
	query := scopedAssetHandleStates(db, binding, []string{assetBindingKey(binding.ID)})
	prefix := ""
	if db.Dialector.Name() == common.DatabaseTypeMySQL {
		prefix = "asset_candidates."
	}
	query = query.Where(prefix+"state_value = ?", strconv.Itoa(binding.UserID))
	if activeOnly {
		query = query.Where(prefix+"status = ?", ConfigurableResourceStateStatusActive)
	}
	var states []ConfigurableResourceState
	err := query.Find(&states).Error
	return states, err
}

// Look up the exact kind/handle across historical user/token tuples. Do not
// filter owner or status: a conflicting or revoked row must prevent insertion.
// New registrations still use zero tuples, while existing rows keep their keys.
func findStoredAssetBinding(db *gorm.DB, binding AssetBinding, handle string) (*ConfigurableResourceState, error) {
	query := assetHandleCandidateQueryScoped(db, []string{binding.Kind}, []string{assetBindingKey(handle)}, &binding)
	if db.Dialector.Name() == common.DatabaseTypeMySQL {
		query = query.Select("asset_candidates.*")
	}
	var states []ConfigurableResourceState
	if err := query.Limit(2).Find(&states).Error; err != nil {
		return nil, err
	}
	if len(states) > 1 {
		return nil, ErrAssetNotOwned // Do not silently choose an ambiguous owner.
	}
	if len(states) == 0 {
		return nil, nil
	}
	return &states[0], nil
}

// FindScopedAssetBindingsContext resolves response metadata after the caller
// has selected a channel/account; other accounts cannot supply group metadata.
func FindScopedAssetBindingsContext(ctx context.Context, channelID, userID int, scope, kind, handle string) ([]AssetBinding, error) {
	states, err := scopedOwnedAssetStates(DB.WithContext(ctx), AssetBinding{ChannelID: channelID, UserID: userID, Scope: scope, Kind: kind, ID: handle}, true)
	if err != nil {
		return nil, err
	}
	bindings, err := decodeAssetBindings(states)
	if err != nil {
		return nil, err
	}
	active, err := filterActiveAssetBindings(DB.WithContext(ctx), bindings)
	if err == nil && len(bindings) > 0 && len(active) == 0 {
		return nil, ErrAssetRevoked
	}
	return active, err
}

// Batch global conflict checks for the entire alias set after acquiring its
// handle mutexes. Claims must still check other channels and account scopes.
func assetRegistrationStates(db *gorm.DB, bindings []AssetBinding) (map[string][]ConfigurableResourceState, error) {
	kindSet, digestSet := make(map[string]bool), make(map[string]bool)
	for _, binding := range bindings {
		kindSet[binding.Kind] = true
		if assetBindingKind(binding.Kind) == "asset" {
			kindSet["asset"], kindSet["task"] = true, true
		}
		digestSet[assetBindingKey(binding.ID)] = true
	}
	kinds, digests := make([]string, 0, len(kindSet)), make([]string, 0, len(digestSet))
	for kind := range kindSet {
		kinds = append(kinds, kind)
	}
	for digest := range digestSet {
		digests = append(digests, digest)
	}
	states := make(map[string][]ConfigurableResourceState)
	for start := 0; start < len(digests); start += assetBindingBatchSize {
		ids, err := assetHandleCandidateIDs(db, kinds, digests[start:min(start+assetBindingBatchSize, len(digests))])
		if err != nil {
			return nil, err
		}
		for offset := 0; offset < len(ids); offset += assetBindingBatchSize {
			var batch []ConfigurableResourceState
			if err := db.Select("channel_id", "resource_id", "pre_request_id", "state_key", "state_value", "status").
				Where("id IN ?", ids[offset:min(offset+assetBindingBatchSize, len(ids))]).Find(&batch).Error; err != nil {
				return nil, err
			}
			for _, state := range batch {
				key := assetBindingLockKey(state.ResourceID, state.StateKey)
				states[key] = append(states[key], state)
			}
		}
	}
	return states, nil
}

// This path never starts a transaction, creates mutexes or acquires row locks.
// A concurrent deletion may commit after these reads, but a read-only poll
// cannot revive it. Any change is revalidated under the writer's locks.
func assetBindingsCurrent(db *gorm.DB, bindings []AssetBinding) (bool, error) {
	var existingBindings []AssetBinding
	families := make(map[string]string)
	for _, binding := range bindings {
		stored, err := findStoredAssetBinding(db, binding, binding.ID)
		if err != nil {
			return false, err
		}
		if stored == nil {
			return false, nil
		}
		if stored.StateValue != strconv.Itoa(binding.UserID) {
			return false, ErrAssetNotOwned
		}
		if stored.Status != ConfigurableResourceStateStatusActive {
			return false, ErrAssetRevoked
		}
		if binding.Kind == "session" {
			continue // A repeated registration must never extend a session's expiry.
		}
		var existing AssetBinding
		if err := common.UnmarshalJsonStr(stored.Metadata, &existing); err != nil {
			return false, err
		}
		if (existing.Project != "" && binding.Project != "" && existing.Project != binding.Project) ||
			(existing.GroupID != "" && binding.GroupID != "" && existing.GroupID != binding.GroupID) {
			return false, ErrAssetNotOwned
		}
		if (existing.Project == "" && binding.Project != "") || (existing.GroupID == "" && binding.GroupID != "") {
			return false, nil
		}
		family := assetLifecycleKey(binding, binding.Kind, assetCanonicalID(binding))
		canonical := assetCanonicalID(existing)
		if previous, ok := families[family]; ok && previous != canonical {
			return false, ErrAssetNotOwned
		}
		families[family] = canonical
		existingBindings = append(existingBindings, existing)
	}
	active, err := filterActiveAssetBindings(db, existingBindings)
	if err != nil {
		return false, err
	}
	if len(active) != len(existingBindings) {
		return false, ErrAssetRevoked
	}
	return true, nil
}
