package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/gin-gonic/gin"
)

// Hanxingtu uses Ark field names with POST /v1/asset/<Action>, Bearer auth
// and bare result objects. Do not forward video routing or project metadata.
func prepareHanxingtuAssetRequest(c *gin.Context, resource *configurable.ResourceConfig) error {
	input, err := assetRequestInput(c)
	if err != nil {
		return err
	}
	project, err := assetRequestProject(c.Request.URL.Query(), input)
	if err != nil {
		return err
	}
	if project != "" && project != "default" {
		return fmt.Errorf("Hanxingtu assets use the default project")
	}
	fields := map[string][]string{
		"asset_groups_create": {"Name", "Description", "GroupType"},
		"asset_groups_list":   {"Filter"}, "asset_groups_delete": {"Id"},
		"assets_create": {"GroupId", "URL", "AssetType", "Name"},
		"assets_list":   {"Filter"}, "assets_get": {"Id"}, "assets_delete": {"Id"},
	}[resource.ID]
	aliases := map[string][]string{
		"Name": {"name"}, "Description": {"description"}, "GroupType": {"group_type"},
		"GroupId": {"group_id"}, "URL": {"url"}, "AssetType": {"asset_type"}, "Filter": {"filter"},
		"Id": {"id", "asset_id", "AssetId", "group_id", "GroupId"},
	}
	body := map[string]json.RawMessage{}
	for _, field := range fields {
		for _, name := range append([]string{field}, aliases[field]...) {
			var value json.RawMessage
			if v := input.Get(name); v.Exists() {
				value = json.RawMessage(v.Raw)
			}
			if values, ok := c.Request.URL.Query()[name]; ok && c.Request.Method == http.MethodGet {
				if len(values) != 1 || value != nil {
					return fmt.Errorf("%s must be supplied once", field)
				}
				if field == "Filter" {
					value = json.RawMessage(values[0])
				} else {
					value, err = common.Marshal(values[0])
					if err != nil {
						return err
					}
				}
			}
			if value == nil {
				continue
			}
			if previous, ok := body[field]; ok && string(previous) != string(value) {
				return fmt.Errorf("conflicting %s fields", field)
			}
			body[field] = value
		}
	}
	if resource.ID == "assets_get" || strings.HasSuffix(resource.ID, "_delete") {
		for _, param := range c.Params {
			value, err := common.Marshal(param.Value)
			if err != nil {
				return err
			}
			if previous, ok := body["Id"]; ok && string(previous) != string(value) {
				return fmt.Errorf("conflicting resource IDs")
			}
			body["Id"] = value
		}
	}
	if raw, ok := body["GroupType"]; ok {
		var groupType string
		if common.Unmarshal(raw, &groupType) != nil || groupType != "AIGC" {
			return fmt.Errorf("Hanxingtu only supports AIGC asset groups")
		}
	}
	if resource.ID == "assets_list" {
		// Match the exact field inspected by the ownership layer. Struct
		// decoding would also accept casing variants it did not authorize.
		var filter map[string]json.RawMessage
		var groupIDs []string
		if common.Unmarshal(body["Filter"], &filter) != nil || common.Unmarshal(filter["GroupIds"], &groupIDs) != nil || len(groupIDs) != 1 || !validAssetHandle(groupIDs[0]) {
			return fmt.Errorf("Filter.GroupIds must contain exactly one asset group ID")
		}
	}
	if raw, ok := body["AssetType"]; ok {
		var assetType string
		if common.Unmarshal(raw, &assetType) != nil || (assetType != "Image" && assetType != "Video" && assetType != "Audio") {
			return fmt.Errorf("AssetType must be Image, Video or Audio")
		}
	}
	raw, err := common.Marshal(body)
	if err != nil {
		return err
	}
	c.Set(tgxMaasAssetBodyKey, raw)
	return nil
}
