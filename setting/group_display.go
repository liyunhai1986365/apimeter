package setting

import (
	"sort"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type GroupDisplayCategory struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Order int    `json:"order"`
}

type GroupDisplayGroup struct {
	Group        string `json:"group"`
	CategoryID   string `json:"category_id"`
	Order        int    `json:"order"`
	UserGroup    bool   `json:"user_group,omitempty"`
	HideDiscount bool   `json:"hide_discount,omitempty"`
}

type GroupDisplayConfig struct {
	Categories []GroupDisplayCategory `json:"categories"`
	Groups     []GroupDisplayGroup    `json:"groups"`
}

var groupDisplayConfig = GroupDisplayConfig{
	Categories: []GroupDisplayCategory{},
	Groups:     []GroupDisplayGroup{},
}
var groupDisplayConfigMutex sync.RWMutex

func NormalizeGroupDisplayConfig(config GroupDisplayConfig) GroupDisplayConfig {
	categoryIDs := make(map[string]struct{}, len(config.Categories))
	categories := make([]GroupDisplayCategory, 0, len(config.Categories))
	for _, category := range config.Categories {
		id := strings.TrimSpace(category.ID)
		if id == "" {
			continue
		}
		if _, ok := categoryIDs[id]; ok {
			continue
		}
		name := strings.TrimSpace(category.Name)
		if name == "" {
			name = id
		}
		categoryIDs[id] = struct{}{}
		categories = append(categories, GroupDisplayCategory{
			ID:    id,
			Name:  name,
			Order: category.Order,
		})
	}

	sort.SliceStable(categories, func(i, j int) bool {
		if categories[i].Order != categories[j].Order {
			return categories[i].Order < categories[j].Order
		}
		if categories[i].Name != categories[j].Name {
			return categories[i].Name < categories[j].Name
		}
		return categories[i].ID < categories[j].ID
	})

	groupNames := make(map[string]struct{}, len(config.Groups))
	groups := make([]GroupDisplayGroup, 0, len(config.Groups))
	for _, group := range config.Groups {
		name := strings.TrimSpace(group.Group)
		if name == "" {
			continue
		}
		if _, ok := groupNames[name]; ok {
			continue
		}
		categoryID := strings.TrimSpace(group.CategoryID)
		if _, ok := categoryIDs[categoryID]; !ok {
			categoryID = ""
		}
		groupNames[name] = struct{}{}
		groups = append(groups, GroupDisplayGroup{
			Group:        name,
			CategoryID:   categoryID,
			Order:        group.Order,
			UserGroup:    group.UserGroup,
			HideDiscount: group.HideDiscount,
		})
	}

	categoryOrder := make(map[string]int, len(categories))
	for index, category := range categories {
		categoryOrder[category.ID] = index
	}
	sort.SliceStable(groups, func(i, j int) bool {
		leftCategoryOrder, leftHasCategory := categoryOrder[groups[i].CategoryID]
		rightCategoryOrder, rightHasCategory := categoryOrder[groups[j].CategoryID]
		if leftHasCategory != rightHasCategory {
			return leftHasCategory
		}
		if leftHasCategory && leftCategoryOrder != rightCategoryOrder {
			return leftCategoryOrder < rightCategoryOrder
		}
		if groups[i].Order != groups[j].Order {
			return groups[i].Order < groups[j].Order
		}
		return groups[i].Group < groups[j].Group
	})

	return GroupDisplayConfig{
		Categories: categories,
		Groups:     groups,
	}
}

func IsGroupDiscountHidden(groupName string) bool {
	groupDisplayConfigMutex.RLock()
	defer groupDisplayConfigMutex.RUnlock()

	for _, group := range groupDisplayConfig.Groups {
		if group.Group == groupName {
			return group.HideDiscount
		}
	}
	return false
}

func GetUserGroupNamesFromDisplayConfig() ([]string, bool) {
	groupDisplayConfigMutex.RLock()
	defer groupDisplayConfigMutex.RUnlock()

	groups := make([]string, 0, len(groupDisplayConfig.Groups))
	for _, group := range groupDisplayConfig.Groups {
		if group.UserGroup {
			groups = append(groups, group.Group)
		}
	}
	return groups, len(groupDisplayConfig.Groups) > 0
}

func GetTokenGroupNamesFromDisplayConfig() ([]string, bool) {
	groupDisplayConfigMutex.RLock()
	defer groupDisplayConfigMutex.RUnlock()

	groups := make([]string, 0, len(groupDisplayConfig.Groups))
	for _, group := range groupDisplayConfig.Groups {
		if !group.UserGroup {
			groups = append(groups, group.Group)
		}
	}
	return groups, len(groupDisplayConfig.Groups) > 0
}

func GetGroupDisplayConfig() GroupDisplayConfig {
	groupDisplayConfigMutex.RLock()
	defer groupDisplayConfigMutex.RUnlock()

	categories := make([]GroupDisplayCategory, len(groupDisplayConfig.Categories))
	copy(categories, groupDisplayConfig.Categories)
	groups := make([]GroupDisplayGroup, len(groupDisplayConfig.Groups))
	copy(groups, groupDisplayConfig.Groups)

	return GroupDisplayConfig{
		Categories: categories,
		Groups:     groups,
	}
}

func GroupDisplayConfig2JSONString() string {
	groupDisplayConfigMutex.RLock()
	defer groupDisplayConfigMutex.RUnlock()

	jsonBytes, err := common.Marshal(groupDisplayConfig)
	if err != nil {
		common.SysLog("error marshalling group display config: " + err.Error())
		return `{"categories":[],"groups":[]}`
	}
	return string(jsonBytes)
}

func UpdateGroupDisplayConfigByJSONString(jsonStr string) error {
	config := GroupDisplayConfig{}
	if err := common.Unmarshal([]byte(jsonStr), &config); err != nil {
		return err
	}

	groupDisplayConfigMutex.Lock()
	defer groupDisplayConfigMutex.Unlock()
	groupDisplayConfig = NormalizeGroupDisplayConfig(config)
	return nil
}
