package model

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const assetLifecycleInitialized = "lifecycle-v1"

var errAssetBindingPlanChanged = errors.New("asset binding lock dependencies changed")

func assetBindingKind(kind string) string {
	if kind == "task" {
		return "asset"
	}
	return kind
}

// Lifecycle records share the existing mutex table/namespace but are scoped to
// a channel and account. Handles remain global for cross-channel claim checks.
// Digesting both strings makes the tuple unambiguous without storing credentials.
func assetLifecycleKey(binding AssetBinding, kind, id string) string {
	identity := fmt.Sprintf("%d:%s:%s:%s", binding.ChannelID, assetBindingKey(binding.Scope), assetBindingKind(kind), assetBindingKey(id))
	return "identity:" + assetBindingKey(identity)
}

func assetCanonicalID(binding AssetBinding) string {
	if binding.CanonicalID != "" {
		return binding.CanonicalID
	}
	return binding.ID
}

// Own identity keys exclude the parent: deleting a child never revokes its group.
// Group aliases also coordinate children that still refer to an original ID.
func assetOwnLifecycleKeys(binding AssetBinding) []string {
	if binding.Kind == "session" {
		return nil
	}
	canonical := assetCanonicalID(binding)
	keys := []string{assetLifecycleKey(binding, binding.Kind, canonical)}
	if binding.Kind == "group" && binding.ID != "" && binding.ID != canonical {
		keys = append(keys, assetLifecycleKey(binding, "group", binding.ID))
	}
	return keys
}

func assetLifecycleKeys(binding AssetBinding) []string {
	keys := assetOwnLifecycleKeys(binding)
	if assetBindingKind(binding.Kind) == "asset" && binding.GroupID != "" {
		keys = append(keys, assetLifecycleKey(binding, "group", binding.GroupID))
	}
	return keys
}

type assetBindingLockPlan map[string]bool

// Include every dependency before deciding to retry, so large alias sets need
// one plan expansion rather than one transaction attempt per newly found alias.
func (plan assetBindingLockPlan) include(binding AssetBinding, locked map[string]ConfigurableResourceState) bool {
	keys := assetLifecycleKeys(binding)
	if binding.ID != "" {
		keys = append(keys, assetBindingLockKey(binding.Kind, assetBindingKey(binding.ID)))
	}
	return plan.includeKeys(keys, locked)
}

func (plan assetBindingLockPlan) includeKeys(keys []string, locked map[string]ConfigurableResourceState) bool {
	missing := false
	for _, key := range keys {
		plan[key] = true
		if _, ok := locked[key]; !ok {
			missing = true
		}
	}
	return missing
}

// Resolve each incoming alias family against its stored canonical ID, including
// newly discovered handles. Expand missing lock dependencies before any writes;
// never acquire additional locks out of order within the current transaction.
func (plan assetBindingLockPlan) resolveStored(tx *gorm.DB, bindings []AssetBinding, locked map[string]ConfigurableResourceState) ([]AssetBinding, error) {
	type ownerHandle struct {
		channel                    int
		scope, kind, digest, owner string
	}
	wanted := make(map[ownerHandle][]string, len(bindings))
	digests := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		digest := assetBindingKey(binding.ID)
		digests[digest] = true
		handle := ownerHandle{binding.ChannelID, binding.Scope, assetBindingKind(binding.Kind), digest, strconv.Itoa(binding.UserID)}
		wanted[handle] = append(wanted[handle], assetLifecycleKey(binding, binding.Kind, assetCanonicalID(binding)))
	}
	keys := make([]string, 0, len(digests))
	for key := range digests {
		keys = append(keys, key)
	}
	missing := false
	canonicalIDs := make(map[string]string)
	for start := 0; start < len(keys); start += assetBindingBatchSize {
		var states []ConfigurableResourceState
		if err := tx.Where("profile_id = ? AND state_key IN ?", assetBindingProfile, keys[start:min(start+assetBindingBatchSize, len(keys))]).Find(&states).Error; err != nil {
			return nil, err
		}
		for _, state := range states {
			matches := wanted[ownerHandle{state.ChannelID, state.PreRequestID, assetBindingKind(state.ResourceID), state.StateKey, state.StateValue}]
			if len(matches) == 0 || state.ResourceID == "session" {
				continue
			}
			var binding AssetBinding
			if err := common.UnmarshalJsonStr(state.Metadata, &binding); err != nil {
				return nil, err
			}
			if plan.include(binding, locked) {
				missing = true
			}
			for _, family := range matches {
				canonical := assetCanonicalID(binding)
				if previous, ok := canonicalIDs[family]; ok && previous != canonical {
					return nil, ErrAssetNotOwned // Do not silently merge established identities.
				}
				canonicalIDs[family] = canonical
			}
		}
	}
	if missing {
		return nil, errAssetBindingPlanChanged
	}
	resolved := append([]AssetBinding(nil), bindings...)
	for i, binding := range resolved {
		family := assetLifecycleKey(binding, binding.Kind, assetCanonicalID(binding))
		if canonical, ok := canonicalIDs[family]; ok {
			resolved[i].CanonicalID = canonical
		}
	}
	return resolved, nil
}

