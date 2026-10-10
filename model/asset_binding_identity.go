package model

import (
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// v2 records the parents of the whole canonical family, including aliases whose
// own metadata has not acquired a GroupID. Only natural writes initialize it;
// historical reads never backfill or rewrite ownership records.
const assetLifecycleRelated = "lifecycle-v2"

type assetIdentityMetadata struct {
	Parents []string `json:"parents"`
}

type assetIdentityNode struct {
	ref     AssetBinding
	state   ConfigurableResourceState
	parents map[string]bool
	revoked bool
}

type assetBindingHistory map[string]*assetIdentityNode

func assetIdentityRef(binding AssetBinding, kind, id string) AssetBinding {
	return AssetBinding{ChannelID: binding.ChannelID, Scope: binding.Scope, Kind: assetBindingKind(kind), ID: id}
}

func assetIdentityRefs(refs map[string]AssetBinding, binding AssetBinding) {
	if binding.Kind == "session" {
		return
	}
	add := func(kind, id string) {
		ref := assetIdentityRef(binding, kind, id)
		refs[assetLifecycleKey(ref, ref.Kind, ref.ID)] = ref
	}
	add(binding.Kind, assetCanonicalID(binding))
	if binding.Kind == "group" && binding.ID != "" {
		add("group", binding.ID)
	}
	if assetBindingKind(binding.Kind) == "asset" && binding.GroupID != "" {
		add("group", binding.GroupID)
	}
}

func assetIdentityStates(db *gorm.DB, keys []string) *gorm.DB {
	return db.Model(&ConfigurableResourceState{}).Where(
		"channel_id = 0 AND profile_id = ? AND resource_id = ? AND pre_request_id = ? AND user_id = 0 AND token_id = 0 AND state_key IN ?",
		assetBindingLockProfile, "handle", "global", keys)
}

// No JSON SQL extensions are required. Escaped JSON is deliberately included;
// Go performs the exact identity check afterward. This bounds returned metadata,
// not rows examined inside the account range (there is no canonical-ID index).
func assetMetadataCandidates(db *gorm.DB, ids []string) *gorm.DB {
	predicates := make([]string, 0, len(ids)+1)
	args := make([]any, 0, len(ids)+1)
	for _, id := range ids {
		literal := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(id)
		predicates = append(predicates, "metadata LIKE ? ESCAPE '!'")
		args = append(args, "%"+literal+"%")
	}
	predicates = append(predicates, "metadata LIKE ? ESCAPE '!'")
	args = append(args, "%\\%")
	return db.Where("("+strings.Join(predicates, " OR ")+")", args...)
}

// Load only the requested identities and their parents. Legacy family discovery
// is outside write transactions, and is never cached across requests. Missing
// keys remain distinguishable from checked, active identities during validation.
func loadAssetBindingHistory(db *gorm.DB, refs map[string]AssetBinding) (assetBindingHistory, error) {
	history := make(assetBindingHistory)
	pending := make(map[string]AssetBinding, len(refs))
	for key, ref := range refs {
		pending[key] = ref
	}
	for len(pending) > 0 {
		keys := make([]string, 0, len(pending))
		for key, ref := range pending {
			history[key] = &assetIdentityNode{ref: ref, parents: make(map[string]bool)}
			keys = append(keys, key)
		}
		for start := 0; start < len(keys); start += assetBindingBatchSize {
			var states []ConfigurableResourceState
			if err := assetIdentityStates(db, keys[start:min(start+assetBindingBatchSize, len(keys))]).
				Select("id", "state_key", "status", "state_value", "metadata").Find(&states).Error; err != nil {
				return nil, err
			}
			for _, state := range states {
				node := history[state.StateKey]
				node.state = state
				node.revoked = state.Status != ConfigurableResourceStateStatusActive
				if !node.revoked && state.StateValue == assetLifecycleRelated {
					var metadata assetIdentityMetadata
					if err := common.UnmarshalJsonStr(state.Metadata, &metadata); err != nil {
						return nil, fmt.Errorf("invalid asset identity metadata: %w", err)
					}
					for _, parent := range metadata.Parents {
						node.parents[parent] = true
					}
				}
			}
		}
		type scopeKind struct {
			channel     int
			scope, kind string
		}
		legacy := make(map[scopeKind][]*assetIdentityNode)
		for key := range pending {
			node := history[key]
			if !node.revoked && node.state.StateValue != assetLifecycleRelated {
				scope := scopeKind{node.ref.ChannelID, node.ref.Scope, node.ref.Kind}
				legacy[scope] = append(legacy[scope], node)
			}
		}
		for scope, nodes := range legacy {
			kinds := []string{scope.kind}
			if scope.kind == "asset" {
				kinds = []string{"asset", "task"}
			}
			for start := 0; start < len(nodes); start += assetBindingBatchSize {
				batch := nodes[start:min(start+assetBindingBatchSize, len(nodes))]
				ids := make([]string, len(batch))
				wanted := make(map[string]*assetIdentityNode, len(batch))
				for i, node := range batch {
					ids[i] = node.ref.ID
					wanted[assetLifecycleKey(node.ref, node.ref.Kind, node.ref.ID)] = node
				}
				query := db.Model(&ConfigurableResourceState{}).Select("id", "metadata", "status").Where("channel_id = ? AND profile_id = ? AND resource_id IN ? AND pre_request_id = ?",
					scope.channel, assetBindingProfile, kinds, scope.scope)
				err := walkAssetStateRows(assetMetadataCandidates(query, ids), func(states []ConfigurableResourceState) error {
					for _, state := range states {
						var binding AssetBinding
						if err := common.UnmarshalJsonStr(state.Metadata, &binding); err != nil {
							return err
						}
						for _, key := range assetOwnLifecycleKeys(binding) {
							node := wanted[key]
							if node == nil {
								continue
							}
							node.revoked = node.revoked || state.Status != ConfigurableResourceStateStatusActive
							if assetBindingKind(binding.Kind) == "asset" && binding.GroupID != "" {
								node.parents[binding.GroupID] = true
							}
							if binding.Kind == "group" && assetCanonicalID(binding) != node.ref.ID {
								node.parents[assetCanonicalID(binding)] = true
							}
						}
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
			}
		}
		next := make(map[string]AssetBinding)
		for key := range pending {
			node := history[key]
			for parent := range node.parents {
				ref := assetIdentityRef(node.ref, "group", parent)
				parentKey := assetLifecycleKey(ref, ref.Kind, ref.ID)
				if _, ok := history[parentKey]; !ok {
					next[parentKey] = ref
				}
			}
		}
		pending = next
	}
	return history, nil
}

func (h assetBindingHistory) revoked(key string, visiting map[string]bool) (bool, error) {
	node, ok := h[key]
	if !ok {
		return false, errAssetBindingPlanChanged
	}
	if node.revoked {
		return true, nil
	}
	if visiting[key] {
		return false, fmt.Errorf("cyclic asset group identities")
	}
	visiting[key] = true
	defer delete(visiting, key)
	for parent := range node.parents {
		if revoked, err := h.revoked(assetLifecycleKey(node.ref, "group", parent), visiting); err != nil || revoked {
			return revoked, err
		}
	}
	return false, nil
}

func filterActiveAssetBindings(db *gorm.DB, bindings []AssetBinding) ([]AssetBinding, error) {
	refs := make(map[string]AssetBinding)
	for _, binding := range bindings {
		assetIdentityRefs(refs, binding)
	}
	history, err := loadAssetBindingHistory(db, refs)
	if err != nil {
		return nil, err
	}
	active := make([]AssetBinding, 0, len(bindings))
	for _, binding := range bindings {
		revoked := false
		for _, key := range assetLifecycleKeys(binding) {
			invalid, err := history.revoked(key, make(map[string]bool))
			if err != nil {
				return nil, err
			}
			revoked = revoked || invalid
		}
		if !revoked {
			active = append(active, binding)
		}
	}
	return active, nil
}

func assetIdentityParentsJSON(parents map[string]bool) (string, error) {
	ids := make([]string, 0, len(parents))
	for id := range parents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	data, err := common.Marshal(assetIdentityMetadata{Parents: ids})
	return string(data), err
}
