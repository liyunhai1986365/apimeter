package oaichat

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
)

type ChatToResponsesStreamEvent struct {
	Type    string
	Payload dto.ResponsesStreamResponse
}

type ChatToResponsesStreamState struct {
	*chatToResponsesStreamChoice
	Tools   *convmeta.ResponsesToolState
	hosted  []dto.ResponsesOutput
	ID      string
	Model   string
	Created int64
	Usage   *dto.Usage
	// DeferChoiceFinish keeps each choice's items open for trailing metadata.
	// Configure before the first chunk; ordinary Chat streams close immediately.
	DeferChoiceFinish bool

	status            string
	incompleteDetails *dto.IncompleteDetails
	sentCreated       bool
	finalized         bool
	nextOutputIndex   int
	nextTextID        int
	nextReasoningID   int
	outputOrder       []chatToResponsesOutputRef
	choices           map[int]*chatToResponsesStreamChoice
	choiceOrder       []*chatToResponsesStreamChoice
}

// Keep builders behind a pointer: switching choices must never copy a live
// strings.Builder, and late annotations must return to the original choice.
type chatToResponsesStreamChoice struct {
	annotations      []any
	finishedStatus   string
	textOutputIndex  int
	textStarted      bool
	textDone         bool
	textSegment      int
	textID           int
	closedMessages   []dto.ResponsesOutput
	reasoningIndex   int
	reasoningStarted bool
	reasoningDone    bool
	reasoningSegment int
	reasoningID      int
	closedReasonings []dto.ResponsesOutput
	toolsByIndex     map[int]*chatToResponsesStreamTool
	text             strings.Builder
	reasoning        strings.Builder
}

type chatToResponsesStreamTool struct {
	Custom      bool
	ChatIndex   int
	OutputIndex int
	ID          string
	Name        string
	Arguments   strings.Builder
	DoneStatus  string
}

type chatToResponsesOutputRef struct {
	Kind      string
	ToolIndex int
	Segment   int
	Choice    *chatToResponsesStreamChoice
}

func NewChatToResponsesStreamState(id string, model string) *ChatToResponsesStreamState {
	return &ChatToResponsesStreamState{
		chatToResponsesStreamChoice: newChatToResponsesStreamChoice(),
		ID:                          id,
		Model:                       model,
		Created:                     time.Now().Unix(),
		Usage:                       &dto.Usage{},
		status:                      "completed",
	}
}

func newChatToResponsesStreamChoice() *chatToResponsesStreamChoice {
	return &chatToResponsesStreamChoice{
		textOutputIndex: -1,
		reasoningIndex:  -1,
		toolsByIndex:    make(map[int]*chatToResponsesStreamTool),
	}
}

func ChatCompletionsStreamChunkToResponsesEvents(chunk *dto.ChatCompletionsStreamResponse, state *ChatToResponsesStreamState) ([]ChatToResponsesStreamEvent, error) {
	if chunk == nil || state == nil || state.finalized {
		return nil, nil
	}
	if state.ID == "" {
		state.ID = chunk.Id
	}
	if state.Model == "" {
		state.Model = chunk.Model
	}
	if state.Created == 0 {
		state.Created = chunk.Created
	}
	if chunk.Usage != nil {
		state.Usage = UsageFromChatUsage(chunk.Usage)
	}

	events := state.ensureCreated()
	for _, choice := range chunk.Choices {
		if state.DeferChoiceFinish {
			state.selectChoice(choice.Index)
			if state.finishedStatus != "" && (choice.Delta.GetReasoningContent() != "" || choice.Delta.GetContentString() != "" || len(choice.Delta.ToolCalls) > 0) {
				// Actual new output starts another segment. Annotation-only
				// chunks still belong to the finished, but not yet closed, one.
				events = append(events, state.doneChoiceEvents()...)
				state.finishedStatus = ""
			}
		}
		if choice.Delta.GetReasoningContent() != "" {
			events = append(events, state.appendReasoningDelta(choice.Delta.GetReasoningContent())...)
		}
		if choice.Delta.GetContentString() != "" {
			events = append(events, state.appendTextDelta(choice.Delta.GetContentString())...)
		}

		for _, annotation := range chatAnnotationsToResponses(choice.Delta.Annotations) {
			events = append(events, state.startText()...)
			index := len(state.annotations)
			state.annotations = append(state.annotations, annotation)
			events = append(events, responsesStreamEvent("response.output_text.annotation.added", dto.ResponsesStreamResponse{
				OutputIndex: intPtr(state.textOutputIndex), ContentIndex: intPtr(0), ItemID: state.messageID(), AnnotationIndex: intPtr(index), Annotation: annotation,
			}))
		}
		for _, toolCall := range choice.Delta.ToolCalls {
			toolEvents, err := state.appendToolCallDelta(toolCall)
			if err != nil {
				return nil, err
			}
			events = append(events, toolEvents...)
		}
		if choice.FinishReason != nil && strings.TrimSpace(*choice.FinishReason) != "" {
			state.applyFinishReason(*choice.FinishReason)
			if state.DeferChoiceFinish {
				state.finishedStatus = state.outputStatus()
			} else {
				events = append(events, state.doneChoiceEvents()...)
			}
		}
	}
	return events, nil
}

