package relayconvert

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
)

func TestFailResponsesStreamPreservesPartialOutputAndIsTerminal(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_output", true: "partial_output"}[started], func(t *testing.T) {
			state, err := NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "resp_failure", Model: "test-model"})
			require.NoError(t, err)
			if started {
				_, err = ConvertStreamResponseChunk(nil, nil, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: kitutil.GetPointer("partial answer")},
				}}})
				require.NoError(t, err)
			}
			state.SetUsage(&dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120})
			results, handled := state.FailResponsesStream("server_error", "upstream interrupted")
			require.True(t, handled)
			require.NotEmpty(t, results)
			if !started {
				require.Equal(t, "response.created", results[0].Value.(ChatToResponsesStreamEvent).Type)
			}
			failed := results[len(results)-1].Value.(ChatToResponsesStreamEvent)
			require.Equal(t, "response.failed", failed.Type)
			require.Equal(t, `"failed"`, string(failed.Payload.Response.Status))
			require.Equal(t, 100, failed.Payload.Response.Usage.InputTokens)
			require.Equal(t, 20, failed.Payload.Response.Usage.OutputTokens)
			require.Equal(t, "server_error", failed.Payload.Response.GetOpenAIError().Code)
			if started {
				require.Len(t, failed.Payload.Response.Output, 1)
				require.Equal(t, "incomplete", failed.Payload.Response.Output[0].Status)
				require.Equal(t, "partial answer", failed.Payload.Response.Output[0].Content[0].Text)
			}
			for _, result := range results {
				require.NotEqual(t, "response.completed", result.Value.(ChatToResponsesStreamEvent).Type)
			}
			repeated, handled := state.FailResponsesStream("server_error", "again")
			require.True(t, handled)
			require.Empty(t, repeated)
			final, err := FinalizeStreamResponse(nil, nil, state)
			require.NoError(t, err)
			require.Empty(t, final)
			late, err := ConvertStreamResponseChunk(nil, nil, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: kitutil.GetPointer("late output")},
			}}})
			require.NoError(t, err)
			require.Empty(t, late)
		})
	}
}

func TestFailResponsesStreamPreservesObservedSearchAndRemainsTerminal(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "search_only", true: "partial_answer"}[started], func(t *testing.T) {
			state, err := NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "resp_search_failure"})
			require.NoError(t, err)
			if started {
				_, err = ConvertStreamResponseChunk(nil, nil, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: kitutil.GetPointer("partial search answer")},
				}}})
				require.NoError(t, err)
			}
			search := &dto.GeminiChatResponse{Candidates: []dto.GeminiChatCandidate{{GroundingMetadata: &dto.GeminiGroundingMetadata{WebSearchQueries: []string{"latest weather"}}}}}
			state.ObserveGeminiSearch(search)
			state.ObserveGeminiSearch(search)
			results, handled := state.FailResponsesStream("server_error", "upstream interrupted")
			require.True(t, handled)
			require.NotEmpty(t, results)
			failed := results[len(results)-1].Value.(ChatToResponsesStreamEvent)
			require.Equal(t, "response.failed", failed.Type)
			require.Equal(t, `"failed"`, string(failed.Payload.Response.Status))
			searchItems := 0
			for _, item := range failed.Payload.Response.Output {
				if item.Type == "web_search_call" {
					searchItems++
					require.Equal(t, "completed", item.Status)
				} else {
					require.Equal(t, "incomplete", item.Status)
				}
			}
			require.Equal(t, 1, searchItems)
			for _, result := range results {
				require.NotEqual(t, "response.completed", result.Value.(ChatToResponsesStreamEvent).Type)
			}
			// Even late, previously unseen search metadata cannot reopen a
			// failed stream or produce a second terminal event.
			search.Candidates[0].GroundingMetadata.WebSearchQueries = []string{"late query"}
			state.ObserveGeminiSearch(search)
			repeated, handled := state.FailResponsesStream("server_error", "again")
			require.True(t, handled)
			require.Empty(t, repeated)
			final, err := FinalizeStreamResponse(nil, nil, state)
			require.NoError(t, err)
			require.Empty(t, final)
		})
	}
}

