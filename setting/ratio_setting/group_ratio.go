package ratio_setting

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/types"
)

var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

var groupRatioMap = types.NewRWMap[string, float64]()

var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9,
	},
}

var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

var groupModelRatioMap = types.NewRWMap[string, map[string]float64]()

var userGroupModelRatioMap = types.NewRWMap[string, map[string]map[string]float64]()

var defaultGroupSpecialUsableGroup = map[string]map[string]string{}

type GroupRatioSetting struct {
	GroupRatio              *types.RWMap[string, float64]                       `json:"group_ratio"`
	GroupGroupRatio         *types.RWMap[string, map[string]float64]            `json:"group_group_ratio"`
	GroupModelRatio         *types.RWMap[string, map[string]float64]            `json:"group_model_ratio"`
	UserGroupModelRatio     *types.RWMap[string, map[string]map[string]float64] `json:"user_group_model_ratio"`
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string]             `json:"group_special_usable_group"`
}

var groupRatioSetting GroupRatioSetting

func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
		GroupModelRatio:         groupModelRatioMap,
		UserGroupModelRatio:     userGroupModelRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	if groupRatioSetting.GroupModelRatio == nil {
		groupRatioSetting.GroupModelRatio = groupModelRatioMap
	}
	if groupRatioSetting.UserGroupModelRatio == nil {
		groupRatioSetting.UserGroupModelRatio = userGroupModelRatioMap
	}
	return &groupRatioSetting
}

func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

func GetGroupModelRatio(usingGroup, modelName string) (float64, bool) {
	modelRatios, ok := groupModelRatioMap.Get(usingGroup)
	if !ok {
		return 0, false
	}
	return getModelSpecificRatio(modelRatios, modelName)
}

func GroupModelRatio2JSONString() string {
	return groupModelRatioMap.MarshalJSONString()
}

func GetGroupModelRatioCopy() map[string]map[string]float64 {
	return groupModelRatioMap.ReadAll()
}

func UpdateGroupModelRatioByJSONString(jsonStr string) error {
	if err := CheckGroupModelRatio(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonString(groupModelRatioMap, jsonStr)
}

func GetUserGroupModelRatio(userGroup, usingGroup, modelName string) (float64, bool) {
	groupRatios, ok := userGroupModelRatioMap.Get(userGroup)
	if !ok {
		return 0, false
	}
	modelRatios, ok := groupRatios[usingGroup]
	if !ok {
		return 0, false
	}
	return getModelSpecificRatio(modelRatios, modelName)
}

func UserGroupModelRatio2JSONString() string {
	return userGroupModelRatioMap.MarshalJSONString()
}

func GetUserGroupModelRatioCopy() map[string]map[string]map[string]float64 {
	return userGroupModelRatioMap.ReadAll()
}

func UpdateUserGroupModelRatioByJSONString(jsonStr string) error {
	if err := CheckUserGroupModelRatio(jsonStr); err != nil {
		return err
	}
	return types.LoadFromJsonString(userGroupModelRatioMap, jsonStr)
}

type EffectiveGroupRatio struct {
	Ratio          float64
	Source         types.GroupRatioSource
	IsUserSpecific bool
}

// ResolveEffectiveGroupRatio resolves the most specific configured ratio.
// Missing keys inherit in this order:
// user group + group + model -> group + model -> user group + group -> group -> 1.
func ResolveEffectiveGroupRatio(userGroup, usingGroup, modelName string) EffectiveGroupRatio {
	if ratio, ok := GetUserGroupModelRatio(userGroup, usingGroup, modelName); ok {
		return EffectiveGroupRatio{Ratio: ratio, Source: types.GroupRatioSourceUserGroupModel, IsUserSpecific: true}
	}
	if ratio, ok := GetGroupModelRatio(usingGroup, modelName); ok {
		return EffectiveGroupRatio{Ratio: ratio, Source: types.GroupRatioSourceGroupModel}
	}
	if ratio, ok := GetGroupGroupRatio(userGroup, usingGroup); ok {
		return EffectiveGroupRatio{Ratio: ratio, Source: types.GroupRatioSourceUserGroup, IsUserSpecific: true}
	}
	if ratio, ok := groupRatioMap.Get(usingGroup); ok {
		return EffectiveGroupRatio{Ratio: ratio, Source: types.GroupRatioSourceGroup}
	}
	common.SysLog("group ratio not found: " + usingGroup)
	return EffectiveGroupRatio{Ratio: 1, Source: types.GroupRatioSourceDefault}
}

func getModelSpecificRatio(modelRatios map[string]float64, modelName string) (float64, bool) {
	if ratio, ok := modelRatios[modelName]; ok {
		return ratio, true
	}
	formattedName := FormatMatchingModelName(modelName)
	if formattedName != modelName {
		ratio, ok := modelRatios[formattedName]
		return ratio, ok
	}
	return 0, false
}

func CheckGroupModelRatio(jsonStr string) error {
	config := make(map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &config); err != nil {
		return err
	}
	for group, modelRatios := range config {
		if strings.TrimSpace(group) == "" {
			return errors.New("group model ratio contains an empty group name")
		}
		for modelName, ratio := range modelRatios {
			if strings.TrimSpace(modelName) == "" {
				return fmt.Errorf("group model ratio contains an empty model name: %s", group)
			}
			if ratio < 0 {
				return fmt.Errorf("group model ratio must be not less than 0: %s/%s", group, modelName)
			}
		}
	}
	return nil
}

func CheckUserGroupModelRatio(jsonStr string) error {
	config := make(map[string]map[string]map[string]float64)
	if err := common.Unmarshal([]byte(jsonStr), &config); err != nil {
		return err
	}
	for userGroup, groupRatios := range config {
		if strings.TrimSpace(userGroup) == "" {
			return errors.New("user group model ratio contains an empty user group name")
		}
		for group, modelRatios := range groupRatios {
			if strings.TrimSpace(group) == "" {
				return fmt.Errorf("user group model ratio contains an empty group name: %s", userGroup)
			}
			for modelName, ratio := range modelRatios {
				if strings.TrimSpace(modelName) == "" {
					return fmt.Errorf("user group model ratio contains an empty model name: %s/%s", userGroup, group)
				}
				if ratio < 0 {
					return fmt.Errorf("user group model ratio must be not less than 0: %s/%s/%s", userGroup, group, modelName)
				}
			}
		}
	}
	return nil
}

func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := common.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
