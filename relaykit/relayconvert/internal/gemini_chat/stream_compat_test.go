package geminichat

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
)

func geminiFrame(t *testing.T, raw string) *dto.GeminiChatResponse {
	t.Helper()
	var response dto.GeminiChatResponse
	require.NoError(t, kitutil.UnmarshalJsonStr(raw, &response))
	return &response
}

func TestGeminiPartialArgumentsBecomeOneExecutableCall(t *testing.T) {
	s := NewGeminiToChatStreamState("id", 1)
	frames := []string{
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_a","name":"run","partialArgs":[{"jsonPath":"$.text","stringValue":"hel"},{"jsonPath":"$.flags[0]","boolValue":false}],"willContinue":true}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"partialArgs":[{"jsonPath":"$.text","stringValue":"lo"},{"jsonPath":"$['odd.key']","numberValue":0},{"jsonPath":"$.nil","nullValue":"NULL_VALUE"}],"willContinue":false}}]}}]}`,
	}
	chunks, err := s.ConvertChunk(geminiFrame(t, frames[0]), "m", nil)
	require.NoError(t, err)
	require.Empty(t, chunks[0].Choices[0].Delta.ToolCalls)
	require.Nil(t, chunks[0].Choices[0].FinishReason)
	chunks, err = s.ConvertChunk(geminiFrame(t, frames[1]), "m", nil)
	require.NoError(t, err)
	require.Len(t, chunks[0].Choices[0].Delta.ToolCalls, 1)
	call := chunks[0].Choices[0].Delta.ToolCalls[0]
	require.Equal(t, "call_a", call.ID)
	require.Equal(t, 0, *call.Index)
	require.JSONEq(t, `{"text":"hello","flags":[false],"odd.key":0,"nil":null}`, call.Function.Arguments)
	chunks, err = s.ConvertChunk(geminiFrame(t, `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call_b","name":"run","args":{"a":1},"willContinue":false}}]},"finishReason":"STOP"}]}`), "m", nil)
	require.NoError(t, err)
	call = chunks[0].Choices[0].Delta.ToolCalls[0]
	require.Equal(t, 1, *call.Index)
	require.JSONEq(t, `{"a":1}`, call.Function.Arguments)
	require.Equal(t, "tool_calls", *chunks[len(chunks)-1].Choices[0].FinishReason)
	chunks, err = s.Finalize("m")
	require.NoError(t, err)
	require.Empty(t, chunks)
}

func TestGeminiRejectsIncompleteOrInvalidPartialCalls(t *testing.T) {
	for _, path := range []string{"$.a[4096]", "$.a[*]", "$"} {
		s := NewGeminiToChatStreamState("id", 1)
		_, err := s.ConvertChunk(&dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{FunctionCall: &dto.FunctionCall{FunctionName: "run", PartialArgs: []dto.GeminiPartialArg{{JSONPath: path, StringValue: kitutil.GetPointer("x")}}}}}}}}}, "m", nil)
		require.Error(t, err)
	}
	s := NewGeminiToChatStreamState("id", 1)
	_, err := s.ConvertChunk(geminiFrame(t, `{"candidates":[{"content":{"parts":[{"functionCall":{"id":"a","name":"run","partialArgs":[{"jsonPath":"$.text","stringValue":"x"}],"willContinue":true}}]}}]}`), "m", nil)
	require.NoError(t, err)
	_, err = s.Finalize("m")
	require.ErrorContains(t, err, "incomplete function call")
	_, err = s.ConvertChunk(geminiFrame(t, `{"candidates":[{"finishReason":"STOP"}]}`), "m", nil)
	require.ErrorContains(t, err, "incomplete function call")
}

func TestGeminiLateGroundingAnnotationsAreDeduplicated(t *testing.T) {
	s := NewGeminiToChatStreamState("id", 1)
	_, err := s.ConvertChunk(geminiFrame(t, `{"candidates":[{"content":{"parts":[{"text":"Hello"}]}}]}`), "m", nil)
	require.NoError(t, err)
	metadata := `{"candidates":[{"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://example.com","title":"Example"}}],"groundingSupports":[{"segment":{"partIndex":0,"startIndex":0,"endIndex":5,"text":"Hello"},"groundingChunkIndices":[0]}]}}]}`
	chunks, err := s.ConvertChunk(geminiFrame(t, metadata), "m", nil)
	require.NoError(t, err)
	require.Contains(t, string(chunks[0].Choices[0].Delta.Annotations), "https://example.com")
	chunks, err = s.ConvertChunk(geminiFrame(t, metadata), "m", nil)
	require.NoError(t, err)
	require.Empty(t, chunks[0].Choices[0].Delta.Annotations)
}

func TestGeminiCandidateFinishesAndUsageAreIndependent(t *testing.T) {
	s := NewGeminiToChatStreamState("id", 1)
	chunks, err := s.ConvertChunk(geminiFrame(t, `{"candidates":[{"index":1,"content":{"parts":[{"functionCall":{"id":"tool","name":"run","args":{}}}]},"finishReason":"STOP"},{"index":2,"content":{"parts":[{"text":"Hi"}]}}],"usageMetadata":{"promptTokenCount":10,"thoughtsTokenCount":3}}`), "m", &dto.Usage{PromptTokens: 10})
	require.NoError(t, err)
	require.Len(t, chunks, 2)
	require.Equal(t, 1, chunks[1].Choices[0].Index)
	require.Equal(t, "tool_calls", *chunks[1].Choices[0].FinishReason)
	chunks, err = s.ConvertChunk(geminiFrame(t, `{"candidates":[{"index":2,"finishReason":"STOP"}],"usageMetadata":{"candidatesTokenCount":5}}`), "m", &dto.Usage{CompletionTokens: 5})
	require.NoError(t, err)
	require.Equal(t, 2, chunks[1].Choices[0].Index)
	require.Equal(t, "stop", *chunks[1].Choices[0].FinishReason)
	require.Equal(t, 10, s.Usage().PromptTokens)
	require.Equal(t, 8, s.Usage().CompletionTokens)
	chunks, err = s.Finalize("m")
	require.NoError(t, err)
	require.Empty(t, chunks)
}

func TestGeminiStreamFinishReasons(t *testing.T) {
	for finish, expected := range map[string]string{
		"STOP": "stop", "MAX_TOKENS": "length", "CONTINUATION": "length", "SAFETY": "content_filter",
	} {
		t.Run(finish, func(t *testing.T) {
			s := NewGeminiToChatStreamState("id", 1)
			frame := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{
				Index: 2, FinishReason: kitutil.GetPointer(finish), Content: dto.GeminiChatContent{Parts: []dto.GeminiPart{{Text: "partial"}}},
			}}}
			chunks, err := s.ConvertChunk(frame, "model", nil)
			require.NoError(t, err)
			finishes := 0
			for _, chunk := range chunks {
				for _, choice := range chunk.Choices {
					if choice.FinishReason != nil {
						finishes++
						require.Equal(t, 2, choice.Index)
						require.Equal(t, expected, *choice.FinishReason)
					}
				}
			}
			require.Equal(t, 1, finishes)
			chunks, err = s.Finalize("model")
			require.NoError(t, err)
			require.Empty(t, chunks, "finalization must not overwrite or duplicate the finish reason")
		})
	}
}
