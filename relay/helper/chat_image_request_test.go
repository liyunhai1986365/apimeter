package helper

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/conversion"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestApplyRequestConvertsImageChatCompletionToImageRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"model":"gpt-image-2",
		"messages":[{"role":"user","content":"Draw a small robot watering plants"}],
		"size":"1024x1024",
		"n":2
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	_, ok := request.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)

	request, plan, err := conversion.ApplyRequest(c, types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, request)
	require.NoError(t, err)
	require.NotNil(t, plan)
	plan.Store(c)

	imageRequest, ok := request.(*dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "gpt-image-2", imageRequest.Model)
	require.Equal(t, "Draw a small robot watering plants", imageRequest.Prompt)
	require.Equal(t, "1024x1024", imageRequest.Size)
	require.NotNil(t, imageRequest.N)
	require.Equal(t, uint(2), *imageRequest.N)
	require.Equal(t, relayconstant.RelayModeImagesGenerations, c.GetInt("relay_mode"))
	require.Equal(t, string(conversion.ConversionOpenAIChatToImageGenerations), c.GetString(conversion.ContextKeyConversionID))
}

func TestGetAndValidateRequestKeepsNormalChatCompletionAsTextRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)

	_, ok := request.(*dto.GeneralOpenAIRequest)
	require.True(t, ok)
	require.Equal(t, 0, c.GetInt("relay_mode"))
	require.False(t, c.GetBool("chat_image_compat"))
}

func TestApplyRequestPreservesImageURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"model":"gemini-3-pro-image-preview",
		"messages":[
			{"role":"system","content":"Use a watercolor style."},
			{"role":"user","content":[
				{"type":"text","text":"Turn this into a poster"},
				{"type":"image_url","image_url":{"url":"https://example.com/input.png"}}
			]}
		]
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	request, plan, err := conversion.ApplyRequest(c, types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, request)
	require.NoError(t, err)
	require.NotNil(t, plan)

	imageRequest, ok := request.(*dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "Use a watercolor style.\nTurn this into a poster", imageRequest.Prompt)
	var imageURLs []string
	require.NoError(t, common.Unmarshal(imageRequest.Images, &imageURLs))
	require.Equal(t, []string{"https://example.com/input.png"}, imageURLs)
}

func TestApplyRequestConvertsGeminiImageGenerateContentToImageRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"contents":[
			{"role":"user","parts":[{"text":"cat"},{"inlineData":{"mimeType":"image/png","data":"aW5wdXQ="}}]}
		],
		"generationConfig":{
			"responseModalities":["TEXT","IMAGE"],
			"candidateCount":2,
			"imageConfig":{"aspectRatio":"1:1","imageSize":"2K"}
		}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{
				string(conversion.ConversionGeminiGenerateContentToImageGenerations),
			},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatGemini)
	require.NoError(t, err)
	_, ok := request.(*dto.GeminiChatRequest)
	require.True(t, ok)

	request, plan, err := conversion.ApplyRequest(c, types.RelayFormatGemini, relayconstant.RelayModeGemini, request)
	require.NoError(t, err)
	require.NotNil(t, plan)
	plan.Store(c)

	imageRequest, ok := request.(*dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "gemini-3.1-flash-image-preview", imageRequest.Model)
	require.Equal(t, "cat", imageRequest.Prompt)
	require.NotNil(t, imageRequest.N)
	require.Equal(t, uint(2), *imageRequest.N)
	var aspectRatio string
	require.NoError(t, common.Unmarshal(imageRequest.Extra["aspect_ratio"], &aspectRatio))
	require.Equal(t, "1:1", aspectRatio)
	var imageSize string
	require.NoError(t, common.Unmarshal(imageRequest.Extra["image_size"], &imageSize))
	require.Equal(t, "2K", imageSize)
	var images []string
	require.NoError(t, common.Unmarshal(imageRequest.Images, &images))
	require.Equal(t, []string{"data:image/png;base64,aW5wdXQ="}, images)
	require.Equal(t, relayconstant.RelayModeImagesGenerations, c.GetInt("relay_mode"))
	require.Equal(t, string(conversion.ConversionGeminiGenerateContentToImageGenerations), c.GetString(conversion.ContextKeyConversionID))
}

