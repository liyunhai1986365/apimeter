package oaichat

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/stretchr/testify/require"
)

func TestChatToResponsesStreamReopensSegmentsWithoutChangingClosedItems(t *testing.T) {
	for _, test := range []struct {
		name, firstFinish, firstStatus, terminal, lastStatus string
	}{
		{"completed_then_failed", "stop", "completed", "response.failed", "incomplete"},
		{"incomplete_then_completed", "length", "incomplete", "response.completed", "completed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := NewChatToResponsesStreamState("resp_segments", "test-model")
			state.Tools = &convmeta.ResponsesToolState{CustomToolNames: map[string]struct{}{"exec": {}}}
			var events []ChatToResponsesStreamEvent
			appendChoice := func(delta dto.ChatCompletionsStreamResponseChoiceDelta, finish string) {
				choice := dto.ChatCompletionsStreamResponseChoice{Delta: delta}
				if finish != "" {
					choice.FinishReason = &finish
				}
				events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
					Choices: []dto.ChatCompletionsStreamResponseChoice{choice},
				})...)
			}
			appendChoice(dto.ChatCompletionsStreamResponseChoiceDelta{
				Content: kitutil.GetPointer("first "), ReasoningContent: kitutil.GetPointer("reason first"),
				Annotations: []byte(`[{"type":"url_citation","url_citation":{"url":"https://first.example","start_index":0,"end_index":5}}]`),
				ToolCalls:   []dto.ToolCallResponse{{Index: kitutil.GetPointer(0), ID: "call_first", Function: dto.FunctionResponse{Name: "lookup", Arguments: `{"q":"first"}`}}},
			}, test.firstFinish)
			require.Equal(t, "first ", state.UsageText())

			// Reasoning and text can reopen independently, with tools between
			// segments. Previously emitted item status must remain unchanged.
			appendChoice(dto.ChatCompletionsStreamResponseChoiceDelta{ReasoningContent: kitutil.GetPointer("reason second")}, "tool_calls")
			appendChoice(dto.ChatCompletionsStreamResponseChoiceDelta{
				Content:     kitutil.GetPointer("second "),
				Annotations: []byte(`[{"type":"url_citation","url_citation":{"url":"https://second.example","start_index":0,"end_index":6}}]`),
				ToolCalls:   []dto.ToolCallResponse{{Index: kitutil.GetPointer(1), ID: "custom_second", Function: dto.FunctionResponse{Name: "exec", Arguments: `{"input":"echo second"}`}}},
			}, "tool_calls")
			require.Equal(t, "first second ", state.UsageText())
			appendChoice(dto.ChatCompletionsStreamResponseChoiceDelta{
				Content: kitutil.GetPointer("third"), ReasoningContent: kitutil.GetPointer("reason third"),
			}, "")
			appendChoice(dto.ChatCompletionsStreamResponseChoiceDelta{Content: kitutil.GetPointer(" tail")}, "")
			require.Equal(t, "first second third tail", state.UsageText())
			events = append(events, state.AddWebSearch([]string{"latest news"})...)
			events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
				Usage: &dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120},
			})...)
			if test.terminal == "response.failed" {
				events = append(events, state.Fail("server_error", "interrupted")...)
			} else {
				events = append(events, FinalizeChatCompletionsStreamToResponses(state)...)
			}
			require.Equal(t, "first second third tail", state.UsageText())
			require.Equal(t, test.terminal, events[len(events)-1].Type)
			response := events[len(events)-1].Payload.Response
			require.Equal(t, 120, response.Usage.TotalTokens)
			require.Equal(t, 100, response.Usage.InputTokens)
			require.Equal(t, 20, response.Usage.OutputTokens)
			require.Nil(t, response.IncompleteDetails)

			wantIDs := []string{"resp_segments_reasoning_0", "resp_segments_msg_0", "call_first", "resp_segments_reasoning_1", "resp_segments_msg_1", "custom_second", "resp_segments_reasoning_2", "resp_segments_msg_2", "ws_resp_segments_0"}
			require.Len(t, response.Output, len(wantIDs))
			for index, item := range response.Output {
				require.Equal(t, wantIDs[index], item.ID)
				status := "completed"
				if index < 3 {
					status = test.firstStatus
				} else if index == 6 || index == 7 {
					status = test.lastStatus
				}
				require.Equal(t, status, item.Status, "item %s", item.ID)
			}
			require.Equal(t, "reason first", response.Output[0].Content[0].Text)
			require.Equal(t, "first ", response.Output[1].Content[0].Text)
			require.Equal(t, "reason second", response.Output[3].Content[0].Text)
			require.Equal(t, "second ", response.Output[4].Content[0].Text)
			require.Equal(t, "reason third", response.Output[6].Content[0].Text)
			require.Equal(t, "third tail", response.Output[7].Content[0].Text)
			require.Len(t, response.Output[1].Content[0].Annotations, 1)
			require.Len(t, response.Output[4].Content[0].Annotations, 1)
			require.Empty(t, response.Output[7].Content[0].Annotations)
			require.Equal(t, "custom_tool_call", response.Output[5].Type)
			require.Equal(t, "echo second", *response.Output[5].Input)
			assertClosedResponsesItemsMatchSnapshot(t, events, response.Output)
			require.Empty(t, state.Fail("server_error", "again"))
			require.Empty(t, FinalizeChatCompletionsStreamToResponses(state))
			require.Empty(t, state.AddWebSearch([]string{"late search"}))
			require.Empty(t, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
				Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: kitutil.GetPointer("late")}}},
			}))
		})
	}
}