func (s *ChatToResponsesStreamState) selectChoice(index int) {
	if s.choices == nil {
		s.choices = make(map[int]*chatToResponsesStreamChoice)
	}
	choice := s.choices[index]
	if choice == nil {
		choice = newChatToResponsesStreamChoice()
		s.choices[index] = choice
		s.choiceOrder = append(s.choiceOrder, choice)
	}
	s.chatToResponsesStreamChoice = choice
}

func (s *ChatToResponsesStreamState) ensureCreated() []ChatToResponsesStreamEvent {
	if s.sentCreated {
		return nil
	}
	s.sentCreated = true
	return []ChatToResponsesStreamEvent{responsesStreamEvent(responsesEventCreated, dto.ResponsesStreamResponse{
		Type:     responsesEventCreated,
		Response: s.createdResponse(),
	})}
}

// Fail terminates the response without claiming that partial output completed.
// The failure snapshot retains the content and usage accumulated so far.
func (s *ChatToResponsesStreamState) Fail(code, message string) []ChatToResponsesStreamEvent {
	if s == nil || s.finalized {
		return nil
	}
	if code = strings.TrimSpace(code); code == "" {
		code = "server_error"
	}
	if message = strings.TrimSpace(message); message == "" {
		message = "upstream response stream failed"
	}
	s.status = "failed"
	s.incompleteDetails = nil
	events := s.ensureCreated()
	events = append(events, s.doneDeltaEvents()...)
	s.finalized = true
	response := s.finalResponse()
	response.Error = map[string]any{"code": code, "message": message}
	events = append(events, responsesStreamEvent("response.failed", dto.ResponsesStreamResponse{
		Type:     "response.failed",
		Response: response,
	}))
	return events
}

func FinalizeChatCompletionsStreamToResponses(state *ChatToResponsesStreamState) []ChatToResponsesStreamEvent {
	if state == nil || state.finalized {
		return nil
	}
	events := state.doneDeltaEvents()
	state.finalized = true
	resp := state.finalResponse()
	eventType := responsesEventCompleted
	if state.status == "incomplete" {
		eventType = responsesEventIncomplete
	}
	events = append(events, responsesStreamEvent(eventType, dto.ResponsesStreamResponse{
		Type:     eventType,
		Response: resp,
	}))
	return events
}

func (s *ChatToResponsesStreamState) UsageText() string {
	if s == nil {
		return ""
	}
	var text strings.Builder
	for _, ref := range s.outputOrder {
		if ref.Kind != "message" {
			continue
		}
		if ref.Segment < len(ref.Choice.closedMessages) {
			text.WriteString(ref.Choice.closedMessages[ref.Segment].Content[0].Text)
		} else {
			text.WriteString(ref.Choice.text.String())
		}
	}
	return text.String()
}

