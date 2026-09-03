package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const maxSplitImageN = 10

type splitImageAttemptResult struct {
	capture *imageResponseCapture
	payload map[string]any
	data    any
	usage   *dto.Usage
	apiErr  *types.NewAPIError
}

func shouldSplitImageNAfterError(c *gin.Context, relayMode int, info *relaycommon.RelayInfo, request *dto.ImageRequest, apiErr *types.NewAPIError) bool {
	if relayMode != relayconstant.RelayModeImagesGenerations || info == nil || request == nil || request.N == nil || apiErr == nil {
		return false
	}
	if info.IsStream || (info.IsAsyncImageRequest && !isLocalImageAsyncWorker(c)) || *request.N < 2 || *request.N > maxSplitImageN || apiErr.StatusCode != http.StatusBadRequest {
		return false
	}

	upstreamErr := apiErr.ToOpenAIError()
	if strings.EqualFold(strings.TrimSpace(upstreamErr.Param), "n") {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(upstreamErr.Type), "invalid_value") {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(upstreamErr.Message))
	return strings.Contains(message, "请求参数无法处理") ||
		strings.Contains(message, "number of images") ||
		strings.Contains(message, "image count") ||
		strings.Contains(message, "parameter n") ||
		strings.Contains(message, "n must be")
}

func submitSplitImageRequests(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request *dto.ImageRequest,
	writer gin.ResponseWriter,
	statusCodeMapping string,
) (*imageResponseCapture, *dto.Usage, *types.NewAPIError) {
	if request == nil || request.N == nil || *request.N < 2 || *request.N > maxSplitImageN {
		return nil, nil, types.NewErrorWithStatusCode(
			fmt.Errorf("image split count must be between 2 and %d", maxSplitImageN),
			types.ErrorCodeInvalidRequest,
			http.StatusBadRequest,
			types.ErrOptionWithSkipRetry(),
		)
	}

	requestedCount := int(*request.N)
	combined := make(map[string]any)
	combinedData := make([]any, 0, requestedCount)
	combinedUsageJSON := make(map[string]any)
	totalUsage := &dto.Usage{}
	var combinedCapture *imageResponseCapture
	var lastError *types.NewAPIError

	results := make([]splitImageAttemptResult, requestedCount)
	var waitGroup sync.WaitGroup
	waitGroup.Add(requestedCount)
	// Each attempt owns its context, relay state, adaptor, and capture; indexed results preserve request order.
	for index := 0; index < requestedCount; index++ {
		go func(index int) {
			defer waitGroup.Done()
			results[index] = submitSingleSplitImageRequest(c, info, request, writer, statusCodeMapping)
		}(index)
	}
	waitGroup.Wait()

	for index, result := range results {
		if result.apiErr != nil {
			lastError = result.apiErr
			logSplitImageFailure(c, index, requestedCount, lastError)
			continue
		}
		if combinedCapture == nil {
			combinedCapture = newImageResponseCapture(writer)
			for key, values := range result.capture.Header() {
				combinedCapture.Header()[key] = append([]string(nil), values...)
			}
			for key, value := range result.payload {
				if key != "data" && key != "usage" {
					combined[key] = value
				}
			}
		}
		combinedData = append(combinedData, result.data)
		mergeImageUsage(totalUsage, result.usage)
		if rawUsage, ok := result.payload["usage"].(map[string]any); ok {
			mergeNumericJSONMap(combinedUsageJSON, rawUsage)
		}
	}
	if combinedCapture == nil {
		if lastError == nil {
			lastError = newImageSubmitUncertainError(fmt.Errorf("all split image submissions failed"), types.ErrorCodeBadResponse, http.StatusBadGateway)
		}
		return nil, nil, lastError
	}

	applySplitImageSuccessCount(c, info, request, len(combinedData))
	combined["data"] = combinedData
	if len(combinedUsageJSON) > 0 {
		combined["usage"] = combinedUsageJSON
	}
	body, err := common.Marshal(combined)
	if err != nil {
		return nil, nil, newImageSubmitUncertainError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	combinedCapture.ReplaceBody(http.StatusOK, body)
	logger.LogInfo(c, fmt.Sprintf("assembled %d successful images from %d single-image submissions", len(combinedData), requestedCount))
	return combinedCapture, totalUsage, nil
}

func submitSingleSplitImageRequest(
	c *gin.Context,
	info *relaycommon.RelayInfo,
	request *dto.ImageRequest,
	writer gin.ResponseWriter,
	statusCodeMapping string,
) splitImageAttemptResult {
	attemptContext := cloneSplitImageContext(c)
	attemptInfo := cloneSplitImageRelayInfo(info)
	attemptRequest, err := common.DeepCopy(request)
	if err != nil {
		return splitImageAttemptResult{apiErr: types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())}
	}
	attemptRequest.N = common.GetPointer(uint(1))
	attemptInfo.Request = attemptRequest

	attemptAdaptor := GetAdaptor(attemptInfo.ApiType)
	if attemptAdaptor == nil {
		return splitImageAttemptResult{apiErr: types.NewError(fmt.Errorf("invalid api type: %d", attemptInfo.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())}
	}
	attemptAdaptor.Init(attemptInfo)

	requestBody, closer, apiErr := buildSplitImageRequestBody(attemptContext, attemptInfo, attemptAdaptor, attemptRequest)
	if apiErr != nil {
		return splitImageAttemptResult{apiErr: apiErr}
	}
	respValue, err := attemptAdaptor.DoRequest(attemptContext, attemptInfo, requestBody)
	if closer != nil {
		_ = closer.Close()
	}
	if err != nil {
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)}
	}
	resp, ok := respValue.(*http.Response)
	if !ok || resp == nil {
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(fmt.Errorf("image split response is missing"), types.ErrorCodeBadResponse, http.StatusBadGateway)}
	}
	if resp.StatusCode == http.StatusCreated && attemptInfo.ApiType == constant.APITypeReplicate {
		resp.StatusCode = http.StatusOK
	}
	if resp.StatusCode != http.StatusOK {
		apiErr = service.RelayErrorHandler(attemptContext.Request.Context(), resp, false)
		service.ResetStatusCode(apiErr, statusCodeMapping)
		return splitImageAttemptResult{apiErr: apiErr}
	}
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		service.CloseResponseBodyGracefully(resp)
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(fmt.Errorf("streaming image response cannot be aggregated"), types.ErrorCodeBadResponse, http.StatusBadGateway)}
	}

	capture := newImageResponseCapture(writer)
	attemptContext.Writer = capture
	usageValue, responseErr := attemptAdaptor.DoResponse(attemptContext, resp, attemptInfo)
	if responseErr != nil {
		service.ResetStatusCode(responseErr, statusCodeMapping)
		return splitImageAttemptResult{apiErr: markImageSubmitUncertainError(responseErr)}
	}
	usage, ok := usageValue.(*dto.Usage)
	if !ok || usage == nil {
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(fmt.Errorf("invalid image split usage type: %T", usageValue), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)}
	}
	if capture.HasBody() {
		syncBody, syncErr := waitImageAsyncSubmitResponse(attemptContext, attemptInfo, capture.BodyBytes())
		if syncErr != nil {
			return splitImageAttemptResult{apiErr: syncErr}
		}
		if len(syncBody) > 0 {
			capture.ReplaceBody(http.StatusOK, syncBody)
		}
	}
	if !capture.HasBody() {
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(fmt.Errorf("image split response body is empty"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)}
	}

	var payload map[string]any
	if err := common.Unmarshal(capture.BodyBytes(), &payload); err != nil {
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(fmt.Errorf("parse image split response: %w", err), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)}
	}
	data, ok := payload["data"].([]any)
	if !ok || len(data) == 0 {
		return splitImageAttemptResult{apiErr: newImageSubmitUncertainError(fmt.Errorf("image split response has no data"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)}
	}
	return splitImageAttemptResult{capture: capture, payload: payload, data: data[0], usage: usage}
}

