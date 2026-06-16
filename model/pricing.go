package model

import (
	"fmt"
	"sort"
	"strings"

	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
)

type Pricing struct {
	ModelName              string                  `json:"model_name"`
	Description            string                  `json:"description,omitempty"`
	Icon                   string                  `json:"icon,omitempty"`
	Tags                   string                  `json:"tags,omitempty"`
	Category               string                  `json:"category,omitempty"`
	VendorID               int                     `json:"vendor_id,omitempty"`
	SortOrder              int                     `json:"sort_order"`
	UpdatedTime            int64                   `json:"updated_time,omitempty"`
	AliasModels            []string                `json:"alias_models,omitempty"`
	ContextLength          int                     `json:"context_length,omitempty"`
	MaxOutputTokens        int                     `json:"max_output_tokens,omitempty"`
	KnowledgeCutoff        string                  `json:"knowledge_cutoff,omitempty"`
	ReleaseDate            string                  `json:"release_date,omitempty"`
	ParameterCount         string                  `json:"parameter_count,omitempty"`
	InputModalities        []string                `json:"input_modalities,omitempty"`
	OutputModalities       []string                `json:"output_modalities,omitempty"`
	Capabilities           []string                `json:"capabilities,omitempty"`
	QuotaType              int                     `json:"quota_type"`
	ModelRatio             float64                 `json:"model_ratio"`
	ModelPrice             float64                 `json:"model_price"`
	OwnerBy                string                  `json:"owner_by"`
	CompletionRatio        float64                 `json:"completion_ratio"`
	CacheRatio             *float64                `json:"cache_ratio,omitempty"`
	CreateCacheRatio       *float64                `json:"create_cache_ratio,omitempty"`
	ImageRatio             *float64                `json:"image_ratio,omitempty"`
	AudioRatio             *float64                `json:"audio_ratio,omitempty"`
	AudioCompletionRatio   *float64                `json:"audio_completion_ratio,omitempty"`
	EnableGroup            []string                `json:"enable_groups"`
	SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
	BillingMode            string                  `json:"billing_mode,omitempty"`
	BillingExpr            string                  `json:"billing_expr,omitempty"`
	PricingVersion         string                  `json:"pricing_version,omitempty"`
}

type PricingVendor struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	SortOrder   int    `json:"sort_order"`
}

var (
	pricingMap           []Pricing
	vendorsList          []PricingVendor
	supportedEndpointMap map[string]common.EndpointInfo
	lastGetPricingTime   time.Time
	updatePricingLock    sync.Mutex

	// 缓存映射：模型名 -> 启用分组 / 计费类型
	modelEnableGroups     = make(map[string][]string)
	modelQuotaTypeMap     = make(map[string]int)
	modelEnableGroupsLock = sync.RWMutex{}
)

var (
	modelSupportEndpointTypes = make(map[string][]constant.EndpointType)
	modelSupportEndpointsLock = sync.RWMutex{}
)

func GetPricing() []Pricing {
	if time.Since(lastGetPricingTime) > time.Minute*1 || len(pricingMap) == 0 {
		updatePricingLock.Lock()
		defer updatePricingLock.Unlock()
		// Double check after acquiring the lock
		if time.Since(lastGetPricingTime) > time.Minute*1 || len(pricingMap) == 0 {
			modelSupportEndpointsLock.Lock()
			defer modelSupportEndpointsLock.Unlock()
			updatePricing()
		}
	}
	return pricingMap
}

func InvalidatePricingCache() {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()

	pricingMap = nil
	vendorsList = nil
	lastGetPricingTime = time.Time{}
}

// GetVendors 返回当前定价接口使用到的供应商信息
func GetVendors() []PricingVendor {
	if time.Since(lastGetPricingTime) > time.Minute*1 || len(pricingMap) == 0 {
		// 保证先刷新一次
		GetPricing()
	}
	return vendorsList
}

func GetModelSupportEndpointTypes(model string) []constant.EndpointType {
	if model == "" {
		return make([]constant.EndpointType, 0)
	}
	modelSupportEndpointsLock.RLock()
	defer modelSupportEndpointsLock.RUnlock()
	if endpoints, ok := modelSupportEndpointTypes[model]; ok {
		return endpoints
	}
	return make([]constant.EndpointType, 0)
}

func parseAliasModels(raw string) []string {
	return parseModelListField(raw)
}

func parseModelListField(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	aliases := make([]string, 0)
	for _, alias := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '\n' || r == '\r'
	}) {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if _, ok := seen[alias]; ok {
			continue
		}
		seen[alias] = struct{}{}
		aliases = append(aliases, alias)
	}
	return aliases
}

