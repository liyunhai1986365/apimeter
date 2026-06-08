package relay

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/imageconv"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) (newAPIError *types.NewAPIError) {
	info.InitChannelMeta(c)

	imageReq, ok := info.Request.(*dto.ImageRequest)
	if !ok {
		return types.NewErrorWithStatusCode(fmt.Errorf("invalid request type, expected dto.ImageRequest, got %T", info.Request), types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	request, err := common.DeepCopy(imageReq)
	if err != nil {
		return types.NewError(fmt.Errorf("failed to copy request to ImageRequest: %w", err), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	err = helper.ModelMappedHelper(c, info, request)
	if err != nil {
		return types.NewError(err, types.ErrorCodeChannelModelMappedError, types.ErrOptionWithSkipRetry())
	}

	adaptor := GetAdaptor(info.ApiType)
	if adaptor == nil {
		return types.NewError(fmt.Errorf("invalid api type: %d", info.ApiType), types.ErrorCodeInvalidApiType, types.ErrOptionWithSkipRetry())
	}
	adaptor.Init(info)

	var requestBody io.Reader

	if !shouldForceConvertImageRequest(info, imageReq) &&
		!isConvertedImageRequest(c, info) &&
		(model_setting.GetGlobalSettings().PassThroughRequestEnabled || info.ChannelSetting.PassThroughBodyEnabled) {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		requestBody = common.ReaderOnly(storage)
	} else {
		convertedRequest, err := adaptor.ConvertImageRequest(c, info, *request)
		if err != nil {
			return types.NewError(err, types.ErrorCodeConvertRequestFailed)
		}
		relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

		switch convertedRequest.(type) {
		case *bytes.Buffer:
			requestBody = convertedRequest.(io.Reader)
		default:
			jsonData, err := common.Marshal(convertedRequest)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}

			// apply param override
			if len(info.ParamOverride) > 0 {
				jsonData, err = relaycommon.ApplyParamOverrideWithRelayInfo(jsonData, info)
				if err != nil {
					return newAPIErrorFromParamOverride(err)
				}
			}

			logger.LogDebug(c, "image request body: %s", jsonData)
			body, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
			if err != nil {
				return types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			}
			defer closer.Close()
			jsonData = nil
			info.UpstreamRequestBodySize = size
			requestBody = body
		}
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return newImageSubmitUncertainError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	var httpResp *http.Response
	if resp != nil {
		httpResp = resp.(*http.Response)
		info.IsStream = info.IsStream || strings.HasPrefix(httpResp.Header.Get("Content-Type"), "text/event-stream")
		if httpResp.StatusCode != http.StatusOK {
			if httpResp.StatusCode == http.StatusCreated && info.ApiType == constant.APITypeReplicate {
				// replicate channel returns 201 Created when using Prefer: wait, treat it as success.
				httpResp.StatusCode = http.StatusOK
			} else {
				newAPIError = service.RelayErrorHandler(c.Request.Context(), httpResp, false)
				// reset status code 重置状态码
				service.ResetStatusCode(newAPIError, statusCodeMappingStr)
				return newAPIError
			}
		}
	}

	originalWriter := c.Writer
	responseCapture := newImageResponseCapture(originalWriter)
	c.Writer = responseCapture

	usage, newAPIError := adaptor.DoResponse(c, httpResp, info)
	c.Writer = originalWriter
	if newAPIError != nil {
		// reset status code 重置状态码
		service.ResetStatusCode(newAPIError, statusCodeMappingStr)
		return markImageSubmitUncertainError(newAPIError)
	}

	if responseCapture.HasBody() {
		if syncBody, syncErr := waitImageAsyncSubmitResponse(c, info, responseCapture.BodyBytes()); syncErr != nil {
			return syncErr
		} else if len(syncBody) > 0 {
			responseCapture.ReplaceBody(http.StatusOK, syncBody)
		} else {
			if asyncErr := recordImageAsyncSubmitResponse(info, responseCapture.BodyBytes()); asyncErr != nil {
				return asyncErr
			}
		}
	}

	usageData := usage.(*dto.Usage)
	if ensureEstimatedImageUsageIfMissing(info, request, usageData) && responseCapture.HasBody() {
		if body, injected := imageResponseBodyWithUsage(responseCapture.BodyBytes(), usageData); injected {
			responseCapture.ReplaceBody(responseCapture.Status(), body)
		}
	}

	if responseCapture.HasBody() {
		body, normalized, err := normalizeOpenAIImageResponseByFormat(c, info, responseCapture.BodyBytes())
		if err != nil {
			return newImageSubmitUncertainError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		if normalized {
			responseCapture.ReplaceBody(responseCapture.Status(), body)
		}
	}

	imageN := uint(1)
	if request.N != nil {
		imageN = *request.N
	}

	// n is handled via OtherRatio so it is applied exactly once in quota
	// calculation (both price-based and ratio-based paths).
	// Adaptors may have already set a more accurate count from the
	// upstream response; only set the default when they haven't.
	if info.PriceData.UsePrice { // only price model use N ratio
		if _, hasN := info.PriceData.OtherRatios["n"]; !hasN {
			info.PriceData.AddOtherRatio("n", float64(imageN))
		}
	}

	if usageData.TotalTokens == 0 {
		usageData.TotalTokens = 1
	}
	if usageData.PromptTokens == 0 {
		usageData.PromptTokens = 1
	}

	quality := "standard"
	if request.Quality == "hd" {
		quality = "hd"
	}

	var logContent []string

	if len(request.Size) > 0 {
		logContent = append(logContent, fmt.Sprintf("大小 %s", request.Size))
	}
	if len(quality) > 0 {
		logContent = append(logContent, fmt.Sprintf("品质 %s", quality))
	}
	if imageN > 0 {
		logContent = append(logContent, fmt.Sprintf("生成数量 %d", imageN))
	}

	service.PostTextConsumeQuota(c, info, usageData, logContent)
	if wrapped, wrapErr := maybeWrapChatImageResponse(c, info, responseCapture.BodyBytes(), usageData); wrapErr != nil {
		return wrapErr
	} else if wrapped {
		return nil
	}
	if !responseCapture.HasBody() {
		return newImageSubmitUncertainError(fmt.Errorf("image response body is empty"), types.ErrorCodeBadResponseBody, http.StatusBadGateway)
	}
	logImageCapturedResponse(c, responseCapture)
	if err := responseCapture.WriteCaptured(); err != nil {
		logger.LogError(c, fmt.Sprintf("image response write failed: %s", err.Error()))
	}
	return nil
}

func logImageCapturedResponse(c *gin.Context, responseCapture *imageResponseCapture) {
	if !common.DebugEnabled || responseCapture == nil {
		return
	}
	body := responseCapture.BodyBytes()
	preview := string(body)
	if len(preview) > 2048 {
		preview = preview[:2048] + "...[TRUNCATED]"
	}
	logger.LogDebug(c, "image response captured: status=%d body_size=%d body=%s", responseCapture.Status(), len(body), common.MaskSensitiveInfo(preview))
}

func isConvertedImageRequest(c *gin.Context, info *relaycommon.RelayInfo) bool {
	if info != nil && info.ConversionID != "" {
		return true
	}
	return conversion.ActiveConversionID(c) != "" || c.GetBool("chat_image_compat")
}

func shouldForceConvertImageRequest(info *relaycommon.RelayInfo, request *dto.ImageRequest) bool {
	if info == nil || request == nil {
		return false
	}
	settings := dto.ChannelSettings{}
	if info.ChannelMeta != nil {
		settings = info.ChannelSetting
	}
	if configurableImageProfileFromSettings(settings) != nil {
		return true
	}
	if settings.OpenAIImageResponseFormatOverride() != "" {
		return true
	}
	if !imageconv.RequestHasImageInput(request) {
		return false
	}
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations:
		return settings.ImageGenerationWithImageToEditEnabled()
	case relayconstant.RelayModeImagesEdits:
		return settings.ImageJSONEditToMultipartEnabled()
	default:
		return false
	}
}