func TestChatToResponsesStreamAnnotationsStartAnOpenMessage(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_citations", "test-model")
	var events []ChatToResponsesStreamEvent
	for _, text := range []string{"first", "second"} {
		annotation, err := kitutil.Marshal([]map[string]any{{"type": "url_citation", "url_citation": map[string]any{"url": "https://" + text + ".example", "start_index": 0, "end_index": len(text)}}})
		require.NoError(t, err)
		// Covers citations before the first text and after a closed message.
		chunk := &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Annotations: annotation}}}}
		annotationEvents := mustResponsesEventsFromChatChunk(t, state, chunk)
		require.Equal(t, "response.output_item.added", annotationEvents[len(annotationEvents)-2].Type)
		require.Equal(t, "response.output_text.annotation.added", annotationEvents[len(annotationEvents)-1].Type)
		require.Zero(t, *annotationEvents[len(annotationEvents)-1].Payload.AnnotationIndex)
		events = append(events, annotationEvents...)
		events = append(events, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
			Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &text}, FinishReason: kitutil.GetPointer("stop")}},
		})...)
	}
	// Empty or invalid annotations must not open another message.
	for _, raw := range []string{"[]", "invalid"} {
		require.Empty(t, mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{
			Choices: []dto.ChatCompletionsStreamResponseChoice{{Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Annotations: []byte(raw)}}},
		}))
	}
	events = append(events, FinalizeChatCompletionsStreamToResponses(state)...)
	output := events[len(events)-1].Payload.Response.Output
	require.Len(t, output, 2)
	for index, text := range []string{"first", "second"} {
		require.Equal(t, text, output[index].Content[0].Text)
		require.Len(t, output[index].Content[0].Annotations, 1)
		require.Equal(t, "https://"+text+".example", output[index].Content[0].Annotations[0].(map[string]any)["url"])
	}
	require.Equal(t, "firstsecond", state.UsageText())
	assertClosedResponsesItemsMatchSnapshot(t, events, output)
}

func assertClosedResponsesItemsMatchSnapshot(t *testing.T, events []ChatToResponsesStreamEvent, output []dto.ResponsesOutput) {
	t.Helper()
	indexes := make(map[string]int)
	done := make(map[string]dto.ResponsesOutput)
	created := 0
	for _, event := range events {
		switch event.Type {
		case "response.created":
			created++
		case "response.output_item.added":
			require.NotContains(t, indexes, event.Payload.Item.ID)
			require.Equal(t, len(indexes), *event.Payload.OutputIndex)
			indexes[event.Payload.Item.ID] = *event.Payload.OutputIndex
		case "response.output_item.done":
			require.Contains(t, indexes, event.Payload.Item.ID)
			require.NotContains(t, done, event.Payload.Item.ID, "each item closes once")
			require.Equal(t, indexes[event.Payload.Item.ID], *event.Payload.OutputIndex)
			done[event.Payload.Item.ID] = *event.Payload.Item
		default:
			if event.Payload.ItemID != "" {
				require.Contains(t, indexes, event.Payload.ItemID, "delta must follow item.added")
				require.NotContains(t, done, event.Payload.ItemID, "closed items cannot receive more events")
				require.Equal(t, indexes[event.Payload.ItemID], *event.Payload.OutputIndex)
			}
		}
	}
	require.Equal(t, 1, created)
	require.Len(t, done, len(output))
	for _, item := range output {
		require.Equal(t, done[item.ID], item, "final snapshot must preserve the emitted done item")
	}
}