func TestFailResponsesStreamPreservesEachItemsEmittedStatus(t *testing.T) {
	for _, test := range []struct {
		name         string
		finishReason *string
		wantStatus   string
	}{
		{"completed", kitutil.GetPointer("stop"), "completed"},
		{"incomplete", kitutil.GetPointer("length"), "incomplete"},
		{"unfinished", nil, "incomplete"},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := &convmeta.Values{ResponsesTools: &convmeta.ResponsesToolState{
				CustomToolNames: map[string]struct{}{"exec": {}},
			}}
			state, err := NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "resp_item_status"})
			require.NoError(t, err)
			tool := func(index int, id, name string) dto.ToolCallResponse {
				return dto.ToolCallResponse{Index: kitutil.GetPointer(index), ID: id, Type: "function", Function: dto.FunctionResponse{Name: name, Arguments: `{"input":"echo hi"}`}}
			}
			done := make(map[string]string)
			recordDone := func(results []ResponseResult) {
				for _, result := range results {
					event := result.Value.(ChatToResponsesStreamEvent)
					require.NotEqual(t, "response.completed", event.Type)
					if event.Type == "response.output_item.done" {
						require.NotContains(t, done, event.Payload.Item.ID, "item must not emit done twice")
						done[event.Payload.Item.ID] = event.Payload.Item.Status
					}
				}
			}
			initial, err := ConvertStreamResponseChunk(nil, info, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
					Content:          kitutil.GetPointer("answer"),
					ReasoningContent: kitutil.GetPointer("reasoning"),
					ToolCalls:        []dto.ToolCallResponse{tool(0, "call_finished", "run"), tool(1, "custom_finished", "exec")},
				},
				FinishReason: test.finishReason,
			}}})
			require.NoError(t, err)
			recordDone(initial)
			if test.finishReason == nil {
				require.Empty(t, done)
			} else {
				require.Len(t, done, 4)
				for _, status := range done {
					require.Equal(t, test.wantStatus, status)
				}
			}

			// A different candidate or later tool can remain unfinished after
			// earlier items have already emitted their terminal status.
			pending, err := ConvertStreamResponseChunk(nil, info, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{
				Delta: dto.ChatCompletionsStreamResponseChoiceDelta{ToolCalls: []dto.ToolCallResponse{tool(2, "call_pending", "run"), tool(3, "custom_pending", "exec")}},
			}}})
			require.NoError(t, err)
			recordDone(pending)
			state.SetUsage(&dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120})
			failed, handled := state.FailResponsesStream("server_error", "upstream interrupted")
			require.True(t, handled)
			recordDone(failed)
			terminal := failed[len(failed)-1].Value.(ChatToResponsesStreamEvent)
			require.Equal(t, "response.failed", terminal.Type)
			require.Equal(t, `"failed"`, string(terminal.Payload.Response.Status))
			require.Nil(t, terminal.Payload.Response.IncompleteDetails)
			require.Equal(t, 120, terminal.Payload.Response.Usage.TotalTokens)
			require.Len(t, done, 6)
			require.Len(t, terminal.Payload.Response.Output, 6)
			typesSeen := make(map[string]bool)
			for _, item := range terminal.Payload.Response.Output {
				typesSeen[item.Type] = true
				want := test.wantStatus
				if item.ID == "call_pending" || item.ID == "custom_pending" {
					want = "incomplete"
				}
				require.Equal(t, want, item.Status, "item %s", item.ID)
				require.Equal(t, done[item.ID], item.Status, "snapshot must match the emitted done status")
			}
			require.Equal(t, map[string]bool{"message": true, "reasoning": true, "function_call": true, "custom_tool_call": true}, typesSeen)
			repeated, handled := state.FailResponsesStream("server_error", "again")
			require.True(t, handled)
			require.Empty(t, repeated)
			final, err := FinalizeStreamResponse(nil, info, state)
			require.NoError(t, err)
			require.Empty(t, final)
		})
	}
}

