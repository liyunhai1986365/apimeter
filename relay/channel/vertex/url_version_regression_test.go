package vertex

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
)

func TestVertexConfiguredVersionReplacesBaseURLVersion(t *testing.T) {
	for _, apiKey := range []bool{true, false} {
		for _, tc := range []struct{ base, version, path string }{
			{"https://proxy.test/v1", "v1beta1", "/v1beta1"},
			{"https://proxy.test/google/v1/", "v1beta1", "/google/v1beta1"},
			{"https://proxy.test/google/v1beta1", "v1", "/google/v1"},
			{"https://proxy.test/google/v1beta1/", "v1beta1", "/google/v1beta1"},
			{"https://proxy.test/google", "", "/google/v1"},
		} {
			info := &relaycommon.RelayInfo{OriginModelName: "gemini-3-pro-preview", ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "gemini-3-pro-preview", ChannelBaseUrl: tc.base, ApiVersion: `{"default":"global"}`,
				ApiKey: `{"project_id":"project"}`, ChannelOtherSettings: dto.ChannelOtherSettings{VertexProjectID: "project", VertexAPIVersion: tc.version},
			}}
			if apiKey {
				info.ApiKey = "test"
				info.ChannelOtherSettings.VertexKeyType = dto.VertexKeyTypeAPIKey
			}
			a := &Adaptor{}
			a.Init(info)
			got, err := a.GetRequestURL(info)
			require.NoError(t, err)
			want := "https://proxy.test" + tc.path + "/projects/project/locations/global/publishers/google/models/gemini-3-pro-preview:generateContent"
			if apiKey {
				want += "?key=test"
			}
			require.Equal(t, want, got)
		}
	}
	// The shared builder must keep sibling provider endpoints valid.
	require.Equal(t, "https://proxy.test/google/v1/projects/p/locations/us/publishers/anthropic/models/claude:rawPredict", BuildAnthropicModelURL("https://proxy.test/google/v1beta1", "v1", "p", "us", "claude", "rawPredict"))
	require.Equal(t, "https://proxy.test/google/v1beta1/projects/p/locations/us/endpoints/openapi/chat/completions", BuildOpenSourceChatCompletionsURL("https://proxy.test/google/v1", "p", "us"))
}
