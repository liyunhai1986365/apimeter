package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func RelayConfigurableResource(c *gin.Context) {
	profileID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceProfileID))
	resourceID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceID))

	channelModel, profile, resource, err := selectConfigurableResourceRoute(c, profileID, resourceID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	requestModel := configurableResourceRequestModel(c, resource)
	if requestModel != "" {
		common.SetContextKey(c, constant.ContextKeyOriginalModel, requestModel)
		c.Set("upstream_model_name", requestModel)
	}
	c.Set(middleware.ContextKeyConfigurableResourceProfileID, profile.ID)
	c.Set(middleware.ContextKeyConfigurableResourceID, resource.ID)
	if apiErr := middleware.SetupContextForSelectedChannel(c, channelModel, requestModel); apiErr != nil {
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}
	billingInfo, apiErr := beginConfigurableResourceBilling(c, profile, resource, requestModel)
	if apiErr != nil {
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}
	billingSettled := false
	defer func() {
		if billingInfo != nil && !billingSettled && billingInfo.Billing != nil {
			billingInfo.Billing.Refund(c)
		}
	}()

	client, err := service.GetHttpClientWithProxy(channelModel.GetSetting().Proxy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if client == nil {
		client = http.DefaultClient
	}
	preResults, err := executeConfigurableResourcePreRequests(c, client, channelModel, resource)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	upstreamReq, err := buildConfigurableResourceRequestWithPreResults(c, channelModel, resource, preResults)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf(
		"configurable resource proxy: method=%s path=%s channel_id=%d profile=%s resource=%s upstream=%s",
		c.Request.Method,
		c.Request.URL.Path,
		channelModel.Id,
		profile.ID,
		resource.ID,
		upstreamReq.URL.Redacted(),
	))
	resp, err := client.Do(upstreamReq)
	if err != nil {
		if billingInfo != nil {
			service.ChargeViolationFeeIfNeeded(c, billingInfo, types.NewError(err, types.ErrorCodeDoRequestFailed))
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf(
		"configurable resource response: method=%s path=%s channel_id=%d profile=%s resource=%s upstream_status=%d",
		c.Request.Method,
		c.Request.URL.Path,
		channelModel.Id,
		profile.ID,
		resource.ID,
		resp.StatusCode,
	))
	if !resource.Response.Passthrough || len(resource.Response.Fields) > 0 {
		responseBody, err = configurable.BuildConfiguredResponse(resource.Response, responseBody, &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{
				ChannelType:    channelModel.Type,
				ChannelId:      channelModel.Id,
				ChannelBaseUrl: channelModel.GetBaseURL(),
				ApiKey:         common.GetContextKeyString(c, constant.ContextKeyChannelKey),
				ChannelSetting: channelModel.GetSetting(),
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if billingInfo != nil {
		if resp.StatusCode >= http.StatusBadRequest {
			service.ChargeViolationFeeIfNeeded(c, billingInfo, types.NewErrorWithStatusCode(fmt.Errorf("configurable resource upstream status %d", resp.StatusCode), types.ErrorCodeBadResponseStatusCode, resp.StatusCode))
		} else if err := service.SettleBilling(c, billingInfo, billingInfo.PriceData.Quota); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else {
			billingSettled = true
		}
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.TrimSpace(contentType) == "" {
		contentType = "application/json"
	}
	c.Data(resp.StatusCode, contentType, responseBody)
}

func selectConfigurableResourceRoute(c *gin.Context, profileID, resourceID string) (*model.Channel, *configurable.Profile, *configurable.ResourceConfig, error) {
	if profileID != "" && resourceID != "" {
		profile, ok := configurable.GetProfile(profileID)
		if !ok {
			return nil, nil, nil, fmt.Errorf("configurable profile %s not found", profileID)
		}
		resource, ok := profile.ResourceByID(resourceID)
		if !ok {
			return nil, nil, nil, fmt.Errorf("configurable resource %s not found", resourceID)
		}
		channelModel, err := selectConfigurableResourceChannel(c, profileID)
		if err != nil {
			return nil, nil, nil, err
		}
		return channelModel, profile, resource, nil
	}

	channelModel, profile, resource, err := selectConfigurableResourceChannelForEndpoint(c, c.Request.Method, c.Request.URL.Path)
	if err != nil {
		return nil, nil, nil, err
	}
	return channelModel, profile, resource, nil
}

func selectConfigurableResourceChannel(c *gin.Context, profileID string) (*model.Channel, error) {
	if channelIDRaw, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
		channelID, parseErr := parseSpecificChannelID(channelIDRaw)
		if parseErr != nil {
			return nil, parseErr
		}
		channelModel, err := model.GetChannelById(channelID, true)
		if err != nil {
			return nil, err
		}
		if configurableResourceChannelMatches(channelModel, profileID, "") {
			return channelModel, nil
		}
		return nil, fmt.Errorf("specific channel %d does not match configurable profile %s", channelID, profileID)
	}

	groups := configurableResourceCandidateGroups(c)
	var channels []model.Channel
	if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeConfigurable, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, err
	}
	candidates := make([]*model.Channel, 0, len(channels))
	for i := range channels {
		for _, group := range groups {
			if configurableResourceChannelMatches(&channels[i], profileID, group) {
				candidates = append(candidates, &channels[i])
				break
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no available configurable channel for profile %s", profileID)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftPriority := candidates[i].GetPriority()
		rightPriority := candidates[j].GetPriority()
		if leftPriority == rightPriority {
			return candidates[i].Id < candidates[j].Id
		}
		return leftPriority > rightPriority
	})
	return candidates[0], nil
}

func selectConfigurableResourceChannelForEndpoint(c *gin.Context, method, path string) (*model.Channel, *configurable.Profile, *configurable.ResourceConfig, error) {
	if channelIDRaw, ok := common.GetContextKey(c, constant.ContextKeyTokenSpecificChannelId); ok {
		channelID, parseErr := parseSpecificChannelID(channelIDRaw)
		if parseErr != nil {
			return nil, nil, nil, parseErr
		}
		channelModel, err := model.GetChannelById(channelID, true)
		if err != nil {
			return nil, nil, nil, err
		}
		profile, resource, ok := configurableResourceForChannelEndpoint(channelModel, method, path, "")
		if ok {
			return channelModel, profile, resource, nil
		}
		return nil, nil, nil, fmt.Errorf("specific channel %d does not match configurable resource %s %s", channelID, method, path)
	}

	groups := configurableResourceCandidateGroups(c)
	var channels []model.Channel
	if err := model.DB.Where("type = ? AND status = ?", constant.ChannelTypeConfigurable, common.ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, nil, nil, err
	}
	type candidate struct {
		channel  *model.Channel
		profile  *configurable.Profile
		resource *configurable.ResourceConfig
	}
	candidates := make([]candidate, 0, len(channels))
	for i := range channels {
		for _, group := range groups {
			profile, resource, ok := configurableResourceForChannelEndpoint(&channels[i], method, path, group)
			requestModel := configurableResourceRequestModel(c, resource)
			if ok && configurableResourceChannelAbilityEnabled(&channels[i], group, requestModel) {
				candidates = append(candidates, candidate{channel: &channels[i], profile: profile, resource: resource})
				break
			}
		}
	}
	if len(candidates) == 0 {
		return nil, nil, nil, fmt.Errorf("no available configurable resource channel for %s %s", method, path)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		leftPriority := candidates[i].channel.GetPriority()
		rightPriority := candidates[j].channel.GetPriority()
		if leftPriority == rightPriority {
			return candidates[i].channel.Id < candidates[j].channel.Id
		}
		return leftPriority > rightPriority
	})
	selected := candidates[0]
	return selected.channel, selected.profile, selected.resource, nil
}

func configurableResourceForChannelEndpoint(channelModel *model.Channel, method, path, group string) (*configurable.Profile, *configurable.ResourceConfig, bool) {
	if channelModel == nil || channelModel.Type != constant.ChannelTypeConfigurable || channelModel.Status != common.ChannelStatusEnabled {
		return nil, nil, false
	}
	setting := channelModel.GetSetting()
	if setting.Protocol == nil || strings.TrimSpace(setting.Protocol.ProfileID) == "" {
		return nil, nil, false
	}
	if strings.TrimSpace(group) != "" {
		groupMatched := false
		for _, channelGroup := range channelModel.GetGroups() {
			if channelGroup == group {
				groupMatched = true
				break
			}
		}
		if !groupMatched {
			return nil, nil, false
		}
	}
	profile, ok := configurable.GetProfile(setting.Protocol.ProfileID)
	if !ok {
		return nil, nil, false
	}
	resource, ok := profile.ResourceForEndpoint(method, path)
	if !ok {
		return nil, nil, false
	}
	return profile, resource, true
}

func configurableResourceRequestModel(c *gin.Context, resource *configurable.ResourceConfig) string {
	source, err := configurableResourceSource(c)
	if err != nil {
		return ""
	}
	modelName := firstConfigurableResourceModelName(source, "model", "model_name")
	if modelName != "" {
		return modelName
	}
	if query, ok := source["query"].(map[string]any); ok {
		modelName = firstConfigurableResourceModelName(query, "model", "model_name")
	}
	if modelName != "" {
		return modelName
	}
	if resource != nil {
		return strings.TrimSpace(resource.Model)
	}
	return ""
}

func firstConfigurableResourceModelName(source map[string]any, keys ...string) string {
	for _, key := range keys {
		value, _ := source[key].(string)
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func beginConfigurableResourceBilling(c *gin.Context, profile *configurable.Profile, resource *configurable.ResourceConfig, modelName string) (*relaycommon.RelayInfo, *types.NewAPIError) {
	if profile == nil || resource == nil || !resource.Billing.Enabled {
		return nil, nil
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, nil
	}
	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeGenRelayInfoFailed, types.ErrOptionWithStatusCode(http.StatusBadRequest))
	}
	info.OriginModelName = modelName
	info.RequestModelName = modelName
	info.CurrentModelName = modelName
	info.ForcePreConsume = true
	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
	}
	info.PriceData = priceData
	if priceData.FreeModel {
		return info, nil
	}
	if apiErr := service.PreConsumeBilling(c, priceData.Quota, info); apiErr != nil {
		return nil, apiErr
	}
	return info, nil
}

func configurableResourceChannelAbilityEnabled(channelModel *model.Channel, group string, modelName string) bool {
	if channelModel == nil || channelModel.Id <= 0 || strings.TrimSpace(group) == "" {
		return false
	}
	modelName = strings.TrimSpace(modelName)
	if modelName != "" {
		return model.IsChannelEnabledForGroupModel(group, modelName, channelModel.Id)
	}
	var count int64
	if err := model.DB.Model(&model.Ability{}).
		Where(model.CommonGroupCol()+" = ? AND channel_id = ? AND enabled = ?", group, channelModel.Id, true).
		Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}

func configurableResourceCandidateGroups(c *gin.Context) []string {
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	if usingGroup == "" {
		usingGroup = common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	}
	if usingGroup == "auto" {
		userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
		groups := service.GetUserAutoGroup(userGroup)
		if len(groups) > 0 {
			return groups
		}
	}
	if usingGroup == "" {
		return []string{"default"}
	}
	return []string{usingGroup}
}

func configurableResourceChannelMatches(channelModel *model.Channel, profileID, group string) bool {
	if channelModel == nil || channelModel.Type != constant.ChannelTypeConfigurable || channelModel.Status != common.ChannelStatusEnabled {
		return false
	}
	setting := channelModel.GetSetting()
	if setting.Protocol == nil || strings.TrimSpace(setting.Protocol.ProfileID) != profileID {
		return false
	}
	if strings.TrimSpace(group) == "" {
		return true
	}
	for _, channelGroup := range channelModel.GetGroups() {
		if channelGroup == group {
			return true
		}
	}
	return false
}

func parseSpecificChannelID(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case string:
		var id int
		if _, err := fmt.Sscanf(v, "%d", &id); err != nil {
			return 0, err
		}
		return id, nil
	default:
		return 0, fmt.Errorf("invalid specific channel id")
	}
}

func buildConfigurableResourceRequest(c *gin.Context, channelModel *model.Channel, resource *configurable.ResourceConfig) (*http.Request, error) {
	return buildConfigurableResourceRequestWithPreResults(c, channelModel, resource, nil)
}

func buildConfigurableResourceRequestWithPreResults(c *gin.Context, channelModel *model.Channel, resource *configurable.ResourceConfig, preResults map[string]any) (*http.Request, error) {
	if channelModel == nil || resource == nil {
		return nil, fmt.Errorf("invalid configurable resource context")
	}
	method := strings.ToUpper(strings.TrimSpace(resource.Upstream.Method))
	if method == "" {
		method = strings.ToUpper(strings.TrimSpace(resource.Public.Method))
	}
	if method == "" {
		method = http.MethodGet
	}
	upstreamPath := replacePathParams(resource.Upstream.Path, c)
	if upstreamPath == "" {
		upstreamPath = replacePathParams(resource.Public.Path, c)
	}
	requestURL := strings.TrimRight(channelModel.GetBaseURL(), "/") + "/" + strings.TrimLeft(upstreamPath, "/")

	var bodyReader io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		bodyBytes, err := buildConfigurableResourceBodyWithPreResults(c, resource, preResults)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	} else if len(resource.Request.Fields) > 0 {
		query, err := buildConfigurableResourceQueryWithPreResults(c, resource, preResults)
		if err != nil {
			return nil, err
		}
		if encoded := query.Encode(); encoded != "" {
			separator := "?"
			if strings.Contains(requestURL, "?") {
				separator = "&"
			}
			requestURL += separator + encoded
		}
	}
	req, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	configurable.ApplyConfiguredHeaders(req, resource.Upstream.Headers, apiKey, "")
	return req, nil
}

