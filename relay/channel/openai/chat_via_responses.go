package openai

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	var status string
	_ = common.Unmarshal(responsesResp.Status, &status)
	if status == "failed" || status == "cancelled" {
		oaiError := responsesResp.GetOpenAIError()
		if oaiError == nil {
			oaiError = &types.OpenAIError{}
		}
		if oaiError.Message == "" {
			oaiError.Message = "upstream response " + status
		}
		if oaiError.Type == "" {
			oaiError.Type = "upstream_error"
		}
		return failedResponsesUsage(responsesResp.Usage, types.WithOpenAIError(*oaiError, http.StatusBadGateway))
	}
	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return failedResponsesUsage(responsesResp.Usage, types.WithOpenAIError(*oaiError, resp.StatusCode))
	}

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	if info.IsStream && info.RelayFormat == types.RelayFormatClaude {
		if err := writeClaudeResponsesJSONStream(c, info, &responsesResp, chatResp, usage); err != nil {
			return nil, err
		}
		return usage, nil
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

// A Responses upstream may return JSON even when the client requested a
// stream. Keep Claude's SSE contract, including any preparation pings, while
// retaining the JSON path's usage calculation for settlement.
func writeClaudeResponsesJSONStream(c *gin.Context, info *relaycommon.RelayInfo, response *dto.OpenAIResponsesResponse, chatResponse *dto.OpenAITextResponse, usage *dto.Usage) *types.NewAPIError {
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, types.RelayFormatClaude, relayconvert.ResponseStreamOptions{
		ID: chatResponse.Id, Model: chatResponse.Model, Created: int64(response.CreatedAt),
	})
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	// The JSON converter collects every output message. Supplying its complete
	// text first also prevents terminal-event recovery from stopping at the
	// first message in a response containing several output messages.
	var events []dto.ResponsesStreamResponse
	if len(chatResponse.Choices) > 0 {
		message := chatResponse.Choices[0].Message
		if message.ReasoningContent != nil && *message.ReasoningContent != "" {
			events = append(events, dto.ResponsesStreamResponse{Type: "response.reasoning_text.delta", Delta: *message.ReasoningContent})
		}
		if text := message.StringContent(); text != "" {
			events = append(events, dto.ResponsesStreamResponse{Type: "response.output_text.delta", Delta: text})
		}
	}
	terminalType := "response.completed"
	var status string
	_ = common.Unmarshal(response.Status, &status)
	if strings.TrimSpace(status) == "incomplete" {
		terminalType = "response.incomplete"
	}
	events = append(events, dto.ResponsesStreamResponse{Type: terminalType, Response: response})

	var results []relayconvert.ResponseResult
	for i := range events {
		converted, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &events[i])
		if err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		results = append(results, converted...)
	}
	state.SetUsage(usage)
	info.EnsureClaudeConvertInfo().Usage = usage
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	results = append(results, finalResults...)

	claudeEvents := make([]dto.ClaudeResponse, 0, len(results))
	for _, result := range results {
		switch value := result.Value.(type) {
		case dto.ClaudeResponse:
			claudeEvents = append(claudeEvents, value)
		case *dto.ClaudeResponse:
			if value != nil {
				claudeEvents = append(claudeEvents, *value)
			}
		default:
			return types.NewOpenAIError(fmt.Errorf("expected Claude stream response, got %T", result.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}
	helper.StopStreamKeepAlive(c)
	helper.SetEventStreamHeaders(c)
	info.SetFirstResponseTime()
	for _, event := range claudeEvents {
		if err := helper.ClaudeData(c, event); err != nil {
			return types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}
	return nil
}

// Responses events carry cumulative usage. On failure only settle an explicit
// upstream snapshot; never estimate tokens from partially delivered text.
type responsesStreamOutcome struct {
	reported *dto.Usage
	terminal *dto.OpenAIResponsesResponse
}

func (s *responsesStreamOutcome) observe(event *dto.ResponsesStreamResponse) {
	if event.Response != nil && event.Response.Usage != nil {
		s.reported = event.Response.Usage
	}
	switch event.Type {
	case "response.completed", "response.done", "response.incomplete":
		if event.Response == nil {
			event.Response = &dto.OpenAIResponsesResponse{}
		}
		if event.Type == "response.incomplete" && len(event.Response.Status) == 0 {
			event.Response.Status = []byte(`"incomplete"`)
		}
		if event.Response.Usage == nil {
			event.Response.Usage = s.reported
		}
		s.terminal = event.Response
	}
}

func failedResponsesUsage(reported *dto.Usage, err *types.NewAPIError) (*dto.Usage, *types.NewAPIError) {
	if reported == nil {
		return nil, err
	}
	usage := relayconvert.UsageFromResponsesUsage(reported)
	return usage, relaycommon.WithStreamUsage(err, usage)
}

func (s *responsesStreamOutcome) failure(err *types.NewAPIError) (*dto.Usage, *types.NewAPIError) {
	return failedResponsesUsage(s.reported, err)
}

func (s *responsesStreamOutcome) missingTerminal() (*dto.Usage, *types.NewAPIError) {
	return s.failure(types.NewOpenAIError(fmt.Errorf("upstream Responses stream ended without a terminal event"), types.ErrorCodeBadResponse, http.StatusBadGateway))
}

func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	accumulator := relayconvert.NewResponsesBufferedAccumulator()
	var reported responsesStreamOutcome

	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:5] != "data:" {
			continue
		}
		data := line[5:]
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal buffered responses stream event: "+err.Error())
			return reported.failure(types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError))
		}
		reported.observe(&streamResp)
		if failure := streamResp.StreamError(); failure != nil {
			return reported.failure(types.WithOpenAIError(*failure, http.StatusBadGateway))
		}
		accumulator.ProcessEvent(&streamResp)
		if reported.terminal != nil {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return reported.failure(types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError))
	}
	finalResponse := reported.terminal
	if finalResponse == nil {
		return reported.missingTerminal()
	}
	accumulator.SupplementResponseOutput(finalResponse)

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, finalResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(finalResponse)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:      responseId,
		Model:   info.UpstreamModelName,
		Created: createAt,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	var reported responsesStreamOutcome

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendGeminiResponse := func(geminiResponse *dto.GeminiChatResponse) bool {
		if geminiResponse == nil {
			return true
		}
		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
		_ = helper.FlushWriter(c)
		return true
	}

	sendStreamResult := func(result relayconvert.ResponseResult) bool {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) == 0 && value.Usage == nil {
				return true
			}
			if err := helper.ObjectData(c, &value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ChatCompletionsStreamResponse:
			if value == nil || (len(value.Choices) == 0 && value.Usage == nil) {
				return true
			}
			if err := helper.ObjectData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ClaudeResponse:
			if value == nil {
				return true
			}
			if err := helper.ClaudeData(c, *value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.GeminiChatResponse:
			return sendGeminiResponse(&value)
		case *dto.GeminiChatResponse:
			return sendGeminiResponse(value)
		default:
			streamErr = types.NewOpenAIError(fmt.Errorf("unsupported converted stream response type %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			sr.Error(err)
			return
		}

		reported.observe(&streamResp)
		if failure := streamResp.StreamError(); failure != nil {
			streamErr = types.WithOpenAIError(*failure, http.StatusBadGateway)
			sr.Stop(streamErr)
			return
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &streamResp)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if !sendStreamResult(result) {
				sr.Stop(streamErr)
				return
			}
		}
		if reported.terminal != nil {
			sr.Done()
		}
	})

	if streamErr != nil {
		return reported.failure(streamErr)
	}
	if reported.terminal == nil {
		return reported.missingTerminal()
	}

	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return reported.failure(types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError))
	}
	for _, result := range finalResults {
		if !sendStreamResult(result) {
			return reported.failure(streamErr)
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, info.UpstreamModelName, *usage)); err != nil {
			return reported.failure(types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError))
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}
