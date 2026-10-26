package configurable

import (
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestArkCompatibleProfileAndValidation(t *testing.T) {
	cfg := &dto.AssetLibrarySettings{Backend: ArkCompatibleAssetBackend, ArkCompatible: &dto.ArkCompatibleAssetSettings{FixedQuery: map[string]string{"ns": "asset_manager_intl", "label": "a&b=中文 +"}}}
	p, ok := AssetProfile(&dto.ChannelProtocolSettings{ProfileID: "generic-video-json", AssetLibrary: cfg})
	require.True(t, ok)
	require.Len(t, p.Resources, 10)
	for _, resource := range p.Resources {
		require.True(t, resource.AssetLibrary)
		require.True(t, resource.DisableReplay)
		u, err := url.Parse(resource.Upstream.Path)
		require.NoError(t, err)
		require.Equal(t, "/api", u.Path)
		require.Equal(t, "a&b=中文 +", u.Query().Get("label"))
		require.Equal(t, OfficialAssetActions[resource.ID], u.Query().Get("Action"))
	}
	_, ok = p.ResourceByID("liveness_session_create")
	require.False(t, ok)
	original, _ := GetProfile("seedance-tgxmaas")
	p.Resources[0].Upstream.Path = "/mutated"
	require.Equal(t, "/v1/private-avatar/groups", original.Resources[0].Upstream.Path)
	for _, invalid := range []dto.ArkCompatibleAssetSettings{
		{EndpointPath: "https://example.com/api"}, {EndpointPath: "//example.com/api"},
		{EndpointPath: "/api?Action=DeleteAsset"}, {EndpointPath: "/api#fragment"},
		{EndpointPath: "/api/../other"}, {EndpointPath: "/api/%2fother"}, {EndpointPath: "/api\\other"},
		{APIVersion: "2024-01-01&Action=DeleteAsset"}, {Pagination: "auto"},
		{FixedQuery: map[string]string{"aCtion": "DeleteAsset"}},
		{FixedQuery: map[string]string{"Version": "2024-01-01"}},
		{FixedQuery: map[string]string{"ProjectName": "project"}},
		{FixedQuery: map[string]string{"api_key": "secret"}},
		{FixedQuery: map[string]string{"ns": "a\nb"}},
	} {
		cfg.ArkCompatible = &invalid
		require.Error(t, ValidateArkCompatibleAssetConfig(cfg), "%+v", invalid)
		_, ok := AssetProfile(&dto.ChannelProtocolSettings{AssetLibrary: cfg})
		require.False(t, ok)
	}
}