func buildConfigurableResourceBody(c *gin.Context, resource *configurable.ResourceConfig) ([]byte, error) {
	return buildConfigurableResourceBodyWithPreResults(c, resource, nil)
}

func buildConfigurableResourceBodyWithPreResults(c *gin.Context, resource *configurable.ResourceConfig, preResults map[string]any) ([]byte, error) {
	source, err := configurableResourceSource(c)
	if err != nil {
		return nil, err
	}
	if len(preResults) > 0 {
		source["pre"] = preResults
	}
	if len(resource.Request.Fields) == 0 {
		if body, ok := source["body"].(map[string]any); ok {
			return common.Marshal(body)
		}
		return []byte("{}"), nil
	}
	body, err := configurable.BuildMappedMap(resource.Request.Fields, source, nil)
	if err != nil {
		return nil, err
	}
	return common.Marshal(body)
}

func buildConfigurableResourceQuery(c *gin.Context, resource *configurable.ResourceConfig) (url.Values, error) {
	return buildConfigurableResourceQueryWithPreResults(c, resource, nil)
}

func buildConfigurableResourceQueryWithPreResults(c *gin.Context, resource *configurable.ResourceConfig, preResults map[string]any) (url.Values, error) {
	source, err := configurableResourceSource(c)
	if err != nil {
		return nil, err
	}
	if len(preResults) > 0 {
		source["pre"] = preResults
	}
	mapped, err := configurable.BuildMappedMap(resource.Request.Fields, source, nil)
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	for key, value := range mapped {
		if value == nil {
			continue
		}
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				values.Add(key, fmt.Sprint(item))
			}
		case []string:
			for _, item := range v {
				values.Add(key, item)
			}
		default:
			values.Set(key, fmt.Sprint(v))
		}
	}
	return values, nil
}