func updatePricing() {
	//modelRatios := common.GetModelRatios()
	enableAbilities, err := GetAllEnableAbilityWithChannels()
	if err != nil {
		common.SysLog(fmt.Sprintf("GetAllEnableAbilityWithChannels error: %v", err))
		return
	}
	// 预加载模型元数据与供应商一次，避免循环查询
	var allMeta []Model
	_ = DB.Find(&allMeta).Error
	metaMap := make(map[string]*Model)
	primaryMetaMap := make(map[string]*Model)
	aliasToPrimaryModel := make(map[string]string)
	prefixList := make([]*Model, 0)
	suffixList := make([]*Model, 0)
	containsList := make([]*Model, 0)
	for i := range allMeta {
		m := &allMeta[i]
		primaryMetaMap[m.ModelName] = m
		for _, alias := range parseAliasModels(m.AliasModels) {
			if alias == m.ModelName {
				continue
			}
			if _, exists := aliasToPrimaryModel[alias]; !exists {
				aliasToPrimaryModel[alias] = m.ModelName
			}
		}
		if m.NameRule == NameRuleExact {
			metaMap[m.ModelName] = m
		} else {
			switch m.NameRule {
			case NameRulePrefix:
				prefixList = append(prefixList, m)
			case NameRuleSuffix:
				suffixList = append(suffixList, m)
			case NameRuleContains:
				containsList = append(containsList, m)
			}
		}
	}

	// 将非精确规则模型匹配到 metaMap
	for _, m := range prefixList {
		for _, pricingModel := range enableAbilities {
			if strings.HasPrefix(pricingModel.Model, m.ModelName) {
				if _, exists := metaMap[pricingModel.Model]; !exists {
					metaMap[pricingModel.Model] = m
				}
				if pricingModel.Model != m.ModelName {
					if _, exists := aliasToPrimaryModel[pricingModel.Model]; !exists {
						aliasToPrimaryModel[pricingModel.Model] = m.ModelName
					}
				}
			}
		}
	}
	for _, m := range suffixList {
		for _, pricingModel := range enableAbilities {
			if strings.HasSuffix(pricingModel.Model, m.ModelName) {
				if _, exists := metaMap[pricingModel.Model]; !exists {
					metaMap[pricingModel.Model] = m
				}
				if pricingModel.Model != m.ModelName {
					if _, exists := aliasToPrimaryModel[pricingModel.Model]; !exists {
						aliasToPrimaryModel[pricingModel.Model] = m.ModelName
					}
				}
			}
		}
	}
	for _, m := range containsList {
		for _, pricingModel := range enableAbilities {
			if strings.Contains(pricingModel.Model, m.ModelName) {
				if _, exists := metaMap[pricingModel.Model]; !exists {
					metaMap[pricingModel.Model] = m
				}
				if pricingModel.Model != m.ModelName {
					if _, exists := aliasToPrimaryModel[pricingModel.Model]; !exists {
						aliasToPrimaryModel[pricingModel.Model] = m.ModelName
					}
				}
			}
		}
	}
	for alias, primary := range aliasToPrimaryModel {
		if meta, ok := primaryMetaMap[primary]; ok {
			metaMap[alias] = meta
		}
	}

	// 预加载供应商
	var vendors []Vendor
	_ = DB.Order("sort_order ASC").Order("id ASC").Find(&vendors).Error
	vendorMap := make(map[int]*Vendor)
	for i := range vendors {
		vendorMap[vendors[i].Id] = &vendors[i]
	}

	// 初始化默认供应商映射
	initDefaultVendorMapping(metaMap, vendorMap, enableAbilities)

	// 构建对前端友好的供应商列表
	vendorsList = make([]PricingVendor, 0, len(vendorMap))
	for _, v := range vendorMap {
		vendorsList = append(vendorsList, PricingVendor{
			ID:          v.Id,
			Name:        v.Name,
			Description: v.Description,
			Icon:        v.Icon,
			SortOrder:   v.SortOrder,
		})
	}
	sort.SliceStable(vendorsList, func(i, j int) bool {
		if vendorsList[i].SortOrder == vendorsList[j].SortOrder {
			return vendorsList[i].ID < vendorsList[j].ID
		}
		return vendorsList[i].SortOrder < vendorsList[j].SortOrder
	})

	modelGroupsMap := make(map[string]*types.Set[string])
	displayModelAliasesMap := make(map[string]*types.Set[string])
	abilityDisplayModelMap := make(map[string]string)

	for _, ability := range enableAbilities {
		displayModel := ability.Model
		if primary, ok := aliasToPrimaryModel[ability.Model]; ok {
			displayModel = primary
		}
		abilityDisplayModelMap[ability.Model] = displayModel

		if displayModel != ability.Model {
			aliases, ok := displayModelAliasesMap[displayModel]
			if !ok {
				aliases = types.NewSet[string]()
				displayModelAliasesMap[displayModel] = aliases
			}
			aliases.Add(ability.Model)
		}

		groups, ok := modelGroupsMap[displayModel]
		if !ok {
			groups = types.NewSet[string]()
			modelGroupsMap[displayModel] = groups
		}
		groups.Add(ability.Group)
	}

	//这里使用切片而不是Set，因为一个模型可能支持多个端点类型，并且第一个端点是优先使用端点
	modelSupportEndpointsStr := make(map[string][]string)

	// 先根据已有能力填充原生端点
	for _, ability := range enableAbilities {
		displayModel := abilityDisplayModelMap[ability.Model]
		if displayModel == "" {
			displayModel = ability.Model
		}
		endpoints := modelSupportEndpointsStr[displayModel]
		channelTypes := common.GetEndpointTypesByChannelType(ability.ChannelType, ability.Model)
		for _, channelType := range channelTypes {
			if !common.StringsContains(endpoints, string(channelType)) {
				endpoints = append(endpoints, string(channelType))
			}
		}
		modelSupportEndpointsStr[displayModel] = endpoints
	}

	// 再补充模型自定义端点：若配置有效则替换默认端点，不做合并
	for modelName, meta := range metaMap {
		if strings.TrimSpace(meta.Endpoints) == "" {
			continue
		}
		var raw map[string]interface{}
		if err := common.Unmarshal([]byte(meta.Endpoints), &raw); err == nil {
			endpoints := make([]string, 0, len(raw))
			for k, v := range raw {
				switch v.(type) {
				case string, map[string]interface{}:
					if !common.StringsContains(endpoints, k) {
						endpoints = append(endpoints, k)
					}
				}
			}
			if len(endpoints) > 0 {
				modelSupportEndpointsStr[modelName] = endpoints
			}
		}
	}

	modelSupportEndpointTypes = make(map[string][]constant.EndpointType)
	for model, endpoints := range modelSupportEndpointsStr {
		supportedEndpoints := make([]constant.EndpointType, 0)
		for _, endpointStr := range endpoints {
			endpointType := constant.EndpointType(endpointStr)
			supportedEndpoints = append(supportedEndpoints, endpointType)
		}
		modelSupportEndpointTypes[model] = supportedEndpoints
	}

	// 构建全局 supportedEndpointMap（默认 + 自定义覆盖）
	supportedEndpointMap = make(map[string]common.EndpointInfo)
	// 1. 默认端点
	for _, endpoints := range modelSupportEndpointTypes {
		for _, et := range endpoints {
			if info, ok := common.GetDefaultEndpointInfo(et); ok {
				if _, exists := supportedEndpointMap[string(et)]; !exists {
					supportedEndpointMap[string(et)] = info
				}
			}
		}
	}
	// 2. 自定义端点（models 表）覆盖默认
	for _, meta := range metaMap {
		if strings.TrimSpace(meta.Endpoints) == "" {
			continue
		}
		if customEndpoints := parseEndpointInfoMap(meta.Endpoints); len(customEndpoints) > 0 {
			for k, info := range customEndpoints {
				supportedEndpointMap[k] = info
			}
		}
	}

	pricingMap = make([]Pricing, 0)
	for model, groups := range modelGroupsMap {
		pricing := Pricing{
			ModelName:              model,
			EnableGroup:            groups.Items(),
			SupportedEndpointTypes: modelSupportEndpointTypes[model],
		}
		if aliases, ok := displayModelAliasesMap[model]; ok {
			pricing.AliasModels = aliases.Items()
		}

		// 补充模型元数据（描述、标签、供应商、状态）
		if meta, ok := metaMap[model]; ok {
			// 若模型被禁用(status!=1)，则直接跳过，不返回给前端
			if meta.Status != 1 {
				continue
			}
			pricing.Description = meta.Description
			pricing.Icon = meta.Icon
			pricing.Tags = meta.Tags
			pricing.Category = meta.Category
			pricing.VendorID = meta.VendorID
			pricing.SortOrder = meta.SortOrder
			pricing.UpdatedTime = meta.UpdatedTime
			pricing.ContextLength = meta.ContextLength
			pricing.MaxOutputTokens = meta.MaxOutputTokens
			pricing.KnowledgeCutoff = meta.KnowledgeCutoff
			pricing.ReleaseDate = meta.ReleaseDate
			pricing.ParameterCount = meta.ParameterCount
			pricing.InputModalities = parseModelListField(meta.InputModalities)
			pricing.OutputModalities = parseModelListField(meta.OutputModalities)
			pricing.Capabilities = parseModelListField(meta.Capabilities)
		}
		modelPrice, findPrice := ratio_setting.GetModelPrice(model, false)
		if findPrice {
			pricing.ModelPrice = modelPrice
			pricing.QuotaType = 1
		} else {
			modelRatio, _, _ := ratio_setting.GetModelRatio(model)
			pricing.ModelRatio = modelRatio
			pricing.CompletionRatio = ratio_setting.GetCompletionRatio(model)
			pricing.QuotaType = 0
		}
		if cacheRatio, ok := ratio_setting.GetCacheRatio(model); ok {
			pricing.CacheRatio = &cacheRatio
		}
		if createCacheRatio, ok := ratio_setting.GetCreateCacheRatio(model); ok {
			pricing.CreateCacheRatio = &createCacheRatio
		}
		if imageRatio, ok := ratio_setting.GetImageRatio(model); ok {
			pricing.ImageRatio = &imageRatio
		}
		if ratio_setting.ContainsAudioRatio(model) {
			audioRatio := ratio_setting.GetAudioRatio(model)
			pricing.AudioRatio = &audioRatio
		}
		if ratio_setting.ContainsAudioCompletionRatio(model) {
			audioCompletionRatio := ratio_setting.GetAudioCompletionRatio(model)
			pricing.AudioCompletionRatio = &audioCompletionRatio
		}
		if billingMode := billing_setting.GetBillingMode(model); billingMode == "tiered_expr" {
			if expr, ok := billing_setting.GetBillingExpr(model); ok && strings.TrimSpace(expr) != "" {
				pricing.BillingMode = billingMode
				pricing.BillingExpr = expr
			}
		}
		pricingMap = append(pricingMap, pricing)
	}

	sort.SliceStable(pricingMap, func(i, j int) bool {
		if pricingMap[i].SortOrder == pricingMap[j].SortOrder {
			if pricingMap[i].UpdatedTime != pricingMap[j].UpdatedTime {
				return pricingMap[i].UpdatedTime > pricingMap[j].UpdatedTime
			}
			return pricingMap[i].ModelName < pricingMap[j].ModelName
		}
		return pricingMap[i].SortOrder < pricingMap[j].SortOrder
	})

	// 防止大更新后数据不通用
	if len(pricingMap) > 0 {
		pricingMap[0].PricingVersion = "5a90f2b86c08bd983a9a2e6d66c255f4eaef9c4bc934386d2b6ae84ef0ff1f1f"
	}

	// 刷新缓存映射，供高并发快速查询
	modelEnableGroupsLock.Lock()
	modelEnableGroups = make(map[string][]string)
	modelQuotaTypeMap = make(map[string]int)
	for _, p := range pricingMap {
		modelEnableGroups[p.ModelName] = p.EnableGroup
		modelQuotaTypeMap[p.ModelName] = p.QuotaType
	}
	modelEnableGroupsLock.Unlock()

	lastGetPricingTime = time.Now()
}