func TestGeminiResponsesDefersEachCandidatesDoneUntilGrounding(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal_eof", true: "failed_tail"}[failure], func(t *testing.T) {
			state, err := NewResponseStreamState(types.RelayFormatGemini, types.RelayFormatOpenAIResponses, ResponseStreamOptions{ID: "resp_grounding"})
			require.NoError(t, err)
			var events []ChatToResponsesStreamEvent
			appendResults := func(results []ResponseResult) {
				for _, result := range results {
					events = append(events, result.Value.(ChatToResponsesStreamEvent))
				}
			}
			convert := func(raw string) []ChatToResponsesStreamEvent {
				var response dto.GeminiChatResponse
				require.NoError(t, kitutil.Unmarshal([]byte(raw), &response))
				results, err := ConvertStreamResponseChunk(nil, nil, state, &response)
				require.NoError(t, err)
				before := len(events)
				appendResults(results)
				return events[before:]
			}
			// Two candidates in one frame must get independent live items.
			initial := convert(`{"candidates":[{"index":0,"content":{"parts":[{"text":"reason zero","thought":true}]}},{"index":1,"content":{"parts":[{"text":"reason one","thought":true}]}}]}`)
			require.Len(t, initial, 5) // created, two item.added and two reasoning deltas
			require.Equal(t, "response.reasoning_summary_text.delta", initial[2].Type)
			require.Equal(t, "reason zero", initial[2].Payload.Delta)
			require.Equal(t, "reason one", initial[4].Payload.Delta)
			textEvents := convert(`{"candidates":[{"index":0,"content":{"parts":[{"text":"晴天"},{"functionCall":{"id":"call_zero","name":"lookup","args":{"q":"zero"}}}]},"finishReason":"STOP"},{"index":1,"content":{"parts":[{"text":"阴天"},{"functionCall":{"id":"call_one","name":"lookup","args":{"q":"one"}}}]},"finishReason":"STOP"}]}`)
			var texts []string
			for _, event := range textEvents {
				if event.Type == "response.output_text.delta" {
					texts = append(texts, event.Payload.Delta)
				}
			}
			require.Equal(t, []string{"晴天", "阴天"}, texts, "text is emitted before finalization")
			// Grounding returns to candidate zero after candidate one's text.
			annotationEvents := convert(`{"candidates":[{"index":0,"groundingMetadata":{"groundingChunks":[{"web":{"uri":"https://zero.example"}}],"groundingSupports":[{"segment":{"partIndex":0,"startIndex":0,"endIndex":6,"text":"晴天"},"groundingChunkIndices":[0]}]}}]}`)
			require.Len(t, annotationEvents, 1)
			require.Equal(t, "response.output_text.annotation.added", annotationEvents[0].Type)
			require.Equal(t, "resp_grounding_msg_0", annotationEvents[0].Payload.ItemID)
			convert(`{"candidates":[{"index":2,"content":{"parts":[{"text":"unfinished"}]}}],"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":20,"totalTokenCount":120}}`)
			for _, event := range events {
				require.NotEqual(t, "response.output_item.done", event.Type)
			}
			require.Equal(t, "晴天阴天unfinished", state.UsageText())
			if failure {
				results, handled := state.FailResponsesStream("server_error", "interrupted")
				require.True(t, handled)
				appendResults(results)
			} else {
				results, err := FinalizeStreamResponse(nil, nil, state)
				require.NoError(t, err)
				appendResults(results)
			}
			terminal := events[len(events)-1]
			wantTerminal, wantLast := "response.completed", "completed"
			if failure {
				wantTerminal, wantLast = "response.failed", "incomplete"
			}
			require.Equal(t, wantTerminal, terminal.Type)
			require.Equal(t, 120, terminal.Payload.Response.Usage.TotalTokens)
			output := terminal.Payload.Response.Output
			require.Len(t, output, 7)
			require.Equal(t, "reason zero", output[0].Content[0].Text)
			require.Equal(t, "reason one", output[1].Content[0].Text)
			require.Equal(t, "晴天", output[2].Content[0].Text)
			require.Len(t, output[2].Content[0].Annotations, 1)
			require.Equal(t, "https://zero.example", output[2].Content[0].Annotations[0].(map[string]any)["url"])
			require.Equal(t, "call_zero", output[3].ID)
			var zeroArguments string
			require.NoError(t, kitutil.Unmarshal(output[3].Arguments, &zeroArguments))
			require.JSONEq(t, `{"q":"zero"}`, zeroArguments)
			require.Equal(t, "阴天", output[4].Content[0].Text)
			require.Empty(t, output[4].Content[0].Annotations)
			require.Equal(t, "call_one", output[5].ID)
			var oneArguments string
			require.NoError(t, kitutil.Unmarshal(output[5].Arguments, &oneArguments))
			require.JSONEq(t, `{"q":"one"}`, oneArguments)
			for _, item := range output[:6] {
				require.Equal(t, "completed", item.Status)
			}
			require.Equal(t, wantLast, output[6].Status)
			require.Equal(t, "unfinished", output[6].Content[0].Text)
			done := make(map[string]dto.ResponsesOutput)
			for _, event := range events {
				if event.Type == "response.output_item.done" {
					require.NotContains(t, done, event.Payload.Item.ID)
					done[event.Payload.Item.ID] = *event.Payload.Item
				}
			}
			require.Len(t, done, len(output))
			for _, item := range output {
				require.Equal(t, done[item.ID], item, "done and final must agree")
			}
			require.Equal(t, "晴天阴天unfinished", state.UsageText())
			repeated, handled := state.FailResponsesStream("server_error", "again")
			require.True(t, handled)
			require.Empty(t, repeated)
		})
	}
}