type assetBindingScope struct {
	channel int
	scope   string
}

type assetBindingLifecycle struct {
	tx      *gorm.DB
	locked  map[string]ConfigurableResourceState
	history map[assetBindingScope]map[string]bool
	ready   map[int]bool
}

func newAssetBindingLifecycle(tx *gorm.DB, locked map[string]ConfigurableResourceState) *assetBindingLifecycle {
	return &assetBindingLifecycle{tx: tx, locked: locked, history: make(map[assetBindingScope]map[string]bool), ready: make(map[int]bool)}
}

func (l *assetBindingLifecycle) validate(binding AssetBinding) error {
	for _, key := range assetLifecycleKeys(binding) {
		state, ok := l.locked[key]
		if !ok {
			return errAssetBindingPlanChanged
		}
		if state.Status != ConfigurableResourceStateStatusActive {
			return ErrAssetNotOwned
		}
		if state.StateValue == assetLifecycleInitialized {
			continue
		}
		// Old revoked aliases are the authoritative tombstones until their
		// identity records have been initialized. A canonical ID need not have
		// its own handle row. Read history once per scope, only on first use.
		scope := assetBindingScope{binding.ChannelID, binding.Scope}
		revoked, ok := l.history[scope]
		if !ok {
			var states []ConfigurableResourceState
			if err := l.tx.Where("channel_id = ? AND profile_id = ? AND pre_request_id = ? AND status = ? AND resource_id IN ?",
				binding.ChannelID, assetBindingProfile, binding.Scope, ConfigurableResourceStateStatusInvalid, []string{"asset", "task", "group"}).Find(&states).Error; err != nil {
				return err
			}
			revoked = make(map[string]bool)
			for _, old := range states {
				var item AssetBinding
				if err := common.UnmarshalJsonStr(old.Metadata, &item); err != nil {
					return err
				}
				for _, identity := range assetOwnLifecycleKeys(item) {
					revoked[identity] = true
				}
			}
			l.history[scope] = revoked
		}
		if revoked[key] {
			return ErrAssetNotOwned
		}
		l.ready[state.Id] = true
	}
	return nil
}

func (l *assetBindingLifecycle) markInitialized() error {
	ids := make([]int, 0, len(l.ready))
	for id := range l.ready {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for start := 0; start < len(ids); start += assetBindingBatchSize {
		if err := assetBindingPrimaryRows(l.tx, ids[start:min(start+assetBindingBatchSize, len(ids))]).UpdateColumn("state_value", assetLifecycleInitialized).Error; err != nil {
			return err
		}
	}
	return nil
}

type assetBindingRecord struct {
	state   ConfigurableResourceState
	binding AssetBinding
}

// Include inactive rows too: they preserve group aliases and identify families
// left partially revoked by older code. All aliases of a selected child are
// included even when only one alias has acquired GroupID metadata so far.
func assetRevocationRecords(db *gorm.DB, target AssetBinding, deleteGroup bool) (AssetBinding, []assetBindingRecord, error) {
	var states []ConfigurableResourceState
	if err := db.Where("channel_id = ? AND profile_id = ? AND pre_request_id = ? AND state_value = ?",
		target.ChannelID, assetBindingProfile, target.Scope, strconv.Itoa(target.UserID)).Find(&states).Error; err != nil {
		return target, nil, err
	}
	records := make([]assetBindingRecord, 0, len(states))
	for _, state := range states {
		var item AssetBinding
		if err := common.UnmarshalJsonStr(state.Metadata, &item); err != nil {
			return target, nil, err
		}
		records = append(records, assetBindingRecord{state, item})
		if target.ID != "" && state.StateKey == assetBindingKey(target.ID) && item.Kind == target.Kind {
			target.CanonicalID = assetCanonicalID(item)
		}
	}
	canonical := assetCanonicalID(target)
	groupIDs := map[string]bool{canonical: true}
	if deleteGroup {
		for _, record := range records {
			item := record.binding
			if item.Kind == "group" && assetCanonicalID(item) == canonical {
				groupIDs[item.ID] = true
			}
		}
	}
	children := make(map[string]bool)
	if deleteGroup {
		for _, record := range records {
			item := record.binding
			if assetBindingKind(item.Kind) == "asset" && item.GroupID != "" && groupIDs[item.GroupID] {
				children[assetCanonicalID(item)] = true
			}
		}
	}
	selected := make([]assetBindingRecord, 0)
	for _, record := range records {
		item := record.binding
		if (assetBindingKind(item.Kind) == assetBindingKind(target.Kind) && assetCanonicalID(item) == canonical) ||
			(deleteGroup && assetBindingKind(item.Kind) == "asset" && children[assetCanonicalID(item)]) {
			selected = append(selected, record)
		}
	}
	return target, selected, nil
}
