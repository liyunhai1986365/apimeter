package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

type NewAPISupplierGroupSnapshot struct {
	Group  string   `json:"group"`
	Models []string `json:"models"`
	Source string   `json:"source"`
	Ratio  string   `json:"ratio,omitempty"`
	Desc   string   `json:"desc,omitempty"`
}

type newAPISupplierGroupInfo struct {
	Name  string
	Ratio string
	Desc  string
}

type NewAPISupplierCheckResult struct {
	SupplierID     int                           `json:"supplier_id"`
	Quota          int                           `json:"quota"`
	UsedQuota      int                           `json:"used_quota"`
	Groups         []string                      `json:"groups"`
	GroupRatios    map[string]string             `json:"group_ratios"`
	GroupModels    []NewAPISupplierGroupSnapshot `json:"group_models"`
	GroupKeys      map[string]string             `json:"group_keys"`
	ModelSource    string                        `json:"model_source"`
	AccessToken    string                        `json:"access_token,omitempty"`
	APIKey         string                        `json:"api_key,omitempty"`
	UpstreamUserID int                           `json:"upstream_user_id"`
	Username       string                        `json:"username"`
}

type NewAPISupplierBalanceResult struct {
	SupplierID     int    `json:"supplier_id"`
	Quota          int    `json:"quota"`
	UsedQuota      int    `json:"used_quota"`
	AccessToken    string `json:"access_token,omitempty"`
	UpstreamUserID int    `json:"upstream_user_id"`
	Username       string `json:"username"`
}

type NewAPISupplierConfigureItem struct {
	UpstreamGroup string   `json:"upstream_group"`
	LocalGroup    string   `json:"local_group"`
	Models        []string `json:"models"`
	ChannelType   int      `json:"channel_type"`
}

type NewAPISupplierConfigureRequest struct {
	Items     []NewAPISupplierConfigureItem `json:"items"`
	Overwrite bool                          `json:"overwrite"`
}

type NewAPISupplierConfiguredChannel struct {
	ChannelID     int    `json:"channel_id"`
	Name          string `json:"name"`
	UpstreamGroup string `json:"upstream_group"`
	LocalGroup    string `json:"local_group"`
	Models        string `json:"models"`
	Created       bool   `json:"created"`
}

type NewAPISupplierTestModelRequest struct {
	UpstreamGroup string `json:"upstream_group"`
	Model         string `json:"model"`
	ChannelType   int    `json:"channel_type"`
	EndpointType  string `json:"endpoint_type"`
	Stream        bool   `json:"stream"`
}

type NewAPISupplierPreparedTestChannel struct {
	Channel       *model.Channel `json:"-"`
	UpstreamGroup string         `json:"upstream_group"`
	Model         string         `json:"model"`
	Key           string         `json:"key,omitempty"`
}

type newAPIEnvelope[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type newAPIUserSelf struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	Quota       int    `json:"quota"`
	UsedQuota   int    `json:"used_quota"`
	AccessToken string `json:"access_token"`
}

type newAPIPricingResponse struct {
	Success     bool                 `json:"success"`
	Message     string               `json:"message"`
	Data        []newAPIPricingModel `json:"data"`
	GroupRatio  map[string]any       `json:"group_ratio"`
	UsableGroup map[string]any       `json:"usable_group"`
}

type newAPIPricingModel struct {
	ModelName    string   `json:"model_name"`
	EnableGroups []string `json:"enable_groups"`
}

type newAPITokenListItem struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
	Key  string `json:"key"`
}