func TestApplyRequestAllowsGeminiImageConversionOnCanonicalImageChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"contents":[{"role":"user","parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["IMAGE"]}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{
				string(conversion.ConversionGeminiGenerateContentToImageGenerations),
			},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatGemini)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatGemini, relayconstant.RelayModeGemini, request)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.NotNil(t, converted)
	require.Equal(t, string(conversion.ConversionGeminiGenerateContentToImageGenerations), string(plan.ID))
}

func TestApplyRequestKeepsGeminiImageGenerateContentNativeWhenSupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"contents":[{"role":"user","parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["IMAGE"]}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeGeminiGenerateContent)},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatGemini)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatGemini, relayconstant.RelayModeGemini, request)
	require.NoError(t, err)
	require.Nil(t, plan)
	require.Same(t, request, converted)
}

func TestApplyRequestRejectsGeminiImageOnUnconfiguredGeminiChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"contents":[{"role":"user","parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["IMAGE"]}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)

	request, err := GetAndValidateRequest(c, types.RelayFormatGemini)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatGemini, relayconstant.RelayModeGemini, request)
	require.Error(t, err)
	require.Nil(t, plan)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), string(conversion.RequestModeOpenAIImageGenerations))
}

func TestApplyRequestConvertsGeminiImageOnGeminiChannelConfiguredAsImageOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"contents":[{"role":"user","parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["IMAGE"]}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-3.1-flash-image-preview:generateContent", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeGemini)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{
				string(conversion.ConversionGeminiGenerateContentToImageGenerations),
			},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatGemini)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatGemini, relayconstant.RelayModeGemini, request)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.IsType(t, &dto.ImageRequest{}, converted)
}

func TestApplyRequestRejectsImagenGenerateContentNativePassThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{
		"contents":[{"role":"user","parts":[{"text":"cat"}]}],
		"generationConfig":{"responseModalities":["IMAGE"]}
	}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/imagen-4.0-generate-001:generateContent", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes: []string{string(conversion.RequestModeGeminiGenerateContent)},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatGemini)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatGemini, relayconstant.RelayModeGemini, request)
	require.Error(t, err)
	require.Nil(t, plan)
	require.Nil(t, converted)
}

func TestApplyRequestRejectsImageChatWhenConversionDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-image-2","messages":[{"role":"user","content":"draw a river"}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			EnabledConversions: []string{"other.conversion"},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, request)
	require.Error(t, err)
	require.Nil(t, plan)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), string(conversion.ConversionOpenAIChatToImageGenerations))
}

func TestApplyRequestAllowsImageChatConversionOnCanonicalImageChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-image-2","messages":[{"role":"user","content":"draw a river"}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIImageGenerations)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, request)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.NotNil(t, converted)
}

func TestApplyRequestRejectsImageChatWhenTargetNativeModeUnsupported(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"gpt-image-2","messages":[{"role":"user","content":"draw a river"}]}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			NativeModes:        []string{string(conversion.RequestModeOpenAIChat)},
			EnabledConversions: []string{string(conversion.ConversionOpenAIChatToImageGenerations)},
		},
	})

	request, err := GetAndValidateRequest(c, types.RelayFormatOpenAI)
	require.NoError(t, err)
	converted, plan, err := conversion.ApplyRequest(c, types.RelayFormatOpenAI, relayconstant.RelayModeChatCompletions, request)
	require.Error(t, err)
	require.Nil(t, plan)
	require.Nil(t, converted)
	require.Contains(t, err.Error(), string(conversion.RequestModeOpenAIImageGenerations))
}
