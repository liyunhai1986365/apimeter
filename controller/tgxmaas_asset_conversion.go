package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const tgxMaasAssetBodyKey = "tgxmaas_asset_converted_body"
const tgxMaasAssetQueryKey = "tgxmaas_asset_converted_query"
const arkAssetActionKey = "ark_asset_action"

// RelayArkAssetAction accepts the Ark action/body protocol with gateway Bearer
// authentication. IDs remain provider handles, including in subsequent calls.
func RelayArkAssetAction(c *gin.Context) {
	actions := map[string]struct{ method, path, param string }{
		"CreateAsset":      {"POST", "/v1/private-avatar/assets", ""},
		"ListAssets":       {"POST", "/v1/private-avatar/assets/list", ""},
		"GetAsset":         {"GET", "/v1/private-avatar/assets/", "asset_id"},
		"UpdateAsset":      {"PATCH", "/v1/private-avatar/assets/", "asset_id"},
		"DeleteAsset":      {"DELETE", "/v1/private-avatar/assets/", "asset_id"},
		"CreateAssetGroup": {"POST", "/v1/private-avatar/groups", ""},
		"ListAssetGroups":  {"POST", "/v1/private-avatar/groups/list", ""},
		"GetAssetGroup":    {"GET", "/v1/private-avatar/groups/", "group_id"},
		"UpdateAssetGroup": {"PATCH", "/v1/private-avatar/groups/", "group_id"},
		"DeleteAssetGroup": {"DELETE", "/v1/private-avatar/groups/", "group_id"},
	}
	action := c.Query("Action")
	target, ok := actions[action]
	if !ok || c.Query("Version") != "2024-01-01" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported asset Action or Version (expected 2024-01-01)"})
		return
	}
	var body map[string]json.RawMessage
	if err := common.UnmarshalBodyReusable(c, &body); err != nil || body == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "asset body must be a JSON object"})
		return
	}
	path := target.path
	originalParams := c.Params
	if target.param != "" {
		var id string
		if err := common.Unmarshal(body["Id"], &id); err != nil || !validAssetHandle(id) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Id must be a non-empty resource handle without path separators"})
			return
		}
		path += id
		c.Params = gin.Params{{Key: target.param, Value: id}}
	}
	originalRequest := c.Request
	request := c.Request.Clone(c.Request.Context())
	request.Method = target.method
	request.URL.Path = path
	request.URL.RawPath = ""
	c.Request = request
	c.Set(arkAssetActionKey, action)
	defer func() { c.Request = originalRequest; c.Params = originalParams }()
	RelayConfigurableResource(c)
}

func validAssetHandle(id string) bool {
	return strings.TrimSpace(id) != "" && id != "." && id != ".." && !strings.ContainsAny(id, "/\\?#%\r\n")
}