func (s *ChatToResponsesStreamState) appendTextDelta(delta string) []ChatToResponsesStreamEvent {
	events := s.startText()
	s.text.WriteString(delta)
	events = append(events, responsesStreamEvent(responsesEventOutputTextDelta, dto.ResponsesStreamResponse{
		Type:         responsesEventOutputTextDelta,
		OutputIndex:  intPtr(s.textOutputIndex),
		ContentIndex: intPtr(0),
		Delta:        delta,
		ItemID:       s.messageID(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) startText() []ChatToResponsesStreamEvent {
	if s.textDone {
		s.textSegment++
		s.textStarted = false
		s.textDone = false
		s.text.Reset()
		s.annotations = nil
	}
	if !s.textStarted {
		s.textStarted = true
		s.textID = s.nextTextID
		s.nextTextID++
		s.textOutputIndex = s.nextIndex(chatToResponsesOutputRef{Kind: "message", Segment: s.textSegment})
		return []ChatToResponsesStreamEvent{responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: intPtr(s.textOutputIndex),
			Item: &dto.ResponsesOutput{
				Type:    responsesOutputTypeMessage,
				ID:      s.messageID(),
				Status:  "in_progress",
				Role:    "assistant",
				Content: []dto.ResponsesOutputContent{},
			},
		})}
	}
	return nil
}

func (s *ChatToResponsesStreamState) appendReasoningDelta(delta string) []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if s.reasoningDone {
		s.reasoningSegment++
		s.reasoningStarted = false
		s.reasoningDone = false
		s.reasoning.Reset()
	}
	if !s.reasoningStarted {
		s.reasoningStarted = true
		s.chatToResponsesStreamChoice.reasoningID = s.nextReasoningID
		s.nextReasoningID++
		s.reasoningIndex = s.nextIndex(chatToResponsesOutputRef{Kind: "reasoning", Segment: s.reasoningSegment})
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: intPtr(s.reasoningIndex),
			Item: &dto.ResponsesOutput{
				Type:    responsesOutputTypeReasoning,
				ID:      s.reasoningID(),
				Status:  "in_progress",
				Content: []dto.ResponsesOutputContent{},
			},
		}))
	}
	s.reasoning.WriteString(delta)
	events = append(events, responsesStreamEvent(responsesEventReasoningSummaryDelta, dto.ResponsesStreamResponse{
		Type:         responsesEventReasoningSummaryDelta,
		OutputIndex:  intPtr(s.reasoningIndex),
		SummaryIndex: intPtr(0),
		Delta:        delta,
		ItemID:       s.reasoningID(),
	}))
	return events
}

func (s *ChatToResponsesStreamState) appendToolCallDelta(toolCall dto.ToolCallResponse) ([]ChatToResponsesStreamEvent, error) {
	chatIndex := 0
	if toolCall.Index != nil {
		chatIndex = *toolCall.Index
	}
	tool := s.toolsByIndex[chatIndex]
	events := make([]ChatToResponsesStreamEvent, 0, 2)
	if tool == nil {
		tool = &chatToResponsesStreamTool{
			ChatIndex:   chatIndex,
			OutputIndex: s.nextIndex(chatToResponsesOutputRef{Kind: "tool", ToolIndex: chatIndex}),
			ID:          strings.TrimSpace(toolCall.ID),
			Name:        strings.TrimSpace(toolCall.Function.Name),
		}
		if tool.ID == "" {
			idIndex := chatIndex
			if s.DeferChoiceFinish {
				idIndex = tool.OutputIndex
			}
			tool.ID = fmt.Sprintf("%s_call_%d", s.ID, idIndex)
		}

		tool.Custom = s.Tools.IsCustomTool(tool.Name)
		s.toolsByIndex[chatIndex] = tool
		events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{
			OutputIndex: intPtr(tool.OutputIndex), ItemID: tool.ID, Item: s.toolOutput(tool, "in_progress"),
		}))
	}
	if strings.TrimSpace(toolCall.ID) != "" {
		tool.ID = strings.TrimSpace(toolCall.ID)
	}
	if strings.TrimSpace(toolCall.Function.Name) != "" {
		tool.Name = strings.TrimSpace(toolCall.Function.Name)
	}
	if toolCall.Function.Arguments != "" {
		tool.Arguments.WriteString(toolCall.Function.Arguments)
		if !tool.Custom {
			events = append(events, responsesStreamEvent(responsesEventFunctionArgsDelta, dto.ResponsesStreamResponse{
				Type:        responsesEventFunctionArgsDelta,
				OutputIndex: intPtr(tool.OutputIndex),
				ItemID:      tool.ID,
				Delta:       toolCall.Function.Arguments,
			}))
		}
	}
	return events, nil
}

func (s *ChatToResponsesStreamState) doneDeltaEvents() []ChatToResponsesStreamEvent {
	if !s.DeferChoiceFinish {
		return s.doneChoiceEvents()
	}
	active := s.chatToResponsesStreamChoice
	var events []ChatToResponsesStreamEvent
	for _, choice := range s.choiceOrder {
		s.chatToResponsesStreamChoice = choice
		events = append(events, s.doneChoiceEvents()...)
	}
	s.chatToResponsesStreamChoice = active
	return events
}