func cloneSplitImageContext(c *gin.Context) *gin.Context {
	cloned := c.Copy()
	if c.Request != nil {
		cloned.Request = c.Request.Clone(c.Request.Context())
	}
	return cloned
}

func cloneSplitImageRelayInfo(info *relaycommon.RelayInfo) *relaycommon.RelayInfo {
	cloned := *info
	cloned.IsStream = false
	cloned.IsAsyncImageRequest = false
	cloned.UpstreamRequestBodySize = 0
	cloned.PriceData.OtherRatios = cloneFloatRatios(info.PriceData.OtherRatios)
	cloned.RequestHeaders = cloneStringMap(info.RequestHeaders)
	cloned.RuntimeHeadersOverride = cloneAnyMap(info.RuntimeHeadersOverride)
	cloned.ParamOverrideAudit = append([]string(nil), info.ParamOverrideAudit...)
	cloned.RequestConversionChain = append([]types.RelayFormat(nil), info.RequestConversionChain...)
	cloned.BillingRequestInput = cloneImageAsyncBillingRequestInput(info.BillingRequestInput)
	if info.ChannelMeta != nil {
		channelMeta := *info.ChannelMeta
		channelMeta.ParamOverride = cloneAnyMap(info.ChannelMeta.ParamOverride)
		channelMeta.HeadersOverride = cloneAnyMap(info.ChannelMeta.HeadersOverride)
		cloned.ChannelMeta = &channelMeta
	}
	return &cloned
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneAnyMap(source map[string]any) map[string]any {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func logSplitImageFailure(c *gin.Context, index, requestedCount int, apiErr *types.NewAPIError) {
	message := "unknown error"
	if apiErr != nil {
		message = apiErr.MaskSensitiveErrorWithStatusCode()
	}
	logger.LogWarn(c, fmt.Sprintf("skipping failed split image submission %d/%d: %s", index+1, requestedCount, message))
}

func applySplitImageSuccessCount(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ImageRequest, successCount int) {
	if info == nil || request == nil || successCount < 1 {
		return
	}
	request.N = common.GetPointer(uint(successCount))
	// Token-based usage is already summed per successful call; only per-call pricing needs an n multiplier.
	if info.PriceData.UsePrice {
		info.PriceData.AddOtherRatio("n", float64(successCount))
	} else {
		delete(info.PriceData.OtherRatios, "n")
	}
	if info.BillingRequestInput == nil || len(info.BillingRequestInput.Body) == 0 {
		return
	}
	var billingRequest map[string]any
	if err := common.Unmarshal(info.BillingRequestInput.Body, &billingRequest); err != nil {
		logger.LogWarn(c, fmt.Sprintf("failed to update split image billing request count: %s", err.Error()))
		return
	}
	billingRequest["n"] = successCount
	body, err := common.Marshal(billingRequest)
	if err != nil {
		logger.LogWarn(c, fmt.Sprintf("failed to marshal split image billing request count: %s", err.Error()))
		return
	}
	info.BillingRequestInput.Body = body
}

func buildSplitImageRequestBody(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.ImageRequest) (io.Reader, io.Closer, *types.NewAPIError) {
	convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed)
	}
	if buffer, ok := convertedRequest.(*bytes.Buffer); ok {
		return buffer, nil, nil
	}

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if _, ok := convertedRequest.(dto.ImageRequest); ok && len(request.Extra) > 0 {
		var payload map[string]any
		if err := common.Unmarshal(jsonData, &payload); err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
		for key, raw := range request.Extra {
			if _, exists := payload[key]; exists || len(raw) == 0 {
				continue
			}
			var value any
			if err := common.Unmarshal(raw, &value); err == nil {
				payload[key] = value
			}
		}
		payload["n"] = 1
		jsonData, err = common.Marshal(payload)
		if err != nil {
			return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
		}
	}
	if len(info.ParamOverride) > 0 {
		jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
		if err != nil {
			return nil, nil, newAPIErrorFromParamOverride(err)
		}
	}
	logger.LogDebug(c, "split image request body: %s", jsonData)
	body, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	info.UpstreamRequestBodySize = body.Size()
	return body, closer, nil
}