func executeConfigurableResourcePreRequests(c *gin.Context, client *http.Client, channelModel *model.Channel, resource *configurable.ResourceConfig) (map[string]any, error) {
	if len(resource.PreRequests) == 0 {
		return nil, nil
	}
	results := map[string]any{}
	for _, preRequest := range resource.PreRequests {
		preID := strings.TrimSpace(preRequest.ID)
		if preID == "" {
			return nil, fmt.Errorf("configurable resource pre_request id is required")
		}
		if skipConfigurablePreRequest(c, preRequest, results) {
			continue
		}
		managedResult, _, managedOK, err := resolveManagedConfigurablePreRequest(c, client, channelModel, resource, preRequest)
		if err != nil {
			return nil, err
		}
		if managedOK {
			results[preID] = managedResult
			continue
		}
		req, err := buildConfigurablePreRequest(c, channelModel, resource, preRequest, results)
		if err != nil {
			return nil, err
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf(
			"configurable resource pre_request proxy: method=%s path=%s channel_id=%d resource=%s pre_request=%s upstream=%s",
			c.Request.Method,
			c.Request.URL.Path,
			channelModel.Id,
			resource.ID,
			preID,
			req.URL.Redacted(),
		))
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf(
			"configurable resource pre_request response: method=%s path=%s channel_id=%d resource=%s pre_request=%s upstream_status=%d",
			c.Request.Method,
			c.Request.URL.Path,
			channelModel.Id,
			resource.ID,
			preID,
			resp.StatusCode,
		))
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("configurable resource pre_request %s failed with status %d: %s", preID, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		result, err := configurablePreRequestResult(preRequest, body)
		if err != nil {
			return nil, err
		}
		results[preID] = result
		if preRequest.ManagedState != nil {
			resultMap, _ := result.(map[string]any)
			if err := saveManagedConfigurablePreRequestResult(c, channelModel, resource, preRequest, resultMap); err != nil {
				return nil, err
			}
		}
	}
	return results, nil
}