func BuildNewAPISupplierChannelPlans(supplier *model.NewAPISupplier, req NewAPISupplierConfigureRequest) ([]model.Channel, error) {
	if supplier == nil {
		return nil, errors.New("supplier is nil")
	}
	if err := validateNewAPISupplierConfigureRequest(supplier, req); err != nil {
		return nil, err
	}
	groupKeys := parseStringMap(supplier.GroupKeysJSON)
	baseURL := strings.TrimRight(strings.TrimSpace(supplier.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("供应商 Base URL 不能为空")
	}
	tag := strings.TrimSpace(supplier.Tag)
	if tag == "" {
		tag = supplier.Name
	}
	groupRatios := parseNewAPISupplierGroupRatios(supplier.GroupModelsJSON)
	plans := make([]model.Channel, 0, len(req.Items))
	for _, item := range req.Items {
		upstreamGroup := strings.TrimSpace(item.UpstreamGroup)
		if upstreamGroup == "" {
			continue
		}
		localGroup := strings.TrimSpace(item.LocalGroup)
		if localGroup == "" {
			localGroup = upstreamGroup
		}
		models := normalizeStringList(item.Models)
		if len(models) == 0 {
			continue
		}
		key := strings.TrimSpace(groupKeys[upstreamGroup])
		if key == "" {
			key = strings.TrimSpace(supplier.APIKey)
		}
		if key == "" {
			key = strings.TrimSpace(supplier.AccessToken)
		}
		if key == "" {
			return nil, fmt.Errorf("供应商分组 %s 缺少可用令牌，请先执行一键检查", upstreamGroup)
		}
		channelType := item.ChannelType
		if channelType == 0 {
			channelType = supplier.ChannelType
		}
		if channelType == 0 {
			channelType = constant.ChannelTypeOpenAI
		}
		plans = append(plans, model.Channel{
			Type:        channelType,
			Key:         key,
			Status:      common.ChannelStatusEnabled,
			Name:        formatNewAPISupplierChannelName(upstreamGroup, groupRatios[upstreamGroup], supplier.Name),
			BaseURL:     common.GetPointer(baseURL),
			Models:      strings.Join(models, ","),
			Group:       localGroup,
			Balance:     float64(supplier.Quota),
			Tag:         common.GetPointer(tag),
			Weight:      supplier.Weight,
			Priority:    supplier.Priority,
			AutoBan:     common.GetPointer(supplier.AutoBan),
			OtherInfo:   fmt.Sprintf("newapi-supplier:%d;upstream_group:%s", supplier.Id, upstreamGroup),
			CreatedTime: common.GetTimestamp(),
		})
	}
	if len(plans) == 0 {
		return nil, errors.New("请选择至少一个分组和模型")
	}
	return plans, nil
}

func ConfigureNewAPISupplierChannels(ctx context.Context, supplier *model.NewAPISupplier, req NewAPISupplierConfigureRequest) ([]NewAPISupplierConfiguredChannel, error) {
	if err := validateNewAPISupplierConfigureRequest(supplier, req); err != nil {
		return nil, err
	}
	if err := ensureNewAPISupplierConfigureKeys(ctx, supplier, req); err != nil {
		return nil, err
	}
	plans, err := BuildNewAPISupplierChannelPlans(supplier, req)
	if err != nil {
		return nil, err
	}
	results := make([]NewAPISupplierConfiguredChannel, 0, len(plans))
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		for _, plan := range plans {
			upstreamGroup := parseOtherInfoValue(plan.OtherInfo, "upstream_group")
			var binding model.NewAPISupplierChannel
			bindErr := tx.Where("supplier_id = ? AND upstream_group = ?", supplier.Id, upstreamGroup).First(&binding).Error
			created := false
			if bindErr == nil {
				plan.Id = binding.ChannelID
				if err := tx.Model(&model.Channel{}).Where("id = ?", binding.ChannelID).Updates(&plan).Error; err != nil {
					return err
				}
				if err := tx.First(&plan, binding.ChannelID).Error; err != nil {
					return err
				}
				if err := plan.UpdateAbilities(tx); err != nil {
					return err
				}
				binding.LocalGroup = plan.Group
				binding.Models = plan.Models
				binding.UpdatedTime = common.GetTimestamp()
				if err := tx.Save(&binding).Error; err != nil {
					return err
				}
			} else {
				if err := tx.Create(&plan).Error; err != nil {
					return err
				}
				if err := plan.AddAbilities(tx); err != nil {
					return err
				}
				now := common.GetTimestamp()
				binding = model.NewAPISupplierChannel{
					SupplierID:    supplier.Id,
					ChannelID:     plan.Id,
					UpstreamGroup: upstreamGroup,
					LocalGroup:    plan.Group,
					Models:        plan.Models,
					SyncMode:      model.NewAPISupplierSyncModeManaged,
					CreatedTime:   now,
					UpdatedTime:   now,
				}
				if err := tx.Create(&binding).Error; err != nil {
					return err
				}
				created = true
			}
			results = append(results, NewAPISupplierConfiguredChannel{
				ChannelID:     plan.Id,
				Name:          plan.Name,
				UpstreamGroup: upstreamGroup,
				LocalGroup:    plan.Group,
				Models:        plan.Models,
				Created:       created,
			})
		}
		return tx.Model(&model.NewAPISupplier{}).Where("id = ?", supplier.Id).Updates(map[string]any{
			"selected_models_json": mustJSON(req.Items),
			"last_configure_time":  common.GetTimestamp(),
			"updated_time":         common.GetTimestamp(),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	model.InitChannelCache()
	ResetProxyClientCache()
	return results, nil
}

func PrepareNewAPISupplierGroupTestChannel(ctx context.Context, supplier *model.NewAPISupplier, req NewAPISupplierTestModelRequest) (*NewAPISupplierPreparedTestChannel, error) {
	if supplier == nil {
		return nil, errors.New("supplier is nil")
	}
	supplier.Normalize()
	upstreamGroup := strings.TrimSpace(req.UpstreamGroup)
	if upstreamGroup == "" {
		return nil, errors.New("供应商分组不能为空")
	}
	groupModels := parseNewAPISupplierGroupSnapshots(supplier.GroupModelsJSON)
	allowedModels, ok := groupModels[upstreamGroup]
	if !ok || len(allowedModels) == 0 {
		return nil, fmt.Errorf("供应商分组 %s 没有模型广场模型数据，请重新检查供应商", upstreamGroup)
	}
	models := mapKeys(allowedModels)
	testModel := strings.TrimSpace(req.Model)
	if testModel == "" {
		testModel = models[0]
	}
	if _, ok := allowedModels[testModel]; !ok {
		return nil, fmt.Errorf("模型 %s 不属于供应商分组 %s，请重新检查供应商后再测试", testModel, upstreamGroup)
	}
	if err := ensureNewAPISupplierConfigureKeys(ctx, supplier, NewAPISupplierConfigureRequest{
		Items: []NewAPISupplierConfigureItem{{
			UpstreamGroup: upstreamGroup,
			LocalGroup:    upstreamGroup,
			Models:        models,
			ChannelType:   req.ChannelType,
		}},
	}); err != nil {
		return nil, err
	}
	groupKeys := parseStringMap(supplier.GroupKeysJSON)
	key := strings.TrimSpace(groupKeys[upstreamGroup])
	if key == "" {
		refreshed, err := model.GetNewAPISupplierByID(supplier.Id)
		if err != nil {
			return nil, err
		}
		*supplier = *refreshed
		groupKeys = parseStringMap(supplier.GroupKeysJSON)
		key = strings.TrimSpace(groupKeys[upstreamGroup])
	}
	if key == "" {
		return nil, fmt.Errorf("供应商分组 %s 缺少可用令牌", upstreamGroup)
	}
	channelType := req.ChannelType
	if channelType == 0 {
		channelType = supplier.ChannelType
	}
	if channelType == 0 {
		channelType = constant.ChannelTypeOpenAI
	}
	tag := strings.TrimSpace(supplier.Tag)
	if tag == "" {
		tag = supplier.Name
	}
	baseURL := strings.TrimRight(strings.TrimSpace(supplier.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("供应商 Base URL 不能为空")
	}
	channel := &model.Channel{
		Type:      channelType,
		Key:       key,
		Status:    common.ChannelStatusEnabled,
		Name:      fmt.Sprintf("%s / %s test", supplier.Name, upstreamGroup),
		BaseURL:   common.GetPointer(baseURL),
		Models:    testModel,
		Group:     upstreamGroup,
		Balance:   float64(supplier.Quota),
		Tag:       common.GetPointer(tag),
		Weight:    supplier.Weight,
		Priority:  supplier.Priority,
		AutoBan:   common.GetPointer(supplier.AutoBan),
		OtherInfo: fmt.Sprintf("newapi-supplier:%d;upstream_group:%s;test_model:true", supplier.Id, upstreamGroup),
	}
	return &NewAPISupplierPreparedTestChannel{
		Channel:       channel,
		UpstreamGroup: upstreamGroup,
		Model:         testModel,
		Key:           key,
	}, nil
}

func ensureNewAPISupplierConfigureKeys(ctx context.Context, supplier *model.NewAPISupplier, req NewAPISupplierConfigureRequest) error {
	if supplier == nil {
		return errors.New("supplier is nil")
	}
	supplier.Normalize()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	_, accessToken, upstreamUserID, err := fetchNewAPISupplierSelf(ctx, client, supplier)
	if err != nil {
		return err
	}
	groupKeys := parseStringMap(supplier.GroupKeysJSON)
	changed := false
	for _, item := range req.Items {
		group := strings.TrimSpace(item.UpstreamGroup)
		if group == "" || strings.TrimSpace(groupKeys[group]) != "" {
			continue
		}
		key, err := ensureNewAPIGroupToken(ctx, client, supplier.BaseURL, accessToken, upstreamUserID, group, item.Models)
		if err != nil {
			return err
		}
		if strings.TrimSpace(key) != "" {
			groupKeys[group] = key
			changed = true
		}
	}
	if !changed {
		return nil
	}
	supplier.GroupKeysJSON = mustJSON(groupKeys)
	supplier.AccessToken = accessToken
	supplier.UpstreamUserID = upstreamUserID
	update := map[string]any{
		"access_token":     accessToken,
		"upstream_user_id": upstreamUserID,
		"group_keys_json":  supplier.GroupKeysJSON,
		"updated_time":     common.GetTimestamp(),
	}
	if len(groupKeys) == 1 {
		for _, key := range groupKeys {
			supplier.APIKey = key
			update["api_key"] = key
		}
	}
	return model.DB.Model(&model.NewAPISupplier{}).Where("id = ?", supplier.Id).Updates(update).Error
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func CheckNewAPISupplier(ctx context.Context, supplier *model.NewAPISupplier) (*NewAPISupplierCheckResult, error) {
	if supplier == nil {
		return nil, errors.New("supplier is nil")
	}
	supplier.Normalize()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	self, accessToken, upstreamUserID, err := fetchNewAPISupplierSelf(ctx, client, supplier)
	if err != nil {
		markSupplierCheckError(supplier.Id, err)
		return nil, err
	}
	groupInfos, groupModels, modelSource := fetchNewAPIModelPlazaGroups(ctx, client, supplier.BaseURL, accessToken, upstreamUserID)
	groups := newAPIGroupNames(groupInfos)
	if len(groups) == 0 {
		err := errors.New("供应商模型广场接口未返回可用分组模型数据")
		markSupplierCheckError(supplier.Id, err)
		return nil, err
	}
	snapshots := make([]NewAPISupplierGroupSnapshot, 0, len(groups))
	groupRatios := make(map[string]string, len(groupInfos))
	groupInfoByName := make(map[string]newAPISupplierGroupInfo, len(groupInfos))
	for _, info := range groupInfos {
		if strings.TrimSpace(info.Name) == "" {
			continue
		}
		groupInfoByName[info.Name] = info
		if strings.TrimSpace(info.Ratio) != "" {
			groupRatios[info.Name] = strings.TrimSpace(info.Ratio)
		}
	}
	for _, group := range groups {
		groupInfo := groupInfoByName[group]
		snapshots = append(snapshots, NewAPISupplierGroupSnapshot{
			Group:  group,
			Models: groupModels[group],
			Source: modelSource,
			Ratio:  groupInfo.Ratio,
			Desc:   groupInfo.Desc,
		})
	}
	result := &NewAPISupplierCheckResult{
		SupplierID:     supplier.Id,
		Quota:          self.Quota,
		UsedQuota:      self.UsedQuota,
		Groups:         groups,
		GroupRatios:    groupRatios,
		GroupModels:    snapshots,
		GroupKeys:      map[string]string{},
		ModelSource:    modelSource,
		AccessToken:    accessToken,
		UpstreamUserID: upstreamUserID,
		Username:       firstNonEmpty(self.Username, supplier.Username),
	}
	updates := map[string]any{
		"access_token":      accessToken,
		"upstream_user_id":  upstreamUserID,
		"upstream_username": result.Username,
		"quota":             self.Quota,
		"used_quota":        self.UsedQuota,
		"groups_json":       mustJSON(groups),
		"group_models_json": mustJSON(snapshots),
		"model_source":      modelSource,
		"status":            model.NewAPISupplierStatusEnabled,
		"last_check_time":   common.GetTimestamp(),
		"last_error":        "",
		"updated_time":      common.GetTimestamp(),
	}
	if err := model.DB.Model(&model.NewAPISupplier{}).Where("id = ?", supplier.Id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return result, nil
}

func QueryNewAPISupplierBalance(ctx context.Context, supplier *model.NewAPISupplier) (*NewAPISupplierBalanceResult, error) {
	if supplier == nil {
		return nil, errors.New("supplier is nil")
	}
	supplier.Normalize()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	self, accessToken, upstreamUserID, err := fetchNewAPISupplierSelf(ctx, client, supplier)
	if err != nil {
		markSupplierCheckError(supplier.Id, err)
		return nil, err
	}
	username := firstNonEmpty(self.Username, supplier.Username)
	updates := map[string]any{
		"access_token":      accessToken,
		"upstream_user_id":  upstreamUserID,
		"upstream_username": username,
		"quota":             self.Quota,
		"used_quota":        self.UsedQuota,
		"status":            model.NewAPISupplierStatusEnabled,
		"last_check_time":   common.GetTimestamp(),
		"last_error":        "",
		"updated_time":      common.GetTimestamp(),
	}
	if err := model.DB.Model(&model.NewAPISupplier{}).Where("id = ?", supplier.Id).Updates(updates).Error; err != nil {
		return nil, err
	}
	return &NewAPISupplierBalanceResult{
		SupplierID:     supplier.Id,
		Quota:          self.Quota,
		UsedQuota:      self.UsedQuota,
		AccessToken:    accessToken,
		UpstreamUserID: upstreamUserID,
		Username:       username,
	}, nil
}

func fetchNewAPISupplierSelf(ctx context.Context, client *http.Client, supplier *model.NewAPISupplier) (newAPIUserSelf, string, int, error) {
	accessToken := strings.TrimSpace(supplier.AccessToken)
	upstreamUserID := supplier.UpstreamUserID
	if accessToken == "" || upstreamUserID == 0 {
		loginToken, userID, err := loginNewAPI(ctx, client, supplier)
		if err != nil {
			return newAPIUserSelf{}, "", 0, err
		}
		accessToken = loginToken
		upstreamUserID = userID
	}
	self, err := fetchNewAPISelf(ctx, client, supplier.BaseURL, accessToken, upstreamUserID)
	if err != nil {
		return newAPIUserSelf{}, "", 0, err
	}
	if self.AccessToken != "" {
		accessToken = self.AccessToken
	}
	if self.Id != 0 {
		upstreamUserID = self.Id
	}
	return self, accessToken, upstreamUserID, nil
}

func loginNewAPI(ctx context.Context, client *http.Client, supplier *model.NewAPISupplier) (string, int, error) {
	if strings.TrimSpace(supplier.Username) == "" || strings.TrimSpace(supplier.Password) == "" {
		return "", 0, errors.New("供应商账号密码不能为空")
	}
	payload, _ := common.Marshal(map[string]string{
		"username": supplier.Username,
		"password": supplier.Password,
	})
	var envelope newAPIEnvelope[struct {
		Id int `json:"id"`
	}]
	if err := doNewAPIJSON(ctx, client, http.MethodPost, supplier.BaseURL, "/api/user/login", "", 0, payload, &envelope); err != nil {
		return "", 0, err
	}
	if !envelope.Success {
		return "", 0, fmt.Errorf("上游登录失败: %s", envelope.Message)
	}
	if envelope.Data.Id == 0 {
		return "", 0, errors.New("上游登录成功但未返回用户 ID")
	}
	accessToken, err := generateNewAPIAccessToken(ctx, client, supplier.BaseURL, envelope.Data.Id)
	if err != nil {
		return "", 0, err
	}
	return accessToken, envelope.Data.Id, nil
}

func generateNewAPIAccessToken(ctx context.Context, client *http.Client, baseURL string, userID int) (string, error) {
	var envelope newAPIEnvelope[string]
	if err := doNewAPIJSON(ctx, client, http.MethodGet, baseURL, "/api/user/token", "", userID, nil, &envelope); err != nil {
		return "", err
	}
	if !envelope.Success || strings.TrimSpace(envelope.Data) == "" {
		return "", fmt.Errorf("生成上游 access token 失败: %s", envelope.Message)
	}
	return strings.TrimSpace(envelope.Data), nil
}

func fetchNewAPISelf(ctx context.Context, client *http.Client, baseURL, accessToken string, userID int) (newAPIUserSelf, error) {
	var envelope newAPIEnvelope[newAPIUserSelf]
	err := doNewAPIJSON(ctx, client, http.MethodGet, baseURL, "/api/user/self", accessToken, userID, nil, &envelope)
	if err != nil {
		return newAPIUserSelf{}, err
	}
	if !envelope.Success {
		return newAPIUserSelf{}, fmt.Errorf("获取上游用户信息失败: %s", envelope.Message)
	}
	return envelope.Data, nil
}

func fetchNewAPIGroups(ctx context.Context, client *http.Client, baseURL, accessToken string, userID int) ([]newAPISupplierGroupInfo, error) {
	var envelope newAPIEnvelope[map[string]any]
	err := doNewAPIJSON(ctx, client, http.MethodGet, baseURL, "/api/user/self/groups", accessToken, userID, nil, &envelope)
	if err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("获取上游分组失败: %s", envelope.Message)
	}
	groups := make([]newAPISupplierGroupInfo, 0, len(envelope.Data))
	for group, value := range envelope.Data {
		groups = append(groups, parseNewAPIGroupInfo(group, value))
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	return groups, nil
}

func parseNewAPIGroupInfo(group string, value any) newAPISupplierGroupInfo {
	info := newAPISupplierGroupInfo{Name: strings.TrimSpace(group)}
	switch v := value.(type) {
	case map[string]any:
		info.Ratio = stringifyGroupRatio(firstMapValue(v, "ratio", "group_ratio", "rate"))
		info.Desc = stringifyGroupText(firstMapValue(v, "desc", "description", "name"))
	case string:
		text := strings.TrimSpace(v)
		if isNumericGroupRatioString(text) {
			info.Ratio = text
		} else {
			info.Desc = text
		}
	case float64:
		info.Ratio = stringifyGroupRatio(v)
	case int:
		info.Ratio = stringifyGroupRatio(v)
	case int64:
		info.Ratio = stringifyGroupRatio(v)
	case jsonNumber:
		info.Ratio = stringifyGroupRatio(v)
	}
	return info
}

func isNumericGroupRatioString(value string) bool {
	if value == "" {
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func stringifyGroupText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

type jsonNumber interface {
	String() string
}

func firstMapValue(data map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}

func stringifyGroupRatio(value any) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
	case float32:
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case jsonNumber:
		return strings.TrimSpace(v.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func newAPIGroupNames(infos []newAPISupplierGroupInfo) []string {
	groups := make([]string, 0, len(infos))
	for _, info := range infos {
		group := strings.TrimSpace(info.Name)
		if group != "" {
			groups = append(groups, group)
		}
	}
	sort.Strings(groups)
	return groups
}

func fetchNewAPIModelPlazaGroups(ctx context.Context, client *http.Client, baseURL, accessToken string, userID int) ([]newAPISupplierGroupInfo, map[string][]string, string) {
	var response newAPIPricingResponse
	if err := doNewAPIJSON(ctx, client, http.MethodGet, baseURL, "/api/pricing", accessToken, userID, nil, &response); err != nil {
		return nil, nil, ""
	}
	if !response.Success || len(response.Data) == 0 {
		return nil, nil, ""
	}
	groupModels := make(map[string][]string)
	groupInfoByName := make(map[string]newAPISupplierGroupInfo)
	for group, ratio := range response.GroupRatio {
		groupName := strings.TrimSpace(group)
		if groupName == "" {
			continue
		}
		info := groupInfoByName[groupName]
		info.Name = groupName
		info.Ratio = stringifyGroupRatio(ratio)
		groupInfoByName[groupName] = info
	}
	for group, value := range response.UsableGroup {
		groupName := strings.TrimSpace(group)
		if groupName == "" {
			continue
		}
		info := groupInfoByName[groupName]
		if info.Name == "" {
			info.Name = groupName
		}
		usableInfo := parseNewAPIGroupInfo(groupName, value)
		if usableInfo.Ratio != "" {
			info.Ratio = usableInfo.Ratio
		}
		if usableInfo.Desc != "" {
			info.Desc = usableInfo.Desc
		}
		groupInfoByName[groupName] = info
	}
	for _, item := range response.Data {
		modelName := strings.TrimSpace(item.ModelName)
		if modelName == "" {
			continue
		}
		for _, group := range item.EnableGroups {
			groupName := strings.TrimSpace(group)
			if groupName == "" || groupName == "all" || groupName == "auto" {
				continue
			}
			groupModels[groupName] = append(groupModels[groupName], modelName)
			info := groupInfoByName[groupName]
			if info.Name == "" {
				info.Name = groupName
			}
			groupInfoByName[groupName] = info
		}
	}
	groups := make([]newAPISupplierGroupInfo, 0, len(groupInfoByName))
	for groupName, info := range groupInfoByName {
		models := normalizeStringList(groupModels[groupName])
		if len(models) == 0 {
			continue
		}
		groupModels[groupName] = models
		groups = append(groups, info)
	}
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].Name < groups[j].Name
	})
	if len(groups) == 0 {
		return nil, nil, ""
	}
	return groups, groupModels, "pricing"
}

func validateNewAPISupplierConfigureRequest(supplier *model.NewAPISupplier, req NewAPISupplierConfigureRequest) error {
	if strings.TrimSpace(supplier.ModelSource) != "pricing" {
		return errors.New("请先重新检查供应商，从模型广场加载分组模型后再配置渠道")
	}
	groupModels := parseNewAPISupplierGroupSnapshots(supplier.GroupModelsJSON)
	if len(groupModels) == 0 {
		return errors.New("请先检查供应商，加载模型广场分组模型")
	}
	for _, item := range req.Items {
		group := strings.TrimSpace(item.UpstreamGroup)
		if group == "" {
			continue
		}
		allowedModels, ok := groupModels[group]
		if !ok || len(allowedModels) == 0 {
			return fmt.Errorf("供应商分组 %s 没有模型广场模型数据，请重新检查供应商", group)
		}
		for _, modelName := range normalizeStringList(item.Models) {
			if _, ok := allowedModels[modelName]; !ok {
				return fmt.Errorf("模型 %s 不属于供应商分组 %s，请重新检查供应商后再配置", modelName, group)
			}
		}
	}
	return nil
}

func parseNewAPISupplierGroupSnapshots(raw string) map[string]map[string]struct{} {
	result := map[string]map[string]struct{}{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	var snapshots []NewAPISupplierGroupSnapshot
	if err := common.Unmarshal([]byte(raw), &snapshots); err != nil {
		return result
	}
	for _, snapshot := range snapshots {
		group := strings.TrimSpace(snapshot.Group)
		if group == "" || strings.TrimSpace(snapshot.Source) != "pricing" {
			continue
		}
		modelSet := make(map[string]struct{}, len(snapshot.Models))
		for _, modelName := range normalizeStringList(snapshot.Models) {
			modelSet[modelName] = struct{}{}
		}
		if len(modelSet) > 0 {
			result[group] = modelSet
		}
	}
	return result
}

func parseNewAPISupplierGroupRatios(raw string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	var snapshots []NewAPISupplierGroupSnapshot
	if err := common.Unmarshal([]byte(raw), &snapshots); err != nil {
		return result
	}
	for _, snapshot := range snapshots {
		group := strings.TrimSpace(snapshot.Group)
		ratio := strings.TrimSpace(snapshot.Ratio)
		if group != "" && ratio != "" {
			result[group] = ratio
		}
	}
	return result
}

func formatNewAPISupplierChannelName(group string, ratio string, channelName string) string {
	group = strings.TrimSpace(group)
	ratio = strings.TrimSpace(ratio)
	channelName = strings.TrimSpace(channelName)
	if ratio == "" {
		ratio = "未配置倍率"
	}
	return fmt.Sprintf("%s - %s - %s", group, ratio, channelName)
}

func ensureNewAPIGroupToken(ctx context.Context, client *http.Client, baseURL, accessToken string, userID int, group string, models []string) (string, error) {
	tokenName := strings.TrimSpace(group)
	list, err := fetchNewAPITokenList(ctx, client, baseURL, "/api/token/?p=1&page_size=100", accessToken, userID)
	if err == nil {
		for _, item := range list {
			if item.Name != tokenName || item.Id == 0 {
				continue
			}
			if isUsableNewAPIKey(item.Key) {
				return normalizeAPIKey(item.Key), nil
			}
			key, err := fetchNewAPITokenKey(ctx, client, baseURL, accessToken, userID, item.Id)
			if err == nil && strings.TrimSpace(key) != "" {
				return normalizeAPIKey(key), nil
			}
			return "", err
		}
	}
	payload, _ := common.Marshal(map[string]any{
		"name":                 tokenName,
		"expired_time":         -1,
		"unlimited_quota":      true,
		"model_limits_enabled": len(models) > 0,
		"model_limits":         strings.Join(normalizeStringList(models), ","),
		"group":                group,
	})
	var createResp newAPIEnvelope[json.RawMessage]
	if err := doNewAPIJSON(ctx, client, http.MethodPost, baseURL, "/api/token/", accessToken, userID, payload, &createResp); err != nil {
		return "", err
	}
	if !createResp.Success {
		return "", fmt.Errorf("创建上游分组令牌失败: %s", createResp.Message)
	}
	token := parseNewAPITokenDetail(createResp.Data)
	if strings.TrimSpace(token.Key) != "" {
		return normalizeAPIKey(token.Key), nil
	}
	tokenID := token.Id
	if tokenID == 0 {
		var findErr error
		var foundKey string
		tokenID, foundKey, findErr = findNewAPITokenByName(ctx, client, baseURL, accessToken, userID, tokenName)
		if findErr != nil {
			return "", fmt.Errorf("创建上游令牌成功但无法定位令牌 ID: %w", findErr)
		}
		if isUsableNewAPIKey(foundKey) {
			return normalizeAPIKey(foundKey), nil
		}
	}
	if tokenID == 0 {
		return "", errors.New("创建上游令牌成功但无法定位令牌 ID")
	}
	key, err := fetchNewAPITokenKey(ctx, client, baseURL, accessToken, userID, tokenID)
	if err == nil && strings.TrimSpace(key) != "" {
		return normalizeAPIKey(key), nil
	}
	detail, detailErr := fetchNewAPITokenDetail(ctx, client, baseURL, accessToken, userID, tokenID)
	if detailErr == nil && isUsableNewAPIKey(detail.Key) {
		return normalizeAPIKey(detail.Key), nil
	}
	if err != nil {
		return "", err
	}
	return "", detailErr
}

func findNewAPITokenByName(ctx context.Context, client *http.Client, baseURL string, accessToken string, userID int, tokenName string) (int, string, error) {
	path := "/api/token/search?keyword=" + url.QueryEscape(tokenName) + "&p=1&page_size=20"
	list, err := fetchNewAPITokenList(ctx, client, baseURL, path, accessToken, userID)
	if err != nil {
		return 0, "", err
	}
	for _, item := range list {
		if item.Id != 0 && item.Name == tokenName {
			return item.Id, item.Key, nil
		}
	}
	return 0, "", nil
}

func fetchNewAPITokenKey(ctx context.Context, client *http.Client, baseURL string, accessToken string, userID int, tokenID int) (string, error) {
	var keyEnvelope newAPIEnvelope[struct {
		Key string `json:"key"`
	}]
	if err := doNewAPIJSON(ctx, client, http.MethodPost, baseURL, fmt.Sprintf("/api/token/%d/key", tokenID), accessToken, userID, []byte("{}"), &keyEnvelope); err != nil {
		return "", err
	}
	if !keyEnvelope.Success || strings.TrimSpace(keyEnvelope.Data.Key) == "" {
		return "", fmt.Errorf("获取上游令牌密钥失败: %s", keyEnvelope.Message)
	}
	return keyEnvelope.Data.Key, nil
}

func fetchNewAPITokenDetail(ctx context.Context, client *http.Client, baseURL string, accessToken string, userID int, tokenID int) (newAPITokenListItem, error) {
	var envelope newAPIEnvelope[json.RawMessage]
	if err := doNewAPIJSON(ctx, client, http.MethodGet, baseURL, fmt.Sprintf("/api/token/%d", tokenID), accessToken, userID, nil, &envelope); err != nil {
		return newAPITokenListItem{}, err
	}
	if !envelope.Success {
		return newAPITokenListItem{}, fmt.Errorf("获取上游令牌详情失败: %s", envelope.Message)
	}
	return parseNewAPITokenDetail(envelope.Data), nil
}

func fetchNewAPITokenList(ctx context.Context, client *http.Client, baseURL string, path string, accessToken string, userID int) ([]newAPITokenListItem, error) {
	var envelope newAPIEnvelope[json.RawMessage]
	if err := doNewAPIJSON(ctx, client, http.MethodGet, baseURL, path, accessToken, userID, nil, &envelope); err != nil {
		return nil, err
	}
	if !envelope.Success {
		return nil, fmt.Errorf("获取上游令牌列表失败: %s", envelope.Message)
	}
	return parseNewAPITokenList(envelope.Data), nil
}

func parseNewAPITokenList(raw json.RawMessage) []newAPITokenListItem {
	var direct []newAPITokenListItem
	if err := common.Unmarshal(raw, &direct); err == nil {
		return direct
	}
	var wrapped struct {
		Items []newAPITokenListItem `json:"items"`
	}
	if err := common.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Items
	}
	return nil
}

func parseNewAPITokenDetail(raw json.RawMessage) newAPITokenListItem {
	var token newAPITokenListItem
	if err := common.Unmarshal(raw, &token); err == nil {
		return token
	}
	return newAPITokenListItem{}
}

func isUsableNewAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" || strings.Contains(key, "*") {
		return false
	}
	return true
}

func doNewAPIJSON(ctx context.Context, client *http.Client, method string, baseURL string, path string, accessToken string, userID int, body []byte, target any) error {
	fullURL := strings.TrimRight(baseURL, "/") + path
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accessToken != "" {
		req.Header.Set("Authorization", accessToken)
	}
	if userID > 0 {
		req.Header.Set("New-Api-User", fmt.Sprintf("%d", userID))
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("上游请求失败: %s %s 返回 %d", method, fullURL, resp.StatusCode)
	}
	return common.DecodeJson(resp.Body, target)
}

func markSupplierCheckError(supplierID int, err error) {
	if supplierID == 0 || err == nil {
		return
	}
	_ = model.DB.Model(&model.NewAPISupplier{}).Where("id = ?", supplierID).Updates(map[string]any{
		"status":          model.NewAPISupplierStatusError,
		"last_error":      err.Error(),
		"last_check_time": common.GetTimestamp(),
		"updated_time":    common.GetTimestamp(),
	}).Error
}

func normalizeStringList(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func parseStringMap(raw string) map[string]string {
	result := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return result
	}
	_ = common.Unmarshal([]byte(raw), &result)
	return result
}

func mustJSON(v any) string {
	b, err := common.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func parseOtherInfoValue(raw string, key string) string {
	for _, part := range strings.Split(raw, ";") {
		name, value, ok := strings.Cut(part, ":")
		if ok && name == key {
			return value
		}
	}
	return ""
}

func normalizeAPIKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "sk-") {
		return key
	}
	return "sk-" + key
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