// GetSupportedEndpointMap 返回全局端点到路径的映射
func GetSupportedEndpointMap() map[string]common.EndpointInfo {
	return supportedEndpointMap
}

func parseEndpointInfoMap(rawEndpoints string) map[string]common.EndpointInfo {
	result := make(map[string]common.EndpointInfo)
	if strings.TrimSpace(rawEndpoints) == "" {
		return result
	}

	var raw map[string]interface{}
	if err := common.Unmarshal([]byte(rawEndpoints), &raw); err != nil {
		return result
	}

	for k, v := range raw {
		switch val := v.(type) {
		case string:
			result[k] = common.EndpointInfo{Path: val, Method: "POST"}
		case map[string]interface{}:
			ep := common.EndpointInfo{Method: "POST"}
			if p, ok := val["path"].(string); ok {
				ep.Path = p
			}
			if m, ok := val["method"].(string); ok {
				ep.Method = strings.ToUpper(m)
			}
			if label, ok := val["label"].(string); ok {
				ep.Label = strings.TrimSpace(label)
			}
			if docsURL, ok := val["docs_url"].(string); ok {
				ep.DocsURL = strings.TrimSpace(docsURL)
			}
			if docsURL, ok := val["docsUrl"].(string); ok && ep.DocsURL == "" {
				ep.DocsURL = strings.TrimSpace(docsURL)
			}
			result[k] = ep
		default:
			// ignore unsupported types
		}
	}

	return result
}
