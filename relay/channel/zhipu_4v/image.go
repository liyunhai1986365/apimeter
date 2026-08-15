package zhipu_4v

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

type zhipuImageRequest struct {
	Model            string `json:"model"`
	Prompt           string `json:"prompt"`
	Quality          string `json:"quality,omitempty"`
	Size             string `json:"size,omitempty"`
	WatermarkEnabled *bool  `json:"watermark_enabled,omitempty"`
	UserID           string `json:"user_id,omitempty"`
}

type zhipuImageResponse struct {
	Created       *int64            `json:"created,omitempty"`
	Data          []zhipuImageData  `json:"data,omitempty"`
	ContentFilter any               `json:"content_filter,omitempty"`
	Usage         *dto.Usage        `json:"usage,omitempty"`
	Error         *zhipuImageError  `json:"error,omitempty"`
	RequestID     string            `json:"request_id,omitempty"`
	ExtendParam   map[string]string `json:"extendParam,omitempty"`
}

type zhipuImageError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type zhipuImageData struct {
	Url      string `json:"url,omitempty"`
	ImageUrl string `json:"image_url,omitempty"`
	B64Json  string `json:"b64_json,omitempty"`
	B64Image string `json:"b64_image,omitempty"`
}

type openAIImagePayload struct {
	Created int64             `json:"created"`
	Data    []openAIImageData `json:"data"`
}

type openAIImageData struct {
	URL     string `json:"url,omitempty"`
	B64Json string `json:"b64_json,omitempty"`
}

func zhipu4vImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var zhipuResp zhipuImageResponse
	if err := common.Unmarshal(responseBody, &zhipuResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if zhipuResp.Error != nil && zhipuResp.Error.Message != "" {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResp.Error.Message,
			Type:    "zhipu_image_error",
			Code:    zhipuResp.Error.Code,
		}, resp.StatusCode)
	}

	payload := openAIImagePayload{}
	if zhipuResp.Created != nil && *zhipuResp.Created != 0 {
		payload.Created = *zhipuResp.Created
	} else {
		payload.Created = info.StartTime.Unix()
	}
	for _, data := range zhipuResp.Data {
		url := data.Url
		if url == "" {
			url = data.ImageUrl
		}
		var base64Data string
		switch {
		case data.B64Json != "":
			base64Data = data.B64Json
		case data.B64Image != "":
			base64Data = data.B64Image
		}

		if zhipuWantsBase64(info, base64Data != "") {
			if base64Data == "" && url != "" {
				_, downloaded, err := service.GetImageFromUrl(url)
				if err != nil {
					logger.LogError(c, "zhipu_image_get_b64_failed: "+err.Error())
					return nil, types.NewError(err, types.ErrorCodeBadResponse)
				}
				base64Data = downloaded
			}
			if base64Data == "" {
				logger.LogWarn(c, "zhipu_image_empty_b64")
				continue
			}
			payload.Data = append(payload.Data, openAIImageData{B64Json: base64Data})
			continue
		}

		if url == "" && base64Data != "" {
			payload.Data = append(payload.Data, openAIImageData{B64Json: base64Data})
			continue
		}
		if url == "" {
			logger.LogWarn(c, "zhipu_image_missing_url")
			continue
		}
		payload.Data = append(payload.Data, openAIImageData{URL: url})
	}

	if len(payload.Data) == 0 {
		return nil, types.NewError(errors.New("zhipu image response contains no usable images"), types.ErrorCodeBadResponseBody)
	}

	jsonResp, err := common.Marshal(payload)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, jsonResp)

	return &dto.Usage{}, nil
}

func zhipuWantsBase64(info *relaycommon.RelayInfo, upstreamHasBase64 bool) bool {
	if info != nil {
		switch info.TokenImageSettings.Normalized().Format {
		case dto.TokenImageFormatB64JSON:
			return true
		case dto.TokenImageFormatURL:
			return false
		}
		if request, ok := info.Request.(*dto.ImageRequest); ok {
			switch strings.ToLower(strings.TrimSpace(request.ResponseFormat)) {
			case dto.TokenImageFormatB64JSON, "base64":
				return true
			case dto.TokenImageFormatURL:
				return false
			}
		}
		if info.ChannelMeta != nil {
			switch info.ChannelSetting.OpenAIImageResponseFormatOverride() {
			case dto.TokenImageFormatB64JSON:
				return true
			case dto.TokenImageFormatURL:
				return false
			}
		}
	}
	return upstreamHasBase64
}
