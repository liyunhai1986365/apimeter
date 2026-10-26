package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// OpenaiImageHandler handles non-streaming OpenAI image responses
// (generations/edits), returning the parsed usage for billing.
func OpenaiImageHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	usage, apiErr := OpenaiImageResponseUsage(info, responseBody, resp.StatusCode)
	if apiErr != nil {
		return nil, apiErr
	}
	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

// OpenaiImageResponseUsage validates image/task JSON and updates the final image
// count and usage. Task completion must replace submission usage before billing.
func OpenaiImageResponseUsage(info *relaycommon.RelayInfo, responseBody []byte, statusCode int) (*dto.Usage, *types.NewAPIError) {
	var usageResp dto.SimpleResponse
	err := common.Unmarshal(responseBody, &usageResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if apiErr := openaiImageResponseError(&usageResp, statusCode); apiErr != nil {
		return nil, apiErr
	}

	imageCount := relaycommon.ImageResponseCount(responseBody)
	taskID := gjson.GetBytes(responseBody, "id")
	if imageCount == 0 && (taskID.Type != gjson.String || strings.TrimSpace(taskID.String()) == "") {
		return nil, imageStreamError(fmt.Errorf("image response contains no completed images"))
	}
	info.UpdateImageCount(imageCount)

	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)
	return &usageResp.Usage, nil
}

// openaiImageDataHasField reports whether an image data entry carries a
// non-empty string value for field.
func openaiImageDataHasField(item gjson.Result, field string) bool {
	value := item.Get(field)
	return value.Type == gjson.String && strings.TrimSpace(value.String()) != ""
}

// Match the non-streaming relay's Base64 aliases and first-nonempty precedence.
func openaiImageBase64(item gjson.Result) string {
	for _, field := range []string{"b64_json", "base64", "image_base64", "imageBase64"} {
		value := item.Get(field)
		if value.Type == gjson.String {
			if encoded := strings.TrimSpace(value.String()); encoded != "" {
				return encoded
			}
		}
	}
	return ""
}