func cloneFloatRatios(source map[string]float64) map[string]float64 {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]float64, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func mergeImageUsage(total *dto.Usage, usage *dto.Usage) {
	if total == nil || usage == nil {
		return
	}
	total.PromptTokens += usage.PromptTokens
	total.CompletionTokens += usage.CompletionTokens
	total.TotalTokens += usage.TotalTokens
	total.PromptCacheHitTokens += usage.PromptCacheHitTokens
	total.InputTokens += usage.InputTokens
	total.OutputTokens += usage.OutputTokens
	total.ClaudeCacheCreation5mTokens += usage.ClaudeCacheCreation5mTokens
	total.ClaudeCacheCreation1hTokens += usage.ClaudeCacheCreation1hTokens
	mergeInputTokenDetails(&total.PromptTokensDetails, usage.PromptTokensDetails)
	mergeOutputTokenDetails(&total.CompletionTokenDetails, usage.CompletionTokenDetails)
	if usage.InputTokensDetails != nil {
		if total.InputTokensDetails == nil {
			total.InputTokensDetails = &dto.InputTokenDetails{}
		}
		mergeInputTokenDetails(total.InputTokensDetails, *usage.InputTokensDetails)
	}
	if usage.OutputTokensDetails != nil {
		if total.OutputTokensDetails == nil {
			total.OutputTokensDetails = &dto.OutputTokenDetails{}
		}
		mergeOutputTokenDetails(total.OutputTokensDetails, *usage.OutputTokensDetails)
	}
	if total.UsageSemantic == "" {
		total.UsageSemantic = usage.UsageSemantic
	}
	if total.UsageSource == "" {
		total.UsageSource = usage.UsageSource
	}
}

func mergeInputTokenDetails(total *dto.InputTokenDetails, details dto.InputTokenDetails) {
	total.CachedTokens += details.CachedTokens
	total.CachedCreationTokens += details.CachedCreationTokens
	total.CacheWriteTokens += details.CacheWriteTokens
	total.TextTokens += details.TextTokens
	total.AudioTokens += details.AudioTokens
	total.ImageTokens += details.ImageTokens
}

func mergeOutputTokenDetails(total *dto.OutputTokenDetails, details dto.OutputTokenDetails) {
	total.TextTokens += details.TextTokens
	total.AudioTokens += details.AudioTokens
	total.ImageTokens += details.ImageTokens
	total.ReasoningTokens += details.ReasoningTokens
	total.AcceptedPredictionTokens += details.AcceptedPredictionTokens
	total.RejectedPredictionTokens += details.RejectedPredictionTokens
}

func mergeNumericJSONMap(total map[string]any, values map[string]any) {
	for key, value := range values {
		switch typed := value.(type) {
		case float64:
			if existing, ok := total[key].(float64); ok {
				total[key] = existing + typed
			} else {
				total[key] = typed
			}
		case map[string]any:
			nested, ok := total[key].(map[string]any)
			if !ok {
				nested = make(map[string]any)
				total[key] = nested
			}
			mergeNumericJSONMap(nested, typed)
		default:
			if _, exists := total[key]; !exists {
				total[key] = value
			}
		}
	}
}
