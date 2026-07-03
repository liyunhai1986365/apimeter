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
		Model:  "nano-banana-2",
		Prompt: "中文海报",
		Extra: map[string]json.RawMessage{
			"aspect_ratio":     []byte(`"16:9"`),
			"resolution":       []byte(`"2k"`),
			"provider_options": []byte(`{"duomi":{"oversea":true}}`),
		},
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "gemini-3.1-flash-lite-image", gjson.GetBytes(data, "model").String())
	require.Equal(t, "中文海报", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, "16:9", gjson.GetBytes(data, "aspect_ratio").String())
	require.Equal(t, "2K", gjson.GetBytes(data, "image_size").String())
	require.True(t, gjson.GetBytes(data, "oversea").Bool())
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
		Model:  "nano-banana-2",
		Prompt: "改成红色字",
		Image:  []byte(`"https://cdn.example.com/ref.jpg"`),
		Extra: map[string]json.RawMessage{
			"resolution": []byte(`"1k"`),
		},
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "gemini-3-pro-image-preview", gjson.GetBytes(data, "model").String())
	require.Equal(t, "改成红色字", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, "1K", gjson.GetBytes(data, "image_size").String())
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

func TestImageAdaptorBuildsWaveSpeedNanoBananaProTextToImagePayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("wavespeed-nano-banana-pro", "https://api.wavespeed.ai", "wavespeed-key", relayconstant.RelayModeImagesGenerations)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "nanp-banana-pro",
		Prompt: "a precise product render",
		Extra: map[string]json.RawMessage{
			"aspect_ratio":  []byte(`"4:3"`),
			"resolution":    []byte(`"2k"`),
			"output_format": []byte(`"jpeg"`),
		},
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "a precise product render", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, "4:3", gjson.GetBytes(data, "aspect_ratio").String())
	require.Equal(t, "2k", gjson.GetBytes(data, "resolution").String())
	require.Equal(t, "jpeg", gjson.GetBytes(data, "output_format").String())
	require.False(t, gjson.GetBytes(data, "images").Exists())

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.wavespeed.ai/api/v3/google/nano-banana-pro/text-to-image", requestURL)

	header := http.Header{}
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	require.Equal(t, "Bearer wavespeed-key", header.Get("Authorization"))
}

func TestImageAdaptorBuildsWaveSpeedNanoBananaProEditPayloadFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("wavespeed-nano-banana-pro", "https://api.wavespeed.ai", "wavespeed-key", relayconstant.RelayModeImagesEdits)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "nanp-banana-pro",
		Prompt: "replace the logo with blue text",
		Image:  []byte(`["https://cdn.example.com/ref-a.png","https://cdn.example.com/ref-b.png"]`),
		Extra: map[string]json.RawMessage{
			"aspect_ratio": []byte(`"1:1"`),
			"resolution":   []byte(`"1k"`),
		},
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "replace the logo with blue text", gjson.GetBytes(data, "prompt").String())
	require.Equal(t, "1:1", gjson.GetBytes(data, "aspect_ratio").String())
	require.Equal(t, "1k", gjson.GetBytes(data, "resolution").String())
	require.Equal(t, "png", gjson.GetBytes(data, "output_format").String())
	require.Equal(t, "https://cdn.example.com/ref-a.png", gjson.GetBytes(data, "images.0").String())
	require.Equal(t, "https://cdn.example.com/ref-b.png", gjson.GetBytes(data, "images.1").String())

	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://api.wavespeed.ai/api/v3/google/nano-banana-pro/edit", requestURL)
}

func TestImageAdaptorBuildsWaveSpeedNanoBanana2PayloadsFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name     string
		model    string
		image    json.RawMessage
		expected string
	}{
		{
			name:     "nano banana 2 text to image",
			model:    "nano-banana-2",
			expected: "https://api.wavespeed.ai/api/v3/google/nano-banana-2/text-to-image",
		},
		{
			name:     "nano banana 2 edit",
			model:    "nano-banana-2",
			image:    []byte(`"https://cdn.example.com/ref.png"`),
			expected: "https://api.wavespeed.ai/api/v3/google/nano-banana-2/edit",
		},
		{
			name:     "nano banana 2 lite text to image",
			model:    "nano-banana-2-lite",
			expected: "https://api.wavespeed.ai/api/v3/google/nano-banana-2-lite/text-to-image",
		},
		{
			name:     "nano banana 2 lite edit",
			model:    "nano-banana-2-lite",
			image:    []byte(`"https://cdn.example.com/ref.png"`),
			expected: "https://api.wavespeed.ai/api/v3/google/nano-banana-2-lite/edit",
		},
		{
			name:     "nano banana text to image",
			model:    "nano-banana",
			expected: "https://api.wavespeed.ai/api/v3/google/nano-banana/text-to-image",
		},
		{
			name:     "nano banana edit",
			model:    "nano-banana",
			image:    []byte(`"https://cdn.example.com/ref.png"`),
			expected: "https://api.wavespeed.ai/api/v3/google/nano-banana/edit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := &ImageAdaptor{}
			info := configurableImageRelayInfoWithProfile("wavespeed-nano-banana-pro", "https://api.wavespeed.ai", "wavespeed-key", relayconstant.RelayModeImagesGenerations)
			adaptor.Init(info)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			c.Request.Header.Set("Content-Type", "application/json")

			payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
				Model:  tc.model,
				Prompt: "cat",
				Image:  tc.image,
				Extra: map[string]json.RawMessage{
					"aspect_ratio": []byte(`"1:1"`),
					"resolution":   []byte(`"1k"`),
				},
			})
			require.NoError(t, err)

			data, err := common.Marshal(payload)
			require.NoError(t, err)
			require.Equal(t, "cat", gjson.GetBytes(data, "prompt").String())

			requestURL, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, tc.expected, requestURL)
		})
	}
}

