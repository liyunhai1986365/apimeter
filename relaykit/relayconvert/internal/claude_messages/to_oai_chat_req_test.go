package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesToChatPreservesToolImagesAndAssistantText(t *testing.T) {
	const body = `{
		"model":"gpt-6.1-sol","max_tokens":1024,"stream":true,
		"messages":[
			{"role":"user","content":"Compare the screenshots."},
			{"role":"assistant","content":[
				{"type":"text","text":"I will inspect both results."},
				{"type":"tool_use","id":"call_a","name":"view","input":{"path":"a.png"}},
				{"type":"tool_use","id":"call_b","name":"view","input":{"path":"b.png"}}
			]},
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"call_a","content":[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1hZ2UtYQ=="}}
				]},
				{"type":"tool_result","tool_use_id":"call_b","content":[
					{"type":"text","text":"Two views of the second result."},
					{"type":"image","source":{"type":"url","url":"https://example.test/b.png"}},
					{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"aW1hZ2UtYw=="}}
				]},
				{"type":"text","text":"Explain the differences."}
			]}
		]}`
	var input dto.ClaudeRequest
	require.NoError(t, kitutil.Unmarshal([]byte(body), &input))
	before, err := kitutil.Marshal(input)
	require.NoError(t, err)
	converted, err := ClaudeMessagesRequestToOpenAIChat(input, nil)
	require.NoError(t, err)
	encoded, err := kitutil.Marshal(converted)
	require.NoError(t, err)
	var got dto.GeneralOpenAIRequest
	require.NoError(t, kitutil.Unmarshal(encoded, &got))
	require.Equal(t, "gpt-6.1-sol", got.Model)
	require.Len(t, got.Messages, 5)
	require.Equal(t, "I will inspect both results.", got.Messages[1].StringContent())
	require.Len(t, got.Messages[1].ParseToolCalls(), 2)
	for i, id := range []string{"call_a", "call_b"} {
		message := got.Messages[i+2]
		require.Equal(t, "tool", message.Role)
		require.Equal(t, id, message.ToolCallId)
		require.Equal(t, "view", *message.Name)
		require.NotEmpty(t, message.StringContent())
		require.NotContains(t, message.StringContent(), "aW1hZ2U", "image data must not become tool-result text")
	}
	require.Contains(t, got.Messages[3].StringContent(), "Two views of the second result.")
	// All tool replies must precede the user message carrying their images.
	media := got.Messages[4]
	require.Equal(t, "user", media.Role)
	var urls []string
	for _, part := range media.ParseContent() {
		if part.Type == "image_url" {
			urls = append(urls, part.GetImageMedia().Url)
		} else if part.Type == "text" {
			require.NotEmpty(t, part.Text)
		}
	}
	require.Equal(t, []string{
		"data:image/png;base64,aW1hZ2UtYQ==",
		"https://example.test/b.png",
		"data:image/jpeg;base64,aW1hZ2UtYw==",
	}, urls)
	require.Contains(t, media.StringContent(), "call_a")
	require.Contains(t, media.StringContent(), "call_b")
	require.Contains(t, media.StringContent(), "Explain the differences.")
	after, err := kitutil.Marshal(input)
	require.NoError(t, err)
	require.Equal(t, before, after, "conversion must not mutate the source request")
}

func TestClaudeMessagesToChatPreservesTextOnlyToolResults(t *testing.T) {
	for _, content := range []any{
		"plain result",
		"",
		[]dto.ClaudeMediaMessage{{Type: "text", Text: kitutil.GetPointer("array result")}},
	} {
		input := dto.ClaudeRequest{Model: "gpt-6.1-sol", Messages: []dto.ClaudeMessage{
			{Role: "assistant", Content: []dto.ClaudeMediaMessage{{Type: "tool_use", Id: "call", Name: "read", Input: map[string]any{}}}},
			{Role: "user", Content: []dto.ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call", Content: content}}},
		}}
		got, err := ClaudeMessagesRequestToOpenAIChat(input, nil)
		require.NoError(t, err)
		require.Len(t, got.Messages, 2)
		require.Nil(t, got.Messages[0].Content, "a tool-only assistant message does not need fabricated text")
		require.Len(t, got.Messages[0].ParseToolCalls(), 1)
		want, ok := content.(string)
		if !ok {
			encoded, err := kitutil.Marshal(content)
			require.NoError(t, err)
			want = string(encoded)
		}
		require.Equal(t, want, got.Messages[1].StringContent())
	}
}

func TestClaudeMessagesToChatWaitsForSplitParallelToolResults(t *testing.T) {
	var input dto.ClaudeRequest
	require.NoError(t, kitutil.Unmarshal([]byte(`{
		"model":"gpt-6.1-sol",
		"messages":[
			{"role":"assistant","content":[
				{"type":"tool_use","id":"a","name":"view","input":{}},
				{"type":"tool_use","id":"b","name":"view","input":{}}
			]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":[
				{"type":"image","source":{"type":"base64","media_type":"image/png","data":"YQ=="}}
			]}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"second result"}]}
		]}`), &input))
	got, err := ClaudeMessagesRequestToOpenAIChat(input, nil)
	require.NoError(t, err)
	require.Len(t, got.Messages, 4)
	require.Equal(t, "assistant", got.Messages[0].Role)
	require.Equal(t, "tool", got.Messages[1].Role)
	require.Equal(t, "a", got.Messages[1].ToolCallId)
	require.Equal(t, "tool", got.Messages[2].Role)
	require.Equal(t, "b", got.Messages[2].ToolCallId)
	require.Equal(t, "user", got.Messages[3].Role)
	parts := got.Messages[3].ParseContent()
	require.Len(t, parts, 2)
	require.Equal(t, "data:image/png;base64,YQ==", parts[1].GetImageMedia().Url)
}

func TestClaudeMessagesToChatRejectsInvalidToolImages(t *testing.T) {
	for _, image := range []dto.ClaudeMediaMessage{
		{Type: "image"},
		{Type: "image", Source: &dto.ClaudeMessageSource{Type: "url"}},
		{Type: "image", Source: &dto.ClaudeMessageSource{Type: "base64", MediaType: "image/png"}},
	} {
		_, err := ClaudeMessagesRequestToOpenAIChat(dto.ClaudeRequest{
			Model: "gpt-6.1-sol", Messages: []dto.ClaudeMessage{
				{Role: "user", Content: []dto.ClaudeMediaMessage{{Type: "tool_result", ToolUseId: "call", Content: []dto.ClaudeMediaMessage{image}}}},
			},
		}, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "call")
	}
}
