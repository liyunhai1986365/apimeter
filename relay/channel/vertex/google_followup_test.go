package vertex

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGoogleVertexRegionsAndVersion(t *testing.T) {
	for _, region := range []string{"us", "eu"} {
		info := &relaycommon.RelayInfo{OriginModelName: "gemini-3-pro-preview", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3-pro-preview", ApiKey: "test", ApiVersion: `{"default":"` + region + `"}`, ChannelOtherSettings: dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey, VertexProjectID: "project", VertexAPIVersion: "v1beta1"}}}
		a := &Adaptor{}
		a.Init(info)
		url, err := a.GetRequestURL(info)
		require.NoError(t, err)
		require.Equal(t, "https://aiplatform."+region+".rep.googleapis.com/v1beta1/projects/project/locations/"+region+"/publishers/google/models/gemini-3-pro-preview:generateContent?key=test", url)
	}
}
func TestGoogleVertexStripsBothFunctionIdentities(t *testing.T) {
	req := &dto.GeminiChatRequest{Contents: []dto.GeminiChatContent{{Parts: []dto.GeminiPart{{FunctionCall: &dto.FunctionCall{ID: "id", FunctionName: "run"}}, {FunctionResponse: &dto.GeminiFunctionResponse{ID: []byte(`"id"`), Name: "run"}}}}}}
	req.Requests = []dto.GeminiChatRequest{*req}
	removeFunctionCallIDs(req)
	require.Empty(t, req.Contents[0].Parts[0].FunctionCall.ID)
	require.Empty(t, req.Contents[0].Parts[1].FunctionResponse.ID)
	require.Empty(t, req.Requests[0].Contents[0].Parts[0].FunctionCall.ID)
}
