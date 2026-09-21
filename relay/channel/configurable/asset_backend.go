package configurable

import "github.com/QuantumNous/new-api/relaykit/dto"

const OfficialAssetBackend = "volcengine-assets"
const YouniyoujuAssetBackend = "youniyouju"
const YouniyoujuAssetPath = "/api/volcengine_asset"

// Action backends carry resource IDs in JSON and preserve provider IDs directly.
// Authentication remains a separate, backend-specific setting.
func IsAssetActionBackend(backend string) bool {
	return backend == OfficialAssetBackend || backend == YouniyoujuAssetBackend
}

// Asset backends reuse existing resource contracts without changing video routes.
var AssetBackendProfiles = map[string]string{
	"tgxmaas":               "seedance-tgxmaas",
	"task":                  "seedance2-ark-task-assets",
	"modelsell":             "seedance2-modelsell",
	"api-assets":            "doubao-seedance-2-api-assets",
	"material":              "doubao-seedance-2",
	"service-inference":     "seedance2-service-inference",
	"max-service-inference": "doubao-seedance-max-service-inference",
}

var OfficialAssetActions = map[string]string{
	"asset_groups_create": "CreateAssetGroup", "asset_groups_list": "ListAssetGroups", "asset_groups_get": "GetAssetGroup", "asset_groups_update": "UpdateAssetGroup", "asset_groups_delete": "DeleteAssetGroup",
	"assets_create": "CreateAsset", "assets_list": "ListAssets", "assets_get": "GetAsset", "assets_update": "UpdateAsset", "assets_delete": "DeleteAsset",
	"liveness_session_create": "CreateVisualValidateSession", "liveness_group_exchange": "GetVisualValidateResult",
}

func AssetProfile(protocol *dto.ChannelProtocolSettings) (*Profile, bool) {
	if protocol == nil {
		return nil, false
	}
	cfg := protocol.AssetLibrary
	if cfg == nil || cfg.Backend == "" || cfg.Backend == "inherit" {
		return GetProfile(protocol.ProfileID)
	}
	if cfg.Backend == "disabled" {
		return nil, false
	}
	if IsAssetActionBackend(cfg.Backend) {
		p, ok := GetProfile("seedance-tgxmaas")
		if !ok {
			return nil, false
		}
		p.ID = cfg.Backend
		path := "/"
		if cfg.Backend == YouniyoujuAssetBackend {
			path = YouniyoujuAssetPath
		}
		p.Resources = append([]ResourceConfig(nil), p.Resources...)
		for i := range p.Resources {
			r := &p.Resources[i]
			r.Upstream = EndpointConfig{Method: "POST", Path: path + "?Action=" + OfficialAssetActions[r.ID] + "&Version=2024-01-01"}
			r.DisableReplay = true
		}
		return p, true
	}
	id, ok := AssetBackendProfiles[cfg.Backend]
	if !ok {
		return nil, false
	}
	p, ok := GetProfile(id)
	if !ok {
		return nil, false
	}
	p.Resources = append([]ResourceConfig(nil), p.Resources...)
	for i := range p.Resources {
		p.Resources[i].DisableReplay = true
	}
	return p, true
}