func resolveManagedConfigurablePreRequest(c *gin.Context, client *http.Client, channelModel *model.Channel, resource *configurable.ResourceConfig, preRequest configurable.PreRequestConfig) (map[string]any, *model.ConfigurableResourceState, bool, error) {
	if preRequest.ManagedState == nil {
		return nil, nil, false, nil
	}
	stateKey := strings.TrimSpace(preRequest.ManagedState.Key)
	if stateKey == "" {
		return nil, nil, false, fmt.Errorf("configurable resource managed_state key is required")
	}
	state, err := model.FindActiveConfigurableResourceState(
		channelModel.Id,
		common.GetContextKeyString(c, middleware.ContextKeyConfigurableResourceProfileID),
		resource.ID,
		preRequest.ID,
		common.GetContextKeyInt(c, constant.ContextKeyUserId),
		common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		stateKey,
	)
	if err != nil || state == nil || strings.TrimSpace(state.StateValue) == "" {
		return nil, &model.ConfigurableResourceState{}, false, nil
	}
	ok, result, err := validateManagedConfigurablePreRequest(c, client, channelModel, resource, preRequest, state.StateValue)
	if err != nil {
		return nil, state, false, err
	}
	if !ok {
		if err := model.MarkConfigurableResourceStateInvalid(state); err != nil {
			return nil, state, false, err
		}
		return nil, state, false, nil
	}
	if len(result) == 0 {
		result = map[string]any{}
	}
	valuePath := strings.TrimSpace(preRequest.ManagedState.ValuePath)
	if valuePath == "" {
		valuePath = "id"
	}
	result[valuePath] = state.StateValue
	state.LastUsedAt = common.GetTimestamp()
	state.UpdatedAt = state.LastUsedAt
	if err := model.UpsertConfigurableResourceState(state); err != nil {
		return nil, state, false, err
	}
	return result, state, true, nil
}

