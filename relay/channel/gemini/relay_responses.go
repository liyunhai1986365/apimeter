package gemini

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GeminiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	logger.LogDebug(c, "Gemini responses response body: %s", responseBody)

	var geminiResponse dto.GeminiChatResponse
	if err := common.Unmarshal(responseBody, &geminiResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	var grounding geminiGroundingCounter
	grounding.observe(&geminiResponse)
	grounding.commit(info)
	if len(geminiResponse.Candidates) == 0 {
		usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
		if geminiResponse.PromptFeedback != nil && geminiResponse.PromptFeedback.BlockReason != nil {
			common.SetContextKey(c, constant.ContextKeyAdminRejectReason, fmt.Sprintf("gemini_block_reason=%s", *geminiResponse.PromptFeedback.BlockReason))
			return &usage, types.NewOpenAIError(
				errors.New("request blocked by Gemini API: "+*geminiResponse.PromptFeedback.BlockReason),
				types.ErrorCodePromptBlocked,
				http.StatusBadRequest,
			)
		}
		common.SetContextKey(c, constant.ContextKeyAdminRejectReason, "gemini_empty_candidates")
		return &usage, types.NewOpenAIError(
			errors.New("empty response from Gemini API"),
			types.ErrorCodeEmptyResponse,
			http.StatusInternalServerError,
		)
	}

	chatResp := responseGeminiChat2OpenAI(c, &geminiResponse)
	chatResp.Model = info.UpstreamModelName
	if responseID := helper.GetResponseID(c); responseID != "" {
		chatResp.Id = responseID
	}
	usage := buildUsageFromGeminiResponse(c, info, &geminiResponse)
	chatResp.Usage = usage

	convertResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAIResponses, chatResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responsesResp, ok := convertResult.Value.(*dto.OpenAIResponsesResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI responses response, got %T", convertResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	responsesUsage := convertResult.Usage
	if responsesUsage == nil || responsesUsage.TotalTokens == 0 {
		responsesResp.Usage = relayconvert.UsageFromChatUsage(&usage)
	}

	relayconvert.AddGeminiSearchToResponses(responsesResp, &geminiResponse)
	responseBody, err = common.Marshal(responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return &usage, nil
}

func GeminiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (resultUsage *dto.Usage, resultErr *types.NewAPIError) {
	defer func() { resultErr = relaycommon.WithStreamUsage(resultErr, resultUsage) }()
	responseID := helper.GetResponseID(c)
	created := common.GetTimestamp()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, relayconvert.ResponseStreamOptions{
		ID:                responseID,
		Model:             info.UpstreamModelName,
		Created:           created,
		DeferChoiceFinish: true,
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	geminiState := relayconvert.NewGeminiToChatStreamState(responseID, created)
	var streamErr *types.NewAPIError
	downstreamFailed := false

	sendEvent := func(event relayconvert.ChatToResponsesStreamEvent) bool {
		data, err := common.Marshal(event.Payload)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		if err := c.Request.Context().Err(); err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
			return false
		}
		helper.ExtendWriteDeadline(c)
		// Write directly so a failed terminal event is not marked as delivered:
		// CustomEvent rendering currently discards response-writer errors.
		_, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
		if err == nil {
			err = helper.FlushWriter(c)
		}
		if err != nil {
			downstreamFailed = true
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
			return false
		}
		return true
	}
	defer func() {
		if resultErr == nil {
			return
		}
		common.SetContextKey(c, constant.ContextKeyRelayFailed, true)
		if downstreamFailed || c.Request.Context().Err() != nil {
			return
		}
		state.SetUsage(resultUsage)
		results, handled := state.FailResponsesStream("server_error", resultErr.ToOpenAIError().Message)
		if !handled {
			return
		}
		for _, result := range results {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok || !sendEvent(event) {
				return
			}
			if event.Type == "response.failed" {
				// Keep the non-nil error for settlement and failure accounting;
				// the controller must not append a second terminal error.
				common.SetContextKey(c, constant.ContextKeyRelayStreamErrorSent, true)
			}
		}
	}()
	sendChunk := func(chunk *dto.ChatCompletionsStreamResponse) bool {
		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, chunk)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
		for _, result := range results {
			event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
			if !ok {
				streamErr = types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			if !sendEvent(event) {
				return false
			}
		}
		return true
	}

	usage, streamAPIError := geminiStreamHandler(c, info, resp, func(data string, geminiResponse *dto.GeminiChatResponse) bool {
		chunks, err := geminiState.ConvertChunk(geminiResponse, info.UpstreamModelName, nil)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
			return false
		}
		for _, chunk := range chunks {
			if !sendChunk(chunk) {
				return false
			}
		}
		return true
	}, state.ObserveGeminiSearch)
	if streamAPIError != nil {
		return usage, streamAPIError
	}
	if streamErr != nil {
		return usage, streamErr
	}
	if streamErr := geminiStreamFinalizationError(info); streamErr != nil {
		return usage, streamErr
	}

	chunks, err := geminiState.Finalize(info.UpstreamModelName)
	if err != nil {
		return usage, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusBadGateway)
	}
	for _, chunk := range chunks {
		if !sendChunk(chunk) {
			return usage, streamErr
		}
	}

	if usage != nil {
		state.SetUsage(usage)
	}
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return usage, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		event, ok := result.Value.(relayconvert.ChatToResponsesStreamEvent)
		if !ok {
			return usage, types.NewOpenAIError(fmt.Errorf("expected OAI responses stream event, got %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if !sendEvent(event) {
			return usage, streamErr
		}
	}
	return usage, nil
}