// normalizeOpenAIUsage maps the OpenAI Images usage shape (input_tokens /
// output_tokens / input_tokens_details / output_tokens_details) onto the
// canonical prompt/completion fields. It is used only on the OpenAI image
// relay paths (generations/edits, streaming and non-streaming): the image
// API never returns prompt_tokens / completion_tokens, so the overwrite (=)
// semantics here are equivalent to the previous additive (+=) behavior while
// avoiding any future double-counting if both field sets are ever populated.
// Do not reuse this on chat/embedding paths without revisiting the overwrite
// semantics.
func normalizeOpenAIUsage(usage *dto.Usage) {
	if usage == nil {
		return
	}
	if usage.InputTokens != 0 {
		usage.PromptTokens = usage.InputTokens
	}
	if usage.OutputTokens != 0 {
		usage.CompletionTokens = usage.OutputTokens
	}
	if usage.InputTokensDetails != nil {
		usage.PromptTokensDetails = *usage.InputTokensDetails
	}
	if usage.OutputTokensDetails != nil {
		usage.CompletionTokenDetails = *usage.OutputTokensDetails
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
}

// OpenaiImageStreamHandler follows the upstream new-api image stream handler
// (4013729f6a2a14b9b5988d3c1a529ea97d2b2d8d), retaining the local safeguard
// against charging failed streams that never produced a completed image.
func OpenaiImageStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, imageStreamError(fmt.Errorf("invalid image response"))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return OpenaiImageHandler(c, info, resp)
	}
	info.IsStream = true
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return openaiImageJSONAsStreamHandler(c, info, resp)
	}

	usage := &dto.Usage{}
	var lastUsageData []byte
	var completedImages int64
	var billingImages []json.RawMessage
	var apiErr *types.NewAPIError
	errorForwarded, writeFailed := false, false

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		raw := common.StringToByteSlice(data)
		var chunk struct {
			Type    string    `json:"type"`
			Usage   dto.Usage `json:"usage"`
			Error   any       `json:"error"`
			Message string    `json:"message"`
			Code    any       `json:"code"`
		}
		if err := common.Unmarshal(raw, &chunk); err != nil {
			apiErr = imageStreamError(fmt.Errorf("invalid image stream event: %w", err))
			sr.Stop(apiErr)
			return
		}
		if strings.ContainsAny(chunk.Type, "\r\n") {
			apiErr = imageStreamError(fmt.Errorf("image stream event has an invalid type"))
			sr.Stop(apiErr)
			return
		}
		upstreamError := imageStreamUpstreamError(chunk.Error)
		eventType := strings.ToLower(strings.TrimSpace(chunk.Type))
		isError := upstreamError != nil || eventType == "error" || eventType == "upstream_error" || strings.HasSuffix(eventType, ".failed")
		if isError {
			if upstreamError == nil {
				upstreamError = &types.OpenAIError{Type: "upstream_error", Message: chunk.Message, Code: chunk.Code}
			}
			if upstreamError.Message == "" {
				upstreamError.Message = "image generation failed"
			}
			apiErr = types.WithOpenAIError(*upstreamError, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
			sr.Error(apiErr)
		}

		normalizeOpenAIUsage(&chunk.Usage)
		if service.ValidUsage(&chunk.Usage) || chunk.Usage.TotalTokens > 0 {
			usage = &chunk.Usage
			lastUsageData = raw
		}
		if !isError && (chunk.Type == "image_generation.completed" || chunk.Type == "image_edit.completed") {
			image := gjson.ParseBytes(raw)
			if !openaiImageDataHasField(image, "url") && openaiImageBase64(image) == "" {
				apiErr = imageStreamError(fmt.Errorf("image stream completed without an image"))
				sr.Stop(apiErr)
				return
			}
			normalizeContext := c.Copy()
			normalizeContext.Request = c.Request.Clone(sr.Context())
			normalized, err := normalizeOpenaiStreamImage(normalizeContext, info, raw)
			if err != nil {
				apiErr = imageStreamError(fmt.Errorf("normalize completed image: %w", err))
				sr.Stop(apiErr)
				return
			}
			raw = normalized
			completedImages++
			// Only expression billing needs the completed response payloads.
			// Previews and errors must not become billable data[] entries.
			if info.TieredBillingSnapshot != nil {
				billingImages = append(billingImages, append(json.RawMessage(nil), raw...))
			}
		}
		if err := writeOpenaiImageStreamChunk(c, raw); err != nil {
			writeFailed = true
			sr.Stop(err)
		} else if isError {
			errorForwarded = true
		}
	})

	if completedImages == 0 && apiErr == nil {
		apiErr = imageStreamError(fmt.Errorf("image stream ended before a completed image: %s", info.StreamStatus.Summary()))
	}
	if apiErr == nil {
		switch info.StreamStatus.EndReason {
		case relaycommon.StreamEndReasonScannerErr, relaycommon.StreamEndReasonTimeout, relaycommon.StreamEndReasonPanic:
			apiErr = imageStreamError(fmt.Errorf("image stream interrupted: %s", info.StreamStatus.Summary()))
		}
	}
	if apiErr != nil && !errorForwarded && !writeFailed && c.Request.Context().Err() == nil {
		body, err := common.Marshal(map[string]any{"type": "error", "error": apiErr.ToOpenAIError()})
		if err == nil {
			_ = writeOpenaiImageStreamChunk(c, body)
		}
	}
	if completedImages == 0 {
		// Preview/error/usage-only chunks are not a successful image result.
		return nil, apiErr
	}

	// Keep the last valid usage, including usage-only chunks after completion.
	// A partial success still consumed upstream resources and must settle.
	applyUsagePostProcessing(info, usage, lastUsageData)
	// A failed client write may race with the scanner reaching EOF/[DONE].
	// Reserve the requested quantity only for client interruptions. Upstream
	// failures and timeouts must settle only the images actually completed.
	clientInterrupted := writeFailed || c.Request.Context().Err() != nil ||
		info.StreamStatus.EndReason == relaycommon.StreamEndReasonClientGone ||
		info.StreamStatus.EndReason == relaycommon.StreamEndReasonPingFail
	if clientInterrupted {
		info.UpdateImageCount(max(completedImages, int64(info.RequestedImageCount())))
	} else {
		info.UpdateImageCount(completedImages)
	}
	if len(billingImages) > 0 {
		// Expose data[] just like a non-streaming image response, and retain
		// the final completion's top-level metadata for native SSE pricing.
		var payload map[string]json.RawMessage
		_ = common.Unmarshal(billingImages[len(billingImages)-1], &payload)
		payload["data"], _ = common.Marshal(billingImages)
		payload["usage"], _ = common.Marshal(usage)
		body, _ := common.Marshal(payload)
		info.SetImageBillingResponse(body)
	}
	if info.StreamStatus.EndReason == relaycommon.StreamEndReasonDone && !writeFailed && c.Request.Context().Err() == nil {
		_ = writeOpenaiImageStreamDone(c)
	}
	return usage, nil
}

