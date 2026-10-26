package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeTokenCountExtractsToolResultImages(t *testing.T) {
	var request ClaudeRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{
		"model":"claude-test","max_tokens":4096,
		"messages":[{"role":"user","content":[
			{"type":"image","source":{"type":"base64","media_type":"image/png","data":"ZGlyZWN0"}},
			{"type":"tool_result","tool_use_id":"call","content":[
				{"type":"text","text":"Screenshot result"},
				{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"dG9vbC1pbWFnZQ=="}},
				{"type":"image","source":{"type":"url","url":"https://example.test/image.png"}},
				{"type":"custom","custom_data":"keep unknown result fields"}
			]}
		]}]
	}`), &request))
	before, err := kitutil.Marshal(request)
	require.NoError(t, err)
	meta := request.GetTokenCountMeta()
	require.Equal(t, 4096, meta.MaxTokens)
	require.Equal(t, 1, meta.MessagesCount)
	require.Len(t, meta.Files, 3)
	for i, data := range []string{"ZGlyZWN0", "dG9vbC1pbWFnZQ==", "https://example.test/image.png"} {
		require.Equal(t, types.FileTypeImage, meta.Files[i].FileType)
		require.Equal(t, data, meta.Files[i].Source.GetRawData())
		require.NotContains(t, meta.CombineText, data)
	}
	require.Contains(t, meta.CombineText, "Screenshot result")
	require.Contains(t, meta.CombineText, "keep unknown result fields")
	after, err := kitutil.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after, "token counting must not change the request sent upstream")
}

func TestClaudeTokenCountPreservesTextOnlyToolResults(t *testing.T) {
	for _, content := range []any{
		"plain result",
		"",
		[]ClaudeMediaMessage{{Type: "text", Text: kitutil.GetPointer("text block")}},
		map[string]any{"custom": "structured result"},
	} {
		request := ClaudeRequest{Messages: []ClaudeMessage{{Role: "user", Content: []ClaudeMediaMessage{{Type: "tool_result", Content: content}}}}}
		meta := request.GetTokenCountMeta()
		normalizedContent, err := kitutil.Any2Type[any](content)
		require.NoError(t, err)
		encoded, err := kitutil.Marshal(normalizedContent)
		require.NoError(t, err)
		require.Equal(t, "user\n"+string(encoded), meta.CombineText)
		require.Empty(t, meta.Files)
	}
}

func TestClaudeTokenCountImageOnlyToolResult(t *testing.T) {
	request := ClaudeRequest{Messages: []ClaudeMessage{{Role: "user", Content: []ClaudeMediaMessage{{
		Type: "tool_result", Content: []ClaudeMediaMessage{{
			Type: "image", Source: &ClaudeMessageSource{Type: "base64", MediaType: "image/png", Data: "aW1hZ2U="},
		}},
	}}}}}
	meta := request.GetTokenCountMeta()
	require.Equal(t, "user", meta.CombineText)
	require.Len(t, meta.Files, 1)
	require.Equal(t, "aW1hZ2U=", meta.Files[0].Source.GetRawData())
}
