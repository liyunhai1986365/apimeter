package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelAssetCredentials(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "test-persistent-secret")
	ch := &Channel{}
	credentials := &AssetCredentials{AccessKeyID: "sensitive-ak", SecretAccessKey: "sensitive-sk"}
	require.NoError(t, ch.SetAssetCredentials(credentials))
	require.NotContains(t, ch.AssetSecret, "sensitive")
	got, err := ch.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, credentials, got)
	raw, err := common.Marshal(ch)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "sensitive")
	require.NotContains(t, string(raw), ch.AssetSecret)
	ch.AssetSecret = "invalid-ciphertext"
	_, err = ch.GetAssetCredentials()
	require.Error(t, err)
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("SESSION_SECRET", "")
	require.ErrorContains(t, ch.SetAssetCredentials(credentials), "persistent")
}

func TestAssetAccountScope(t *testing.T) {
	url := "https://video.example"
	ch := &Channel{Key: "video-key", BaseURL: &url, AssetSecret: "encrypted-credentials"}
	require.Equal(t, "nmyk", ch.AssetHandleProject("nmyk"))
	settings := dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{AssetLibrary: &dto.AssetLibrarySettings{Backend: "tgxmaas", AuthMode: "api_key", BaseURL: "https://assets.example"}}}
	ch.SetSetting(settings)
	first := ch.AssetStateKey("nmyk")
	require.NotEqual(t, first, ch.AssetStateKey("other-project"))
	ch.Key = "another-video-key"
	require.Equal(t, first, ch.AssetStateKey("nmyk"), "dedicated asset account is independent of video key")
	ch.AssetSecret = "new-account-credentials"
	require.NotEqual(t, first, ch.AssetStateKey("nmyk"))
	ch.AssetSecret = "encrypted-credentials"
	settings.Protocol.AssetLibrary.BaseURL = "https://other-assets.example"
	ch.SetSetting(settings)
	require.NotEqual(t, first, ch.AssetStateKey("nmyk"))
}