func TestChatToResponsesDeferredChoicesKeepFinishStatusOnFailure(t *testing.T) {
	for _, finish := range []struct{ reason, status string }{
		{"stop", "completed"}, {"length", "incomplete"}, {"content_filter", "incomplete"},
	} {
		t.Run(finish.reason, func(t *testing.T) {
			state := NewChatToResponsesStreamState("resp_choices", "test-model")
			state.DeferChoiceFinish = true
			state.Tools = &convmeta.ResponsesToolState{CustomToolNames: map[string]struct{}{"exec": {}}}
			choice := func(index int, prefix string, finishReason *string) dto.ChatCompletionsStreamResponseChoice {
				return dto.ChatCompletionsStreamResponseChoice{
					Index: index,
					Delta: dto.ChatCompletionsStreamResponseChoiceDelta{
						Content: kitutil.GetPointer(prefix + " text"), ReasoningContent: kitutil.GetPointer(prefix + " reasoning"),
						ToolCalls: []dto.ToolCallResponse{
							{Index: kitutil.GetPointer(0), ID: prefix + "_function", Function: dto.FunctionResponse{Name: "lookup", Arguments: `{"q":"` + prefix + `"}`}},
							{Index: kitutil.GetPointer(1), ID: prefix + "_custom", Function: dto.FunctionResponse{Name: "exec", Arguments: `{"input":"` + prefix + `"}`}},
						},
					},
					FinishReason: finishReason,
				}
			}
			events := mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{
				choice(5, "finished", &finish.reason), choice(2, "unfinished", nil),
			}})
			for _, event := range events {
				require.NotEqual(t, "response.output_item.done", event.Type)
			}
			events = append(events, state.Fail("server_error", "failed tail")...)
			output := events[len(events)-1].Payload.Response.Output
			require.Len(t, output, 8)
			for index, item := range output {
				want := "incomplete"
				if index < 4 {
					want = finish.status
				}
				require.Equal(t, want, item.Status, "item %s", item.ID)
			}
			require.Equal(t, "finished", *output[3].Input)
			require.Equal(t, "unfinished", *output[7].Input)
			require.Equal(t, "finished textunfinished text", state.UsageText())
			assertClosedResponsesItemsMatchSnapshot(t, events, output)
		})
	}
}

func TestChatToResponsesDeferredChoiceStartsANewBodySegment(t *testing.T) {
	state := NewChatToResponsesStreamState("resp_reopened", "test-model")
	state.DeferChoiceFinish = true
	chunk := func(text, reasoning, finish string) *dto.ChatCompletionsStreamResponse {
		choice := dto.ChatCompletionsStreamResponseChoice{Index: 3, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Content: &text, ReasoningContent: &reasoning}}
		if finish != "" {
			choice.FinishReason = &finish
		}
		return &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{choice}}
	}
	events := mustResponsesEventsFromChatChunk(t, state, chunk("first", "reason first", "stop"))
	annotationEvents := mustResponsesEventsFromChatChunk(t, state, &dto.ChatCompletionsStreamResponse{Choices: []dto.ChatCompletionsStreamResponseChoice{{
		Index: 3, Delta: dto.ChatCompletionsStreamResponseChoiceDelta{Annotations: []byte(`[{"type":"url_citation","url_citation":{"start_index":0,"end_index":5,"url":"https://first.example"}}]`)},
	}}})
	require.Len(t, annotationEvents, 1)
	require.Equal(t, "resp_reopened_msg_0", annotationEvents[0].Payload.ItemID)
	events = append(events, annotationEvents...)
	events = append(events, mustResponsesEventsFromChatChunk(t, state, chunk("second", "reason second", ""))...)
	require.Equal(t, "firstsecond", state.UsageText())
	events = append(events, state.Fail("server_error", "interrupted")...)
	output := events[len(events)-1].Payload.Response.Output
	require.Len(t, output, 4)
	require.Equal(t, "reason first", output[0].Content[0].Text)
	require.Equal(t, "first", output[1].Content[0].Text)
	require.Len(t, output[1].Content[0].Annotations, 1)
	require.Equal(t, "reason second", output[2].Content[0].Text)
	require.Equal(t, "second", output[3].Content[0].Text)
	require.Empty(t, output[3].Content[0].Annotations)
	require.Equal(t, "completed", output[0].Status)
	require.Equal(t, "completed", output[1].Status)
	require.Equal(t, "incomplete", output[2].Status)
	require.Equal(t, "incomplete", output[3].Status)
	assertClosedResponsesItemsMatchSnapshot(t, events, output)
}
