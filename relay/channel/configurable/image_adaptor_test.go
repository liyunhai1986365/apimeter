package configurable

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageAdaptorBuildsApixoTextToImagePayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfo(relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:   "gpt-image-2",
		Prompt:  "a clean product photo",
		Size:    "2048x1536",
		Quality: "auto",
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "async", gjson.GetBytes(data, "request_type").String())
	require.Equal(t, "text-to-image", gjson.GetBytes(data, "input.mode").String())
	require.Equal(t, "a clean product photo", gjson.GetBytes(data, "input.prompt").String())
	require.Equal(t, "4:3", gjson.GetBytes(data, "input.aspect_ratio").String())
	require.Equal(t, "2k", gjson.GetBytes(data, "input.resolution").String())
	require.Equal(t, "medium", gjson.GetBytes(data, "input.quality").String())
	require.False(t, gjson.GetBytes(data, "input.image_urls").Exists())
}

func TestImageAdaptorBuildsApixoImageEditPayloadFromMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	require.NoError(t, writer.WriteField("model", "gpt-image-2"))
	require.NoError(t, writer.WriteField("prompt", "edit this image"))
	require.NoError(t, writer.WriteField("size", "auto"))
	require.NoError(t, writer.WriteField("quality", "auto"))
	part, err := writer.CreateFormFile("image[]", "source.jpg")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake-jpg"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfo(relayconstant.RelayModeImagesEdits)
	adaptor.Init(info)
	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:   "gpt-image-2",
		Prompt:  "edit this image",
		Size:    "auto",
		Quality: "auto",
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "image-to-image", gjson.GetBytes(data, "input.mode").String())
	require.Equal(t, "auto", gjson.GetBytes(data, "input.aspect_ratio").String())
	require.Equal(t, "1k", gjson.GetBytes(data, "input.resolution").String())
	require.Equal(t, "medium", gjson.GetBytes(data, "input.quality").String())
	require.Equal(t, int64(1), gjson.GetBytes(data, "input.image_urls.#").Int())
	require.Contains(t, gjson.GetBytes(data, "input.image_urls.0").String(), "/api/relay-temp-images/")
}

func TestImageAdaptorConvertsBase64ImageInputToTemporaryURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("duomi-gemini-image", "https://duomiapi.com", "duomi-key", relayconstant.RelayModeImagesEdits)
	adaptor.Init(info)
	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "edit this image",
		Image:  []byte(`"data:image/png;base64,ZmFrZS1pbWFnZQ=="`),
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(data, "image_urls.0").String(), "https://cdn.example.com/api/relay-temp-images/")
}

func TestImageAdaptorApixoRequestURLAndSubmitResponseFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfo(relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.apixo.ai/api/v1/generateTask/gpt-image-2", requestURL)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"code":200,"message":"success","data":{"taskId":"task-123"}}`))),
	}
	usage, newAPIError := adaptor.DoResponse(c, resp, info)
	require.Nil(t, newAPIError)
	require.IsType(t, &dto.Usage{}, usage)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "task-123", gjson.GetBytes(recorder.Body.Bytes(), "id").String())
}

func TestImageAdaptorBuildsDuomiGeminiTextToImagePayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("duomi-gemini-image", "https://duomiapi.com", "duomi-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-3-pro-image-preview",
		Prompt: "中文海报",
		Size:   "1K",
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "gemini-3-pro-image-preview", gjson.GetBytes(data, "model").String())
	require.Equal(t, "中文海报", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, "1K", gjson.GetBytes(data, "image_size").String())
	require.False(t, gjson.GetBytes(data, "image_urls").Exists())

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/api/gemini/nano-banana", requestURL)

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	require.Equal(t, "duomi-key", header.Get("Authorization"))
}

func TestImageAdaptorBuildsDuomiGeminiImageEditPayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("duomi-gemini-image", "https://duomiapi.com", "duomi-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gemini-2.5-flash-image",
		Prompt: "改成红色字",
		Image:  []byte(`"https://cdn.example.com/ref.jpg"`),
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "gemini-2.5-flash-image", gjson.GetBytes(data, "model").String())
	require.Equal(t, "改成红色字", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, "https://cdn.example.com/ref.jpg", gjson.GetBytes(data, "image_urls.0").String())

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/api/gemini/nano-banana-edit", requestURL)
}