func TestImageAdaptorBuildsWaveSpeedNanoBanana2AdvancedOptionsFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	adaptor := &ImageAdaptor{}
	info := configurableImageRelayInfoWithProfile("wavespeed-nano-banana-pro", "https://api.wavespeed.ai", "wavespeed-key", relayconstant.RelayModeImagesEdits)
	adaptor.Init(info)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "nano-banana-2",
		Prompt: "edit with live references",
		Image:  []byte(`["https://cdn.example.com/ref-1.png","https://cdn.example.com/ref-2.png"]`),
		Extra: map[string]json.RawMessage{
			"aspect_ratio":        []byte(`"1:8"`),
			"resolution":          []byte(`"0.5k"`),
			"enable_web_search":   []byte(`true`),
			"enable_image_search": []byte(`true`),
			"output_format":       []byte(`"jpeg"`),
		},
	})
	require.NoError(t, err)

	data, err := common.Marshal(payload)
	require.NoError(t, err)
	require.Equal(t, "1:8", gjson.GetBytes(data, "aspect_ratio").String())
	require.Equal(t, "0.5k", gjson.GetBytes(data, "resolution").String())
	require.True(t, gjson.GetBytes(data, "enable_web_search").Bool())
	require.True(t, gjson.GetBytes(data, "enable_image_search").Bool())
	require.Equal(t, "jpeg", gjson.GetBytes(data, "output_format").String())
	require.Equal(t, int64(2), gjson.GetBytes(data, "images.#").Int())
}

func TestImageAdaptorBuildsWaveSpeedGPTImage2PayloadsFromProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		name     string
		image    json.RawMessage
		expected string
	}{
		{
			name:     "text to image",
			expected: "https://api.wavespeed.ai/api/v3/openai/gpt-image-2/text-to-image",
		},
		{
			name:     "edit",
			image:    []byte(`"https://cdn.example.com/ref.png"`),
			expected: "https://api.wavespeed.ai/api/v3/openai/gpt-image-2/edit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adaptor := &ImageAdaptor{}
			info := configurableImageRelayInfoWithProfile("wavespeed-nano-banana-pro", "https://api.wavespeed.ai", "wavespeed-key", relayconstant.RelayModeImagesGenerations)
			adaptor.Init(info)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			c.Request.Header.Set("Content-Type", "application/json")

			payload, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
				Model:   "gpt-image-2",
				Prompt:  "cat",
				Size:    "2048x1536",
				Quality: "high",
				Image:   tc.image,
			})
			require.NoError(t, err)

			data, err := common.Marshal(payload)
			require.NoError(t, err)
			require.Equal(t, "cat", gjson.GetBytes(data, "prompt").String())
			require.Equal(t, "high", gjson.GetBytes(data, "quality").String())

			requestURL, err := adaptor.GetRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, tc.expected, requestURL)
		})
	}
}

func TestImageTaskResponseFromWaveSpeedNanoBananaProProfileExtractsOutputs(t *testing.T) {
	profile, ok := GetProfile("wavespeed-nano-banana-pro")
	require.True(t, ok)

	response, ok, err := ImageTaskResponseFromProfile("pred-123", profile, []byte(`{
		"code":200,
		"message":"success",
		"data":{
			"id":"pred-123",
			"status":"completed",
			"outputs":["https://cdn.example.com/result.png"],
			"created_at":"2026-07-02T10:20:30.000Z",
			"error":""
		}
	}`))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "pred-123", response.ID)
	require.Equal(t, "success", response.State)
	require.Equal(t, "https://cdn.example.com/result.png", response.Data.Images[0].URL)
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