// Conversion is scoped to the selected TgxMaas profile, so shared generic
// routes retain the contracts of other providers.
func prepareTgxMaasAssetRequest(c *gin.Context, ch *model.Channel, profile *configurable.Profile, resource *configurable.ResourceConfig) error {
	if profile.ID != "seedance-tgxmaas" || strings.HasPrefix(c.Request.URL.Path, "/v1/real-avatar/") {
		return nil
	}
	native := c.GetString(arkAssetActionKey) == "" && strings.HasPrefix(c.Request.URL.Path, "/v1/private-avatar/")
	for _, param := range c.Params {
		if !validAssetHandle(param.Value) {
			return fmt.Errorf("invalid resource ID")
		}
	}
	body := map[string]json.RawMessage{}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := common.UnmarshalBodyReusable(c, &body); err != nil {
			return err
		}
		if body == nil {
			return fmt.Errorf("asset body must be a JSON object")
		}
	}
	aliases := map[string]string{
		"name": "Name", "description": "Description", "url": "URL", "asset_type": "AssetType", "group_id": "GroupId",
		"group_type": "GroupType", "project_name": "ProjectName", "filter": "Filter", "next_token": "NextToken", "max_results": "MaxResults",
		"page_number": "PageNumber", "page_size": "PageSize", "page": "PageNumber",
	}
	for from, to := range aliases {
		if value, ok := body[from]; ok {
			if _, exists := body[to]; exists {
				return fmt.Errorf("both %s and %s were supplied", from, to)
			}
			body[to] = value
			delete(body, from)
		}
	}
	if c.GetString(arkAssetActionKey) == "" {
		for key, values := range c.Request.URL.Query() {
			if c.Request.Method != http.MethodGet && key != "ProjectName" && key != "project_name" {
				continue
			}
			if len(values) != 1 {
				return fmt.Errorf("query parameter %s must occur once", key)
			}
			name := key
			if mapped, ok := aliases[key]; ok {
				name = mapped
			}
			if _, exists := body[name]; exists {
				return fmt.Errorf("%s was supplied in both body and query", name)
			}
			switch name {
			case "Filter", "MaxResults", "PageNumber", "PageSize":
				if !gjson.ValidBytes([]byte(values[0])) {
					return fmt.Errorf("%s must contain valid JSON", key)
				}
				body[name] = json.RawMessage(values[0])
			default:
				raw, err := common.Marshal(values[0])
				if err != nil {
					return err
				}
				body[name] = raw
			}
		}
	}
	if c.Request.URL.Path == "/v1/assets/get" {
		id := ""
		for _, name := range []string{"asset_id", "Id", "id"} {
			if raw, exists := body[name]; exists {
				var value string
				if err := common.Unmarshal(raw, &value); err != nil || !validAssetHandle(value) {
					return fmt.Errorf("%s must be a valid asset ID", name)
				}
				if id != "" && id != value {
					return fmt.Errorf("conflicting asset IDs")
				}
				id = value
			}
		}
		if id == "" {
			return fmt.Errorf("asset_id is required")
		}
		// The public endpoint carries the handle in JSON/query rather than the
		// path. Reuse the profile's asset_id -> id mapping for the upstream GET.
		c.Params = gin.Params{{Key: "id", Value: id}}
	}
	// Preserve project scope for every operation, including GET requests whose
	// upstream transport has no JSON body. Do not default it here: asset creation
	// inherits the group's project when the client omits ProjectName.
	query := url.Values{}
	if raw, ok := body["ProjectName"]; ok {
		var project string
		if strings.TrimSpace(string(raw)) == "null" || common.Unmarshal(raw, &project) != nil {
			return fmt.Errorf("ProjectName must be a string")
		}
		query.Set("ProjectName", project)
	}
	c.Set(tgxMaasAssetQueryKey, query)
	if resource.ID == "assets_list" || resource.ID == "asset_groups_list" {
		_, pageNumber := body["PageNumber"]
		_, pageSize := body["PageSize"]
		_, nextToken := body["NextToken"]
		_, maxResults := body["MaxResults"]
		if (pageNumber || pageSize) && (nextToken || maxResults) {
			return fmt.Errorf("PageNumber/PageSize and MaxResults/NextToken pagination cannot be combined")
		}
		for _, field := range []string{"PageNumber", "PageSize", "MaxResults"} {
			if raw, ok := body[field]; ok {
				var value int
				if common.Unmarshal(raw, &value) != nil || value <= 0 {
					return fmt.Errorf("%s must be a positive integer", field)
				}
			}
		}
	}
	project := query.Get("ProjectName")
	resolve := func(id string) (string, error) {
		return model.ResolveTgxMaasAssetHandle(ch.Id, common.GetContextKeyInt(c, constant.ContextKeyUserId), project, id)
	}
	for i := range c.Params {
		id, err := resolve(c.Params[i].Value)
		if err != nil {
			return fmt.Errorf("resolve asset handle: %w", err)
		}
		c.Params[i].Value = id
	}
	if raw, ok := body["GroupId"]; ok {
		var id string
		if common.Unmarshal(raw, &id) != nil {
			return fmt.Errorf("GroupId must be a string")
		}
		id, err := resolve(id)
		if err != nil {
			return err
		}
		body["GroupId"], _ = common.Marshal(id)
	}
	if raw, ok := body["Filter"]; ok {
		var filter map[string]json.RawMessage
		if common.Unmarshal(raw, &filter) != nil || filter == nil {
			return fmt.Errorf("Filter must be an object")
		}
		for _, field := range []string{"GroupIds", "Ids"} {
			if value, ok := filter[field]; ok {
				var ids []string
				if common.Unmarshal(value, &ids) != nil {
					return fmt.Errorf("Filter.%s must be an array of IDs", field)
				}
				for i := range ids {
					id, err := resolve(ids[i])
					if err != nil {
						return err
					}
					ids[i] = id
				}
				filter[field], _ = common.Marshal(ids)
			}
		}
		body["Filter"], _ = common.Marshal(filter)
	}
	if c.GetString(arkAssetActionKey) != "" {
		delete(body, "Id")
	}
	if _, ok := body["model"]; !ok && !native {
		name := configurableResourceRequestModel(c, resource)
		if name == "" {
			for _, candidate := range strings.Split(ch.Models, ",") {
				candidate = strings.TrimSpace(candidate)
				for _, group := range configurableResourceCandidateGroups(c) {
					if candidate != "" && configurableResourceChannelAbilityEnabled(ch, group, candidate) {
						name = candidate
						break
					}
				}
				if name != "" {
					break
				}
			}
		}
		if name != "" {
			raw, err := common.Marshal(name)
			if err != nil {
				return err
			}
			body["model"] = raw
		}
	}
	raw, err := common.Marshal(body)
	if err != nil {
		return err
	}
	c.Set(tgxMaasAssetBodyKey, raw)
	return nil
}

func rememberTgxMaasAssetHandles(c *gin.Context, channelID int, resourceID string, response []byte) error {
	if gjson.GetBytes(response, "ResponseMetadata.Error").Exists() {
		return nil
	}
	project := ""
	if raw, ok := c.Get(tgxMaasAssetBodyKey); ok {
		project = gjson.GetBytes(raw.([]byte), "ProjectName").String()
	}
	remember := func(item gjson.Result) error {
		original := item.Get("upstream_asset_id").String()
		if strings.HasPrefix(resourceID, "asset_groups_") {
			original = item.Get("upstream_group_id").String()
		}
		if original == "" {
			return nil
		}
		if !validAssetHandle(original) || !validAssetHandle(item.Get("Id").String()) {
			return fmt.Errorf("invalid upstream asset handle")
		}
		scope := item.Get("ProjectName").String()
		if scope == "" {
			scope = project
		}
		return model.SaveTgxMaasAssetHandle(channelID, common.GetContextKeyInt(c, constant.ContextKeyUserId), scope, original, item.Get("Id").String())
	}
	result := gjson.GetBytes(response, "Result")
	if err := remember(result); err != nil {
		return err
	}
	for _, item := range result.Get("Items").Array() {
		if err := remember(item); err != nil {
			return err
		}
	}
	return nil
}