func TestImageAdaptorBuildsMoxingTextToImagePayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("moxing-gpt-image-2", "https://api.moxing.example", "moxing-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	n := uint(1)
	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "生成一个小狗的图片",
		N:              &n,
		Size:           "2k",
		ResponseFormat: "url",
		Extra: map[string]json.RawMessage{
			"aspect_ratio": []byte(`"21:9"`),
		},
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", gjson.GetBytes(data, "model").String())
	require.Equal(t, "生成一个小狗的图片", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, int64(1), gjson.GetBytes(data, "n").Int())
	require.Equal(t, "url", gjson.GetBytes(data, "response_format").String())
	require.Equal(t, "2K", gjson.GetBytes(data, "size").String())
	require.Equal(t, "21:9", gjson.GetBytes(data, "aspect_ratio").String())
	require.False(t, gjson.GetBytes(data, "reference_images").Exists())

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.moxing.example/v1/images/generations", requestURL)

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	require.Equal(t, "Bearer moxing-key", header.Get("Authorization"))
}

func TestImageAdaptorBuildsMoxingReferenceImagesPayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("moxing-gpt-image-2", "https://api.moxing.example", "moxing-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "参考图片生成一张海报",
		Size:   "2160x3840",
		Image:  []byte(`["https://example.com/ref1.png","https://example.com/ref2.png"]`),
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "4K", gjson.GetBytes(data, "size").String())
	require.Equal(t, "9:16", gjson.GetBytes(data, "aspect_ratio").String())
	require.Equal(t, int64(2), gjson.GetBytes(data, "reference_images.#").Int())
	require.Equal(t, "https://example.com/ref1.png", gjson.GetBytes(data, "reference_images.0").String())
	require.Equal(t, "https://example.com/ref2.png", gjson.GetBytes(data, "reference_images.1").String())
}

func TestImageAdaptorKeepsMoxingTextToImageFourKPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("moxing-gpt-image-2", "https://api.moxing.example", "moxing-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "生成竖版海报",
		Size:   "2160x3840",
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "4K", gjson.GetBytes(data, "size").String())
	require.Equal(t, "9:16", gjson.GetBytes(data, "aspect_ratio").String())
	require.False(t, gjson.GetBytes(data, "reference_images").Exists())
}

func TestImageAdaptorMoxingSubmitResponsePassesThroughOpenAIImageResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("moxing-gpt-image-2", "https://api.moxing.example", "moxing-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"created":1780000000,"data":[{"url":"https://cdn.example.com/image.png"}]}`))),
	}
	usage, newAPIError := adaptor.DoResponse(c, resp, info)
	require.Nil(t, newAPIError)
	require.IsType(t, &dto.Usage{}, usage)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "https://cdn.example.com/image.png", gjson.GetBytes(recorder.Body.Bytes(), "data.0.url").String())
}

func TestImageTaskResponseFromProfileExtractsBase64Images(t *testing.T) {
	profile := &Profile{
		MediaType: "image",
		Fetch: EndpointConfig{
			Response: ResponseConfig{
				TaskIDPath:       "id",
				StatusPath:       "state",
				ResultBase64Path: "images.#.b64_json",
				StatusMap: map[string]string{
					"success": "SUCCESS",
				},
			},
		},
	}

	response, ok, err := ImageTaskResponseFromProfile("task-1", profile, []byte(`{
		"id":"task-1",
		"state":"success",
		"images":[{"b64_json":"data:image/png;base64,ZmFrZQ=="}]
	}`))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "data:image/png;base64,ZmFrZQ==", response.Data.Images[0].B64Json)
}

func configurableImageRelayInfo(relayMode int) *relaycommon.RelayInfo {
	return configurableImageRelayInfoWithProfile("apixo-gpt-image-2", "https://api.apixo.ai", "apixo-key", relayMode)
}

func configurableImageRelayInfoWithProfile(profileID, baseURL, key string, relayMode int) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayMode:       relayMode,
		OriginModelName: "gpt-image-2",
		RequestURLPath:  "/v1/images/generations",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeConfigurable,
			ChannelBaseUrl:    baseURL,
			ApiKey:            key,
			UpstreamModelName: "gpt-image-2",
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: profileID,
				},
			},
		},
	}
}