func validateManagedConfigurablePreRequest(c *gin.Context, client *http.Client, channelModel *model.Channel, resource *configurable.ResourceConfig, preRequest configurable.PreRequestConfig, stateValue string) (bool, map[string]any, error) {
	validate := preRequest.ManagedState.Validate
	if strings.TrimSpace(validate.Path) == "" {
		return true, map[string]any{strings.TrimSpace(preRequest.ManagedState.ValuePath): stateValue}, nil
	}
	method := strings.ToUpper(strings.TrimSpace(validate.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := strings.ReplaceAll(validate.Path, "{group_id}", stateValue)
	path = strings.ReplaceAll(path, "{value}", stateValue)
	requestURL := strings.TrimRight(channelModel.GetBaseURL(), "/") + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequest(method, requestURL, nil)
	if err != nil {
		return false, nil, err
	}
	req.Header.Set("Accept", "application/json")
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	configurable.ApplyConfiguredHeaders(req, resource.Upstream.Headers, apiKey, "")
	configurable.ApplyConfiguredHeaders(req, validate.Headers, apiKey, "")
	logger.LogInfo(c.Request.Context(), fmt.Sprintf(
		"configurable resource managed_state validate: method=%s path=%s channel_id=%d resource=%s pre_request=%s upstream=%s",
		c.Request.Method,
		c.Request.URL.Path,
		channelModel.Id,
		resource.ID,
		preRequest.ID,
		req.URL.Redacted(),
	))
	resp, err := client.Do(req)
	if err != nil {
		return false, nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return false, nil, readErr
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return false, nil, nil
	}
	var result map[string]any
	if err := common.Unmarshal(body, &result); err != nil {
		return false, nil, err
	}
	return true, result, nil
}

func saveManagedConfigurablePreRequestResult(c *gin.Context, channelModel *model.Channel, resource *configurable.ResourceConfig, preRequest configurable.PreRequestConfig, result map[string]any) error {
	if preRequest.ManagedState == nil {
		return nil
	}
	stateKey := strings.TrimSpace(preRequest.ManagedState.Key)
	valuePath := strings.TrimSpace(preRequest.ManagedState.ValuePath)
	if valuePath == "" {
		valuePath = "id"
	}
	value, _ := result[valuePath].(string)
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return model.UpsertConfigurableResourceState(&model.ConfigurableResourceState{
		ChannelID:    channelModel.Id,
		ProfileID:    common.GetContextKeyString(c, middleware.ContextKeyConfigurableResourceProfileID),
		ResourceID:   resource.ID,
		PreRequestID: preRequest.ID,
		UserID:       common.GetContextKeyInt(c, constant.ContextKeyUserId),
		TokenID:      common.GetContextKeyInt(c, constant.ContextKeyTokenId),
		StateKey:     stateKey,
		StateValue:   value,
		Status:       model.ConfigurableResourceStateStatusActive,
	})
}

func skipConfigurablePreRequest(c *gin.Context, preRequest configurable.PreRequestConfig, preResults map[string]any) bool {
	path := strings.TrimSpace(preRequest.SkipIfPresent)
	if path == "" {
		return false
	}
	source, err := configurableResourceSource(c)
	if err != nil {
		return false
	}
	if len(preResults) > 0 {
		source["pre"] = preResults
	}
	return !isConfigurableResourceEmptyValue(configurableValueFromSource(path, source))
}

func configurableValueFromSource(path string, source map[string]any) any {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	parts := strings.Split(path, ".")
	var current any = source
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[part]
	}
	return current
}

func isConfigurableResourceEmptyValue(value any) bool {
	if value == nil {
		return true
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return false
}

func buildConfigurablePreRequest(c *gin.Context, channelModel *model.Channel, resource *configurable.ResourceConfig, preRequest configurable.PreRequestConfig, preResults map[string]any) (*http.Request, error) {
	method := strings.ToUpper(strings.TrimSpace(preRequest.Upstream.Method))
	if method == "" {
		method = http.MethodGet
	}
	upstreamPath := replacePathParams(preRequest.Upstream.Path, c)
	requestURL := strings.TrimRight(channelModel.GetBaseURL(), "/") + "/" + strings.TrimLeft(upstreamPath, "/")
	source, err := configurableResourceSource(c)
	if err != nil {
		return nil, err
	}
	if len(preResults) > 0 {
		source["pre"] = preResults
	}
	var bodyReader io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		bodyBytes := []byte("{}")
		if len(preRequest.Request.Fields) > 0 {
			body, err := configurable.BuildMappedMap(preRequest.Request.Fields, source, nil)
			if err != nil {
				return nil, err
			}
			bodyBytes, err = common.Marshal(body)
			if err != nil {
				return nil, err
			}
		}
		bodyReader = bytes.NewReader(bodyBytes)
	} else if len(preRequest.Request.Fields) > 0 {
		mapped, err := configurable.BuildMappedMap(preRequest.Request.Fields, source, nil)
		if err != nil {
			return nil, err
		}
		query := url.Values{}
		for key, value := range mapped {
			if value != nil {
				query.Set(key, fmt.Sprint(value))
			}
		}
		if encoded := query.Encode(); encoded != "" {
			requestURL += "?" + encoded
		}
	}
	req, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	apiKey := common.GetContextKeyString(c, constant.ContextKeyChannelKey)
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	configurable.ApplyConfiguredHeaders(req, resource.Upstream.Headers, apiKey, "")
	configurable.ApplyConfiguredHeaders(req, preRequest.Upstream.Headers, apiKey, "")
	return req, nil
}

func configurablePreRequestResult(preRequest configurable.PreRequestConfig, body []byte) (any, error) {
	if len(preRequest.Response.Fields) == 0 {
		var result map[string]any
		if err := common.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		return result, nil
	}
	result := map[string]any{}
	for _, field := range preRequest.Response.Fields {
		if strings.TrimSpace(field.To) == "" {
			continue
		}
		value := field.Value
		if field.From != "" {
			value = gjson.GetBytes(body, field.From).Value()
		}
		if field.OmitEmpty && value == nil {
			continue
		}
		result[field.To] = value
	}
	return result, nil
}

func configurableResourceSource(c *gin.Context) (map[string]any, error) {
	source := map[string]any{
		"body":  map[string]any{},
		"query": map[string]any{},
		"path":  map[string]any{},
	}
	if c.Request != nil && c.Request.Body != nil && c.Request.ContentLength != 0 {
		var body map[string]any
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return nil, err
		}
		source["body"] = body
		for key, value := range body {
			source[key] = value
		}
	}
	query := make(map[string]any)
	for key, values := range c.Request.URL.Query() {
		if len(values) == 1 {
			query[key] = values[0]
		} else if len(values) > 1 {
			items := make([]any, 0, len(values))
			for _, value := range values {
				items = append(items, value)
			}
			query[key] = items
		}
	}
	source["query"] = query
	pathValues := make(map[string]any)
	for _, param := range c.Params {
		pathValues[param.Key] = param.Value
	}
	source["path"] = pathValues
	return source, nil
}

func replacePathParams(path string, c *gin.Context) string {
	result := path
	for upstreamName, publicName := range configurableResourcePathParams(c) {
		value := c.Param(publicName)
		if strings.TrimSpace(value) == "" {
			continue
		}
		result = strings.ReplaceAll(result, "{"+upstreamName+"}", value)
		result = strings.ReplaceAll(result, ":"+upstreamName, value)
	}
	for _, param := range c.Params {
		result = strings.ReplaceAll(result, "{"+param.Key+"}", param.Value)
		result = strings.ReplaceAll(result, ":"+param.Key, param.Value)
	}
	return result
}

func configurableResourcePathParams(c *gin.Context) map[string]string {
	profileID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceProfileID))
	resourceID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceID))
	profile, ok := configurable.GetProfile(profileID)
	if !ok {
		return nil
	}
	resource, ok := profile.ResourceByID(resourceID)
	if !ok {
		return nil
	}
	return resource.PathParams
}
