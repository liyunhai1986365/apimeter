package configurable

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAssetBackendIndependentProfiles(t *testing.T) {
	for backend, id := range AssetBackendProfiles {
		t.Run(backend, func(t *testing.T) {
			p, ok := AssetProfile(&dto.ChannelProtocolSettings{ProfileID: "generic-video-json", AssetLibrary: &dto.AssetLibrarySettings{Backend: backend}})
			require.True(t, ok)
			require.Equal(t, id, p.ID)
			require.NotEmpty(t, p.Resources)
			for _, resource := range p.Resources {
				require.True(t, resource.AssetLibrary, "%s must use asset routing and credentials", resource.ID)
			}
			original, _ := GetProfile(id)
			// Editing the derived resource slice must not mutate global embedded profiles.
			p.Resources[0].Name = "modified derived profile"
			require.NotEqual(t, p.Resources[0].Name, original.Resources[0].Name)
		})
	}
	official, ok := AssetProfile(&dto.ChannelProtocolSettings{AssetLibrary: &dto.AssetLibrarySettings{Backend: OfficialAssetBackend}})
	require.True(t, ok)
	require.Len(t, official.Resources, 12)
	proxy, ok := AssetProfile(&dto.ChannelProtocolSettings{AssetLibrary: &dto.AssetLibrarySettings{Backend: YouniyoujuAssetBackend}})
	require.True(t, ok)
	require.Len(t, proxy.Resources, 12)
	for _, resource := range proxy.Resources {
		require.True(t, resource.AssetLibrary)
		require.True(t, resource.DisableReplay, "asset handles and liveness tokens must not be replayed on another account")
	}
	legacy, _ := GetProfile("seedance-tgxmaas")
	require.Equal(t, "/v1/private-avatar/groups", legacy.Resources[0].Upstream.Path)
}