func (s *ChatToResponsesStreamState) doneChoiceEvents() []ChatToResponsesStreamEvent {
	events := make([]ChatToResponsesStreamEvent, 0)
	status := s.outputStatus()
	if s.finishedStatus != "" {
		// A failed tail must not change the upstream finish of another
		// choice whose item.done was deferred only for late metadata.
		status = s.finishedStatus
	}
	if s.textStarted && !s.textDone {
		s.textDone = true
		message := s.messageOutput(status)
		s.closedMessages = append(s.closedMessages, *message)
		events = append(events, responsesStreamEvent("response.output_text.done", dto.ResponsesStreamResponse{
			Type:         "response.output_text.done",
			OutputIndex:  intPtr(s.textOutputIndex),
			ContentIndex: intPtr(0),
			ItemID:       s.messageID(),
		}))
		events = append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemDone,
			OutputIndex: intPtr(s.textOutputIndex),
			Item:        message,
		}))
	}
	if s.reasoningStarted && !s.reasoningDone {
		s.reasoningDone = true
		reasoning := s.reasoningOutput(status)
		s.closedReasonings = append(s.closedReasonings, *reasoning)
		events = append(events, responsesStreamEvent(responsesEventReasoningSummaryDone, dto.ResponsesStreamResponse{
			Type:         responsesEventReasoningSummaryDone,
			OutputIndex:  intPtr(s.reasoningIndex),
			SummaryIndex: intPtr(0),
			ItemID:       s.reasoningID(),
			Part: &dto.ResponsesReasoningSummaryPart{
				Type: "summary_text",
				Text: s.reasoning.String(),
			},
		}))
		events = append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemDone,
			OutputIndex: intPtr(s.reasoningIndex),
			Item:        reasoning,
		}))
	}
	for _, tool := range s.sortedTools() {
		if tool.DoneStatus != "" {
			continue
		}
		tool.DoneStatus = status

		if tool.Custom {
			input := customToolInputFromArguments(tool.Arguments.String())
			if input != "" {
				events = append(events, responsesStreamEvent("response.custom_tool_call_input.delta", dto.ResponsesStreamResponse{OutputIndex: intPtr(tool.OutputIndex), ItemID: tool.ID, Delta: input}))
			}
			events = append(events, responsesStreamEvent("response.custom_tool_call_input.done", dto.ResponsesStreamResponse{OutputIndex: intPtr(tool.OutputIndex), ItemID: tool.ID, Input: &input}))
		} else {
			events = append(events, responsesStreamEvent(responsesEventFunctionArgsDone, dto.ResponsesStreamResponse{
				Type:        responsesEventFunctionArgsDone,
				OutputIndex: intPtr(tool.OutputIndex),
				ItemID:      tool.ID,
			}))
		}
		events = append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemDone,
			OutputIndex: intPtr(tool.OutputIndex),
			Item:        s.toolOutput(tool, status),
		}))
	}
	return events
}

func (s *ChatToResponsesStreamState) applyFinishReason(finishReason string) {
	if status, details := ResponsesStatusFromChatFinishReason(finishReason); status != "" {
		s.status = status
		s.incompleteDetails = details
	}
}

func (s *ChatToResponsesStreamState) finalResponse() *dto.OpenAIResponsesResponse {
	output := make([]dto.ResponsesOutput, 0, len(s.outputOrder))
	// Text and reasoning were snapshotted by doneDeltaEvents. Keep their
	// emitted content and status even if later segments or a failure follow.
	for _, ref := range s.outputOrder {
		switch ref.Kind {
		case "message":
			output = append(output, ref.Choice.closedMessages[ref.Segment])
		case "reasoning":
			output = append(output, ref.Choice.closedReasonings[ref.Segment])
		case "hosted":
			output = append(output, s.hosted[ref.ToolIndex])
		case "tool":
			if tool := ref.Choice.toolsByIndex[ref.ToolIndex]; tool != nil {
				output = append(output, *s.toolOutput(tool, tool.DoneStatus))
			}
		}
	}
	return &dto.OpenAIResponsesResponse{
		ID:                s.ID,
		Object:            "response",
		CreatedAt:         int(s.Created),
		Status:            []byte(fmt.Sprintf("%q", s.status)),
		IncompleteDetails: s.incompleteDetails,
		Model:             s.Model,
		Output:            output,
		Usage:             s.Usage,
	}
}

