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
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func RelayConfigurableResource(c *gin.Context) {
	profileID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceProfileID))
	resourceID := strings.TrimSpace(c.GetString(middleware.ContextKeyConfigurableResourceID))

	channelModel, profile, resource, err := selectConfigurableResourceRoute(c, profileID, resourceID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.Set(middleware.ContextKeyConfigurableResourceProfileID, profile.ID)
	c.Set(middleware.ContextKeyConfigurableResourceID, resource.ID)
	if apiErr := middleware.SetupContextForSelectedChannel(c, channelModel, ""); apiErr != nil {
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
		return
	}

	upstreamReq, err := buildConfigurableResourceRequest(c, channelModel, resource)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	client, err := service.GetHttpClientWithProxy(channelModel.GetSetting().Proxy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
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
			if ok {
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
	url := strings.TrimRight(channelModel.GetBaseURL(), "/") + "/" + strings.TrimLeft(upstreamPath, "/")

	var bodyReader io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		bodyBytes, err := buildConfigurableResourceBody(c, resource)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(bodyBytes)
	} else if len(resource.Request.Fields) > 0 {
		query, err := buildConfigurableResourceQuery(c, resource)
		if err != nil {
			return nil, err
		}
		if encoded := query.Encode(); encoded != "" {
			separator := "?"
			if strings.Contains(url, "?") {
				separator = "&"
			}
			url += separator + encoded
		}
	}
	req, err := http.NewRequest(method, url, bodyReader)
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
	source, err := configurableResourceSource(c)
	if err != nil {
		return nil, err
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
	source, err := configurableResourceSource(c)
	if err != nil {
		return nil, err
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
