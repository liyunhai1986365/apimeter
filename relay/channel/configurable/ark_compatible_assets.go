package configurable

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
)

var arkAssetPathPattern = regexp.MustCompile(`^/[A-Za-z0-9/_-]*$`)
var arkAssetQueryKeyPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
var arkAssetVersionPattern = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

func ArkCompatibleAssetConfig(cfg *dto.AssetLibrarySettings) dto.ArkCompatibleAssetSettings {
	var result dto.ArkCompatibleAssetSettings
	if cfg != nil && cfg.ArkCompatible != nil {
		result = *cfg.ArkCompatible
	}
	if result.EndpointPath == "" {
		result.EndpointPath = "/api"
	}
	if result.APIVersion == "" {
		result.APIVersion = "2024-01-01"
	}
	if result.Pagination == "" {
		result.Pagination = "page"
	}
	return result
}

func ValidateArkCompatibleAssetConfig(cfg *dto.AssetLibrarySettings) error {
	c := ArkCompatibleAssetConfig(cfg)
	if len(c.EndpointPath) > 512 || !arkAssetPathPattern.MatchString(c.EndpointPath) || strings.Contains(c.EndpointPath, "//") {
		return fmt.Errorf("asset endpoint path must start with / and contain only letters, digits, /, _ or -")
	}
	if !arkAssetVersionPattern.MatchString(c.APIVersion) {
		return fmt.Errorf("asset API version must use YYYY-MM-DD")
	}
	if c.Pagination != "page" && c.Pagination != "cursor" {
		return fmt.Errorf("asset pagination must be page or cursor")
	}
	if len(c.FixedQuery) > 32 {
		return fmt.Errorf("asset fixed query supports at most 32 parameters")
	}
	for key, value := range c.FixedQuery {
		if len(key) > 64 || !arkAssetQueryKeyPattern.MatchString(key) || len(value) > 2048 || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("invalid asset fixed query parameter")
		}
		switch strings.ToLower(key) {
		case "action", "version", "projectname", "project_name", "authorization", "key", "api_key", "apikey", "token", "access_token":
			return fmt.Errorf("asset fixed query cannot contain reserved parameter %s", key)
		}
	}
	return nil
}

// This canonical endpoint also forms the ownership scope. Changing namespace
// or path must never reuse bindings or cursors from a different library.
func ArkCompatibleAssetEndpoint(cfg *dto.AssetLibrarySettings, action string) string {
	c := ArkCompatibleAssetConfig(cfg)
	query := url.Values{}
	for key, value := range c.FixedQuery {
		query.Set(key, value)
	}
	query.Set("Version", c.APIVersion)
	if action != "" {
		query.Set("Action", action)
	}
	return c.EndpointPath + "?" + query.Encode()
}

func arkCompatibleAssetProfile(cfg *dto.AssetLibrarySettings) (*Profile, bool) {
	if ValidateArkCompatibleAssetConfig(cfg) != nil {
		return nil, false
	}
	p, ok := GetProfile("seedance-tgxmaas")
	if !ok {
		return nil, false
	}
	p.ID = ArkCompatibleAssetBackend
	resources := make([]ResourceConfig, 0, 10)
	for _, resource := range p.Resources {
		if strings.HasPrefix(resource.ID, "liveness_") {
			continue
		}
		action, ok := OfficialAssetActions[resource.ID]
		if !ok {
			continue
		}
		resource.Upstream = EndpointConfig{Method: "POST", Path: ArkCompatibleAssetEndpoint(cfg, action)}
		resource.DisableReplay = true
		resources = append(resources, resource)
	}
	p.Resources = resources
	return p, true
}