func (s *ChatToResponsesStreamState) createdResponse() *dto.OpenAIResponsesResponse {
	return &dto.OpenAIResponsesResponse{
		ID:        s.ID,
		Object:    "response",
		CreatedAt: int(s.Created),
		Status:    []byte(`"in_progress"`),
		Model:     s.Model,
		Output:    []dto.ResponsesOutput{},
	}
}

func (s *ChatToResponsesStreamState) nextIndex(ref chatToResponsesOutputRef) int {
	index := s.nextOutputIndex
	s.nextOutputIndex++
	ref.Choice = s.chatToResponsesStreamChoice
	s.outputOrder = append(s.outputOrder, ref)
	return index
}

func (s *ChatToResponsesStreamState) sortedTools() []*chatToResponsesStreamTool {
	indexes := make([]int, 0, len(s.toolsByIndex))
	for index := range s.toolsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	tools := make([]*chatToResponsesStreamTool, 0, len(indexes))
	for _, index := range indexes {
		tools = append(tools, s.toolsByIndex[index])
	}
	return tools
}

func (s *ChatToResponsesStreamState) outputStatus() string {
	if s.status == "incomplete" || s.status == "failed" {
		return "incomplete"
	}
	return "completed"
}

func (s *ChatToResponsesStreamState) messageID() string {
	return fmt.Sprintf("%s_msg_%d", s.ID, s.textID)
}

func (s *ChatToResponsesStreamState) reasoningID() string {
	return fmt.Sprintf("%s_reasoning_%d", s.ID, s.chatToResponsesStreamChoice.reasoningID)
}

func (s *ChatToResponsesStreamState) messageOutput(status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type:   responsesOutputTypeMessage,
		ID:     s.messageID(),
		Status: status,
		Role:   "assistant",
		Content: []dto.ResponsesOutputContent{
			{
				Type:        "output_text",
				Text:        s.text.String(),
				Annotations: append([]any{}, s.annotations...),
			},
		},
	}
}

func (s *ChatToResponsesStreamState) reasoningOutput(status string) *dto.ResponsesOutput {
	return &dto.ResponsesOutput{
		Type:   responsesOutputTypeReasoning,
		ID:     s.reasoningID(),
		Status: status,
		Content: []dto.ResponsesOutputContent{
			{
				Type: "summary_text",
				Text: s.reasoning.String(),
			},
		},
	}
}

func (s *ChatToResponsesStreamState) toolOutput(tool *chatToResponsesStreamTool, status string) *dto.ResponsesOutput {
	namespace, name := s.Tools.ResponsesToolName(tool.Name)
	if tool.Custom {
		input := customToolInputFromArguments(tool.Arguments.String())
		return &dto.ResponsesOutput{Type: "custom_tool_call", ID: tool.ID, Status: status, CallId: tool.ID, Name: name, Namespace: namespace, Input: &input}
	}

	return &dto.ResponsesOutput{
		Type:      responsesOutputTypeFunctionCall,
		ID:        tool.ID,
		Status:    status,
		CallId:    tool.ID,
		Name:      name,
		Namespace: namespace,
		Arguments: chatArgumentsRawMessage(tool.Arguments.String()),
	}
}

// Emit one complete search item after Gemini's late grounding metadata arrives.
func (s *ChatToResponsesStreamState) AddWebSearch(queries []string) []ChatToResponsesStreamEvent {
	if s == nil || s.finalized || len(queries) == 0 {
		return nil
	}
	events := s.ensureCreated()
	item := WebSearchOutput(queries, fmt.Sprintf("ws_%s_%d", s.ID, len(s.hosted)))
	index := s.nextIndex(chatToResponsesOutputRef{Kind: "hosted", ToolIndex: len(s.hosted)})
	s.hosted = append(s.hosted, item)
	pending := item
	pending.Status = "in_progress"
	events = append(events, responsesStreamEvent(responsesEventOutputItemAdded, dto.ResponsesStreamResponse{OutputIndex: intPtr(index), Item: &pending}))
	for _, event := range []string{"response.web_search_call.in_progress", "response.web_search_call.searching", "response.web_search_call.completed"} {
		events = append(events, responsesStreamEvent(event, dto.ResponsesStreamResponse{OutputIndex: intPtr(index), ItemID: item.ID}))
	}
	return append(events, responsesStreamEvent(responsesEventOutputItemDone, dto.ResponsesStreamResponse{OutputIndex: intPtr(index), Item: &item}))
}
