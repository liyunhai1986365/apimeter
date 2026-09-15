package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const tgxMaasAssetBodyKey = "tgxmaas_asset_converted_body"
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
	if profile.ID != "seedance-tgxmaas" || (c.GetString(arkAssetActionKey) == "" && strings.HasPrefix(c.Request.URL.Path, "/v1/private-avatar/")) || strings.HasPrefix(c.Request.URL.Path, "/v1/real-avatar/") {
		return nil
	}
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
		"project_name": "ProjectName", "filter": "Filter", "next_token": "NextToken", "max_results": "MaxResults",
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
	if c.Request.Method == http.MethodGet && c.GetString(arkAssetActionKey) == "" {
		for key, values := range c.Request.URL.Query() {
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
	// The supplier only offers the default project and opaque cursor pagination.
	// Reject unsupported semantics instead of silently querying a different scope.
	if raw, ok := body["ProjectName"]; ok {
		var project string
		if err := common.Unmarshal(raw, &project); err != nil || (project != "" && project != "default") {
			return fmt.Errorf("TgxMaas only supports ProjectName=default")
		}
	}
	if _, ok := body["PageNumber"]; ok {
		return fmt.Errorf("use MaxResults/NextToken pagination for TgxMaas")
	}
	if _, ok := body["PageSize"]; ok {
		return fmt.Errorf("use MaxResults/NextToken pagination for TgxMaas")
	}
	if c.GetString(arkAssetActionKey) != "" {
		delete(body, "Id")
	}
	if _, ok := body["model"]; !ok {
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
