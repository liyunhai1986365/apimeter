package dto

import (
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGeminiEstimateIncludesSystemAndToolHistory(t *testing.T) {
	var req GeminiChatRequest
	require.NoError(t, kitutil.UnmarshalJsonStr(`{"systemInstruction":{"parts":[{"text":"system rules"}]},"generationConfig":{"mediaResolution":"MEDIA_RESOLUTION_LOW"},"tools":[{"functionDeclarations":[{"name":"tool","parameters":{"type":"OBJECT"}}]}],"contents":[{"role":"model","parts":[{"functionCall":{"name":"tool","args":{"q":"search"}}}]},{"role":"user","parts":[{"functionResponse":{"name":"tool","response":{"output":"plain text"},"parts":[{"inlineData":{"mimeType":"image/png","data":"AA=="},"mediaResolution":{"level":"MEDIA_RESOLUTION_HIGH"}}]}},{"inlineData":{"mimeType":"image/png","data":"AA=="}}]}]}`, &req))
	meta := req.GetTokenCountMeta()
	for _, expected := range []string{"system rules", "tool", "search", "plain text", "functionDeclarations"} {
		require.Contains(t, meta.CombineText, expected)
	}
	require.NotContains(t, meta.CombineText, `"plain text"`)
	require.Len(t, meta.Files, 2)
	require.Equal(t, "MEDIA_RESOLUTION_HIGH", meta.Files[0].Detail)
	require.Equal(t, "MEDIA_RESOLUTION_LOW", meta.Files[1].Detail)
}
