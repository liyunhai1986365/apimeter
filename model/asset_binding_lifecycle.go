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

type assetBindingLockMode uint8

const (
	assetBindingSharedLock assetBindingLockMode = iota
	assetBindingExclusiveLock
)

type assetBindingLockPlan map[string]assetBindingLockMode

type assetBindingRowLock struct {
	id   int
	mode assetBindingLockMode
}

// Include every dependency before deciding to retry, so large alias sets need
// one plan expansion rather than one transaction attempt per newly found alias.
func (plan assetBindingLockPlan) include(binding AssetBinding, locked map[string]ConfigurableResourceState) bool {
	keys := assetOwnLifecycleKeys(binding)
	if binding.ID != "" {
		keys = append(keys, assetBindingLockKey(binding.Kind, assetBindingKey(binding.ID)))
	}
	missing := plan.includeKeys(keys, locked)
	if assetBindingKind(binding.Kind) == "asset" && binding.GroupID != "" {
		key := assetLifecycleKey(binding, "group", binding.GroupID)
		// Different children may register concurrently. Only deletion or a
		// write to the group itself needs to exclude other group readers.
		if _, exists := plan[key]; !exists {
			plan[key] = assetBindingSharedLock
		}
		if _, exists := locked[key]; !exists {
			missing = true
		}
	}
	return missing
}

func (plan assetBindingLockPlan) includeKeys(keys []string, locked map[string]ConfigurableResourceState) bool {
	missing := false
	for _, key := range keys {
		if mode, exists := plan[key]; exists && mode == assetBindingSharedLock {
			// Discovering a stronger dependency must restart the transaction,
			// never upgrade a shared lock while other writers hold it too.
			missing = true
		}
		plan[key] = assetBindingExclusiveLock
		if _, ok := locked[key]; !ok {
			missing = true
		}
	}
	return missing
}

// Resolve each incoming alias family against its stored canonical ID, including
// newly discovered handles. Expand missing lock dependencies before any writes;
// never acquire additional locks out of order within the current transaction.
func (plan assetBindingLockPlan) resolveStored(tx *gorm.DB, bindings []AssetBinding, locked map[string]ConfigurableResourceState, refs map[string]AssetBinding) ([]AssetBinding, error) {
	type ownerHandle struct {
		channel                    int
		scope, kind, digest, owner string
	}
	wanted := make(map[ownerHandle][]string, len(bindings))
	scopes := make(map[ownerHandle]AssetBinding)
	digests := make(map[string]bool, len(bindings))
	for _, binding := range bindings {
		digest := assetBindingKey(binding.ID)
		digests[digest] = true
		handle := ownerHandle{binding.ChannelID, binding.Scope, assetBindingKind(binding.Kind), digest, strconv.Itoa(binding.UserID)}
		wanted[handle] = append(wanted[handle], assetLifecycleKey(binding, binding.Kind, assetCanonicalID(binding)))
		scopes[ownerHandle{channel: binding.ChannelID, scope: binding.Scope, kind: assetBindingKind(binding.Kind)}] = binding
	}
	keys := make([]string, 0, len(digests))
	for key := range digests {
		keys = append(keys, key)
	}
	missing := false
	canonicalIDs := make(map[string]string)
	for start := 0; start < len(keys); start += assetBindingBatchSize {
		var states []ConfigurableResourceState
		for _, binding := range scopes {
			var scoped []ConfigurableResourceState
			if err := scopedAssetHandleStates(tx, binding, keys[start:min(start+assetBindingBatchSize, len(keys))]).Find(&scoped).Error; err != nil {
				return nil, err
			}
			states = append(states, scoped...)
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
			assetIdentityRefs(refs, binding)
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

type assetBindingLifecycle struct {
	tx      *gorm.DB
	locked  map[string]ConfigurableResourceState
	history assetBindingHistory
	ready   map[string]bool
	refs    map[string]AssetBinding
}

func newAssetBindingLifecycle(tx *gorm.DB, locked map[string]ConfigurableResourceState, history assetBindingHistory, refs map[string]AssetBinding) *assetBindingLifecycle {
	return &assetBindingLifecycle{tx: tx, locked: locked, history: history, refs: refs, ready: make(map[string]bool)}
}

func (l *assetBindingLifecycle) validateIdentity(key string, visiting map[string]bool) error {
	state, ok := l.locked[key]
	if !ok {
		return errAssetBindingPlanChanged
	}
	if state.Status != ConfigurableResourceStateStatusActive {
		return ErrAssetRevoked
	}
	node, checked := l.history[key]
	if !checked {
		return errAssetBindingPlanChanged
	}
	// A concurrent writer may have added a parent after the unlocked history
	// read. Rediscover and acquire every dependency in PK order on the next try.
	if state.StateValue != node.state.StateValue || state.Metadata != node.state.Metadata {
		return errAssetBindingPlanChanged
	}
	if node.revoked {
		return ErrAssetRevoked
	}
	if visiting[key] {
		return fmt.Errorf("cyclic asset group identities")
	}
	visiting[key] = true
	defer delete(visiting, key)
	for parent := range node.parents {
		if err := l.validateIdentity(assetLifecycleKey(node.ref, "group", parent), visiting); err != nil {
			return err
		}
	}
	l.ready[key] = true
	return nil
}

func (l *assetBindingLifecycle) validate(binding AssetBinding) error {
	assetIdentityRefs(l.refs, binding)
	own := assetLifecycleKey(binding, binding.Kind, assetCanonicalID(binding))
	if node := l.history[own]; node != nil && assetBindingKind(binding.Kind) == "asset" && binding.GroupID != "" {
		node.parents[binding.GroupID] = true
	}
	if binding.Kind == "group" && binding.ID != "" && binding.ID != assetCanonicalID(binding) {
		alias := assetLifecycleKey(binding, "group", binding.ID)
		if node := l.history[alias]; node != nil {
			node.parents[assetCanonicalID(binding)] = true
		}
	}
	for _, key := range assetLifecycleKeys(binding) {
		if err := l.validateIdentity(key, make(map[string]bool)); err != nil {
			return err
		}
	}
	return nil
}

func (l *assetBindingLifecycle) markInitialized() error {
	// All identities are already locked in PK order. Batch equal parent sets so
	// creating many children in one group does not add one UPDATE per child.
	batches := make(map[string][]int)
	for key := range l.ready {
		state, node := l.locked[key], l.history[key]
		metadata, err := assetIdentityParentsJSON(node.parents)
		if err != nil {
			return err
		}
		if state.StateValue == assetLifecycleRelated && state.Metadata == metadata {
			continue
		}
		batches[metadata] = append(batches[metadata], state.Id)
	}
	for metadata, ids := range batches {
		sort.Ints(ids)
		for start := 0; start < len(ids); start += assetBindingBatchSize {
			if err := assetBindingPrimaryRows(l.tx, ids[start:min(start+assetBindingBatchSize, len(ids))]).
				Updates(map[string]any{"state_value": assetLifecycleRelated, "metadata": metadata}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