// writeOpenaiImageStreamChunk rebuilds the SSE frame for an image stream chunk:
// it emits an "event:" line derived from the JSON "type" field (when present)
// followed by the verbatim "data:" payload, mirroring helper.ResponseChunkData.
func writeOpenaiImageStreamChunk(c *gin.Context, data []byte) error {
	var payload struct {
		Type string `json:"type"`
	}
	_ = common.Unmarshal(data, &payload)
	if eventName := strings.TrimSpace(payload.Type); eventName != "" {
		return helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventName}, string(data))
	}
	return helper.StringData(c, string(data))
}

func imageStreamError(err error) *types.NewAPIError {
	return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusBadGateway, types.ErrOptionWithSkipRetry())
}

func imageStreamUpstreamError(value any) *types.OpenAIError {
	switch value := value.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil
		}
	case bool:
		if !value {
			return nil
		}
	case map[string]any:
		if len(value) == 0 {
			return nil
		}
	}
	return dto.GetOpenAIError(value)
}

// Some providers report errors with HTTP 200 or omit the error type.
// Empty error placeholders must not invalidate otherwise successful images.
func openaiImageResponseError(response *dto.SimpleResponse, status int) *types.NewAPIError {
	if upstreamError := imageStreamUpstreamError(response.Error); upstreamError != nil {
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			status = http.StatusBadGateway
		}
		return types.WithOpenAIError(*upstreamError, status, types.ErrOptionWithSkipRetry())
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return types.NewOpenAIError(fmt.Errorf("image upstream returned HTTP %d", status), types.ErrorCodeBadResponse, status)
	}
	return nil
}

func openaiImageJSONAsStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	// Only decode usage/error. Do not Unmarshal data[] into dto.ImageResponse —
	// b64_json values are large and would be copied into Go strings then
	// re-marshaled for each SSE event.
	var usageResp dto.SimpleResponse
	if err := common.Unmarshal(responseBody, &usageResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if apiErr := openaiImageResponseError(&usageResp, resp.StatusCode); apiErr != nil {
		return nil, apiErr
	}
	imageCount := relaycommon.ImageResponseCount(responseBody)
	taskResolved := false
	if imageCount == 0 && !info.IsAsyncImageRequest && info.ImageTaskWaiter != nil {
		// A successful submission can contain a task ID instead of images.
		// Resolve it before emitting SSE so polling errors remain refundable.
		resolved, apiErr := info.ImageTaskWaiter(c, info, responseBody)
		if apiErr != nil {
			return nil, apiErr
		}
		if len(resolved) > 0 {
			responseBody = resolved
			taskResolved = true
			usageResp = dto.SimpleResponse{}
			if err := common.Unmarshal(responseBody, &usageResp); err != nil {
				return nil, imageStreamError(err)
			}
			if apiErr := openaiImageResponseError(&usageResp, http.StatusOK); apiErr != nil {
				return nil, apiErr
			}
			imageCount = relaycommon.ImageResponseCount(responseBody)
		}
	}
	if imageCount == 0 {
		return nil, imageStreamError(fmt.Errorf("image response contains no completed images"))
	}
	normalizeOpenAIUsage(&usageResp.Usage)
	applyUsagePostProcessing(info, &usageResp.Usage, responseBody)
	info.UpdateImageCount(imageCount)
	info.SetImageBillingResponse(responseBody)

	// Finish format/storage conversion before writing any events. A storage
	// failure must not leave the client with a partly emitted JSON fallback.
	var images [][]byte
	for _, item := range gjson.GetBytes(responseBody, "data").Array() {
		if !openaiImageDataHasField(item, "url") && openaiImageBase64(item) == "" {
			continue
		}
		raw := responseBody[item.Index : item.Index+len(item.Raw)]
		if !taskResolved {
			// Task results are already normalized by ImageTaskWaiter. Applying
			// force-store again would upload twice or fetch our own stored URL.
			raw, err = normalizeOpenaiStreamImage(c, info, raw)
			if err != nil {
				return nil, imageStreamError(fmt.Errorf("normalize completed image: %w", err))
			}
		}
		images = append(images, raw)
	}
	if info.StreamStatus == nil {
		info.StreamStatus = relaycommon.NewStreamStatus()
	}
	eventType := "image_generation.completed"
	if info.RelayMode == relayconstant.RelayModeImagesEdits {
		eventType = "image_edit.completed"
	}

	helper.SetEventStreamHeaders(c)
	c.Status(http.StatusOK)

	created := gjson.GetBytes(responseBody, "created").Int()
	if created == 0 {
		created = time.Now().Unix()
	}
	if info != nil {
		info.SetFirstResponseTime()
	}

	validUsage := service.ValidUsage(&usageResp.Usage)
	var usageJSON []byte
	if validUsage {
		usageJSON, err = common.Marshal(usageResp.Usage)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
	}

	// gjson.Result.Array returns the element list for an array and a single
	// element for an object-shaped data, keeping document-relative indexes so
	// the zero-copy field forwarding below stays valid for both shapes.
	// Entries without url or b64_json carry no image and are not forwarded.
	response := gjson.ParseBytes(responseBody)
	for _, imageBody := range images {
		image := gjson.ParseBytes(imageBody)
		payload, err := common.Marshal(struct {
			Type string `json:"type"`
		}{Type: eventType})
		if err != nil {
			return nil, imageStreamError(err)
		}
		payload, err = sjson.SetBytes(payload, "created_at", created)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if validUsage {
			payload, err = sjson.SetRawBytes(payload, "usage", usageJSON)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
		}
		// b64_json goes last: every sjson.Set* reallocates the whole payload,
		// so inserting the large blob after all small fields avoids re-copying
		// multi-MB buffers.
		for _, field := range []string{"url", "revised_prompt", "size", "quality", "output_format", "background", "b64_json"} {
			value := image.Get(field)
			valueBody := imageBody
			switch field {
			case "size", "quality", "output_format", "background":
				// OpenAI puts shared image metadata at the response root.
				// Preserve per-image values when an upstream supplies them.
				if value.Type != gjson.String || strings.TrimSpace(value.String()) == "" {
					value = response.Get(field)
					valueBody = responseBody
				}
			}
			if value.Type != gjson.String || value.Raw == `""` {
				continue
			}
			raw := []byte(value.Raw)
			if value.Index > 0 {
				raw = valueBody[value.Index : value.Index+len(value.Raw)]
			}
			payload, err = sjson.SetRawBytes(payload, field, raw)
			if err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			}
		}
		if writeErr := helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: eventType}, string(payload)); writeErr != nil {
			if info != nil && info.StreamStatus != nil {
				info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, writeErr)
			}
			return &usageResp.Usage, nil
		}
		info.ReceivedResponseCount++
	}
	if err := writeOpenaiImageStreamDone(c); err != nil {
		if info != nil && info.StreamStatus != nil {
			info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonClientGone, err)
		}
		return &usageResp.Usage, nil
	}
	if info != nil {
		if info.StreamStatus == nil {
			info.StreamStatus = relaycommon.NewStreamStatus()
		}
		info.StreamStatus.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	}
	return &usageResp.Usage, nil
}

// normalizeOpenaiStreamImage preserves event metadata and applies the same
// completed-image policy as the non-streaming relay. Previews bypass it.
func normalizeOpenaiStreamImage(c *gin.Context, info *relaycommon.RelayInfo, raw []byte) ([]byte, error) {
	image := gjson.ParseBytes(raw)
	original := dto.ImageData{Url: image.Get("url").String(), B64Json: image.Get("b64_json").String()}
	normalized := dto.ImageData{Url: original.Url, B64Json: openaiImageBase64(image)}
	var err error
	if info.ImageDataNormalizer != nil {
		normalized, err = info.ImageDataNormalizer(c, info, normalized)
		if err != nil {
			return nil, err
		}
	}
	// Remove alias payloads before adding canonical fields, so URL responses
	// cannot leak Base64 and large images are not duplicated in the event.
	for _, field := range []string{"base64", "image_base64", "imageBase64"} {
		if image.Get(field).Exists() {
			raw, err = sjson.DeleteBytes(raw, field)
			if err != nil {
				return nil, err
			}
		}
	}
	for _, field := range []struct{ name, before, after string }{
		{"url", original.Url, normalized.Url},
		{"b64_json", original.B64Json, normalized.B64Json},
	} {
		if field.before != field.after {
			raw, err = sjson.SetBytes(raw, field.name, field.after)
			if err != nil {
				return nil, err
			}
		}
	}
	return raw, nil
}

func writeOpenaiImageStreamDone(c *gin.Context) error {
	return helper.StringData(c, "[DONE]")
}
