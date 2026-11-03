package controller

import (
	"fmt"

	"github.com/tidwall/gjson"
)

// Immutable, canonical filters also form part of the shared scan identity.
// YH accepts only name and a single group upstream; apply the other conditions
// during ownership filtering, leaving raw page lengths intact for pagination.
type yhAssetListFilter struct {
	GroupID  string
	GroupIDs map[string]bool
	AssetIDs map[string]bool
	IDs      map[string]bool
	Statuses map[string]bool
}

func newYHAssetListFilter(a *assetAccessRequest, filter gjson.Result, groupID string) (*yhAssetListFilter, error) {
	aliases := map[string]map[string]string{"group": {}, "asset": {}}
	for _, binding := range a.bindings {
		if ids := aliases[binding.Kind]; ids != nil && binding.CanonicalID != "" {
			ids[binding.ID] = binding.CanonicalID
		}
	}
	canonical := func(kind, id string) string {
		if value := aliases[kind][id]; value != "" {
			return value
		}
		return id
	}
	f := &yhAssetListFilter{GroupID: canonical("group", groupID)}
	for _, field := range []struct {
		name, kind string
		target     *map[string]bool
	}{
		{"GroupIds", "group", &f.GroupIDs},
		{"AssetIds", "asset", &f.AssetIDs},
		{"Ids", "asset", &f.IDs},
		{"Statuses", "", &f.Statuses},
	} {
		values := filter.Get(field.name)
		if !values.Exists() {
			continue
		}
		if !values.IsArray() {
			return nil, fmt.Errorf("Filter.%s must be an array", field.name)
		}
		*field.target = make(map[string]bool, len(values.Array()))
		for _, value := range values.Array() {
			if value.Type != gjson.String || value.String() == "" {
				return nil, fmt.Errorf("Filter.%s must contain nonempty strings", field.name)
			}
			(*field.target)[canonical(field.kind, value.String())] = true
		}
	}
	return f, nil
}

func (f *yhAssetListFilter) upstreamGroupID() string {
	if f.GroupID != "" {
		return f.GroupID
	}
	if len(f.GroupIDs) == 1 {
		for id := range f.GroupIDs {
			return id
		}
	}
	return ""
}

func (f *yhAssetListFilter) filtersGroups() bool {
	return f.GroupID != "" || len(f.GroupIDs) > 0
}

func (f *yhAssetListFilter) matchesID(canonical string) bool {
	return (len(f.AssetIDs) == 0 || f.AssetIDs[canonical]) && (len(f.IDs) == 0 || f.IDs[canonical])
}

func (f *yhAssetListFilter) matches(row assetListRow, canonical, ownedGroup string) bool {
	if !f.matchesID(canonical) {
		return false
	}
	if len(f.Statuses) > 0 && !f.Statuses[gjson.Get(row.raw, "Status").String()] {
		return false
	}
	if f.filtersGroups() {
		groupID := gjson.Get(row.raw, "GroupId").String()
		if groupID == "" {
			// YH list rows may omit group_id. Reuse metadata loaded with the
			// ownership snapshot, without a database lookup for every row.
			groupID = ownedGroup
		}
		if groupID == "" {
			// Single-group requests are already restricted by the supplier.
			groupID = f.upstreamGroupID()
		}
		if (f.GroupID != "" && groupID != f.GroupID) || (len(f.GroupIDs) > 0 && !f.GroupIDs[groupID]) {
			return false
		}
	}
	return true
}
