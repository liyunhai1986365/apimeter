package openai

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIImageRequestKeepsStandardEndpointAndBearerAuthorization(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gemini-3-pro-image-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	gotURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/v1/images/generations", gotURL)

	header := http.Header{}
	err = adaptor.SetupRequestHeader(c, &header, info)
	require.NoError(t, err)
	require.Equal(t, "Bearer duomi-key", header.Get("Authorization"))
}

func TestOpenAIImageGenerationWithImageInputUsesEditsEndpoint(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这只猫的背景",
		Size:   "1024x1024",
		Image:  []byte(`["data:image/png;base64,ZmFrZS1pbWFnZQ=="]`),
	})
	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)
	require.Contains(t, c.Request.Header.Get("Content-Type"), "multipart/form-data")

	gotURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/v1/images/edits", gotURL)

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), multipartBoundary(t, c.Request.Header.Get("Content-Type")))
	form, err := reader.ReadForm(1024 * 1024)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", form.Value["model"][0])
	require.Equal(t, "修改这只猫的背景", form.Value["prompt"][0])
	require.Equal(t, "1024x1024", form.Value["size"][0])
	require.Len(t, form.File["image"], 1)
	require.Equal(t, "image-1.png", form.File["image"][0].Filename)
}

func TestOpenAIImageGenerationWithImageInputKeepsGenerationsWhenAutoConvertDisabled(t *testing.T) {
	disabled := false
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				ImageAutoConvertGenerationWithImageToEdit: &disabled,
			},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这只猫的背景",
		Image:  []byte(`["data:image/png;base64,ZmFrZS1pbWFnZQ=="]`),
	})
	require.NoError(t, err)
	_, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, relayconstant.RelayModeImagesGenerations, info.RelayMode)
	require.Equal(t, "application/json", c.Request.Header.Get("Content-Type"))

	gotURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/v1/images/generations", gotURL)
}

func TestOpenAIImageGenerationDoesNotDefaultResponseFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "画一张海报",
	})
	require.NoError(t, err)
	imageReq, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Empty(t, imageReq.ResponseFormat)
}

func TestOpenAIImageGenerationKeepsExplicitResponseFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "画一张海报",
		ResponseFormat: "b64_json",
	})
	require.NoError(t, err)
	imageReq, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "b64_json", imageReq.ResponseFormat)
}

func TestOpenAIImageGenerationTokenFormatOverridesUserRequestForUpstreamRequest(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesGenerations,
		RequestURLPath:     "/v1/images/generations",
		OriginModelName:    "gpt-image-2",
		TokenImageSettings: dto.TokenImageSettings{Format: "url"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "画一张海报",
		ResponseFormat: "b64_json",
	})
	require.NoError(t, err)
	imageReq, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "url", imageReq.ResponseFormat)
}

func TestOpenAIImageGenerationKeepsUserResponseFormatBeforeChannelFallback(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "url",
			},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "画一张海报",
		ResponseFormat: "b64_json",
	})
	require.NoError(t, err)
	imageReq, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "b64_json", imageReq.ResponseFormat)
}

func TestOpenAIImageGenerationUsesChannelResponseFormatWhenUserOmitted(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesGenerations,
		RequestURLPath:  "/v1/images/generations",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "url",
			},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "画一张海报",
	})
	require.NoError(t, err)
	imageReq, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "url", imageReq.ResponseFormat)
}

func TestOpenAIImageGenerationTokenFormatOverridesChannelResponseFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesGenerations,
		RequestURLPath:     "/v1/images/generations",
		OriginModelName:    "gpt-image-2",
		TokenImageSettings: dto.TokenImageSettings{Format: "b64_json"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "url",
			},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	original := dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "画一张海报",
		ResponseFormat: "url",
	}
	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, original)
	require.NoError(t, err)
	imageReq, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, "b64_json", imageReq.ResponseFormat)
	require.Equal(t, "url", original.ResponseFormat)
}

func TestOpenAIImageEditMultipartUsesConfiguredResponseFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "b64_json",
			},
		},
	}

	var source bytes.Buffer
	sourceWriter := multipart.NewWriter(&source)
	require.NoError(t, sourceWriter.WriteField("model", "gpt-image-2"))
	require.NoError(t, sourceWriter.WriteField("prompt", "修改这只猫"))
	require.NoError(t, sourceWriter.WriteField("response_format", "url"))
	part, err := sourceWriter.CreateFormFile("image", "cat.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, sourceWriter.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(source.Bytes()))
	c.Request.Header.Set("Content-Type", sourceWriter.FormDataContentType())

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "修改这只猫",
		ResponseFormat: "url",
	})
	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), multipartBoundary(t, c.Request.Header.Get("Content-Type")))
	form, err := reader.ReadForm(1024 * 1024)
	require.NoError(t, err)
	require.Equal(t, "url", form.Value["response_format"][0])
	require.Len(t, form.File["image"], 1)
}

func TestOpenAIImageEditMultipartParamOverrideCanDeleteConfiguredResponseFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "url",
			},
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode": "delete",
						"path": "response_format",
					},
				},
			},
		},
	}

	var source bytes.Buffer
	sourceWriter := multipart.NewWriter(&source)
	require.NoError(t, sourceWriter.WriteField("model", "gpt-image-2"))
	require.NoError(t, sourceWriter.WriteField("prompt", "修改这只猫"))
	part, err := sourceWriter.CreateFormFile("image", "cat.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, sourceWriter.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(source.Bytes()))
	c.Request.Header.Set("Content-Type", sourceWriter.FormDataContentType())

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这只猫",
	})
	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), multipartBoundary(t, c.Request.Header.Get("Content-Type")))
	form, err := reader.ReadForm(1024 * 1024)
	require.NoError(t, err)
	require.NotContains(t, form.Value, "response_format")
	require.Len(t, form.File["image"], 1)
}

func TestOpenAIImageEditMultipartParamOverrideDeletesClientResponseFormat(t *testing.T) {
	for _, responseFormat := range []string{"url", "b64_json", "custom"} {
		t.Run(responseFormat, func(t *testing.T) {
			var source bytes.Buffer
			writer := multipart.NewWriter(&source)
			require.NoError(t, writer.WriteField("model", "gpt-image-2.5-sunburst"))
			require.NoError(t, writer.WriteField("prompt", "edit the image"))
			require.NoError(t, writer.WriteField("response_format", responseFormat))
			file, err := writer.CreateFormFile("image", "input.png")
			require.NoError(t, err)
			_, err = file.Write([]byte("fake image"))
			require.NoError(t, err)
			require.NoError(t, writer.Close())

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(source.Bytes()))
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeImagesEdits,
				ChannelMeta: &relaycommon.ChannelMeta{
					ParamOverride: map[string]any{
						"operations": []any{map[string]any{"mode": "delete", "path": "response_format"}},
					},
				},
			}
			request := dto.ImageRequest{Model: "gpt-image-2.5-sunburst", Prompt: "edit the image", ResponseFormat: responseFormat}
			adaptor := &Adaptor{}
			for _, deleteResponseFormat := range []bool{true, false} {
				if !deleteResponseFormat {
					// A fallback channel that accepts the field must still see the original value.
					info.ParamOverride = nil
				}
				converted, err := adaptor.ConvertImageRequest(c, info, request)
				require.NoError(t, err)
				body, ok := converted.(*bytes.Buffer)
				require.True(t, ok)
				form, err := multipart.NewReader(body, multipartBoundary(t, c.Request.Header.Get("Content-Type"))).ReadForm(1 << 20)
				require.NoError(t, err)
				if deleteResponseFormat {
					require.NotContains(t, form.Value, "response_format")
				} else {
					require.Equal(t, []string{responseFormat}, form.Value["response_format"])
				}
				require.Equal(t, []string{request.Model}, form.Value["model"])
				require.Equal(t, []string{request.Prompt}, form.Value["prompt"])
				require.Len(t, form.File["image"], 1)
				require.NoError(t, form.RemoveAll())
				require.Equal(t, []string{responseFormat}, c.Request.MultipartForm.Value["response_format"])
			}
			require.NoError(t, c.Request.MultipartForm.RemoveAll())
		})
	}
}

func TestOpenAIImageEditMultipartTokenFormatOverridesUserAndChannelResponseFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesEdits,
		RequestURLPath:     "/v1/images/edits",
		OriginModelName:    "gpt-image-2",
		TokenImageSettings: dto.TokenImageSettings{Format: "url"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "b64_json",
			},
		},
	}

	var source bytes.Buffer
	sourceWriter := multipart.NewWriter(&source)
	require.NoError(t, sourceWriter.WriteField("model", "gpt-image-2"))
	require.NoError(t, sourceWriter.WriteField("prompt", "修改这只猫"))
	require.NoError(t, sourceWriter.WriteField("response_format", "b64_json"))
	part, err := sourceWriter.CreateFormFile("image", "cat.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("fake image"))
	require.NoError(t, err)
	require.NoError(t, sourceWriter.Close())

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(source.Bytes()))
	c.Request.Header.Set("Content-Type", sourceWriter.FormDataContentType())

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:          "gpt-image-2",
		Prompt:         "修改这只猫",
		ResponseFormat: "b64_json",
	})
	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), multipartBoundary(t, c.Request.Header.Get("Content-Type")))
	form, err := reader.ReadForm(1024 * 1024)
	require.NoError(t, err)
	require.Equal(t, "url", form.Value["response_format"][0])
	require.Len(t, form.File["image"], 1)
}

func TestOpenAIImageEditsJSONInputBuildsMultipart(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这只猫的背景",
		Image:  []byte(`["data:image/png;base64,ZmFrZS1pbWFnZQ=="]`),
	})
	require.NoError(t, err)
	body, ok := converted.(*bytes.Buffer)
	require.True(t, ok)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)
	require.Contains(t, c.Request.Header.Get("Content-Type"), "multipart/form-data")

	reader := multipart.NewReader(bytes.NewReader(body.Bytes()), multipartBoundary(t, c.Request.Header.Get("Content-Type")))
	form, err := reader.ReadForm(1024 * 1024)
	require.NoError(t, err)
	require.Equal(t, "gpt-image-2", form.Value["model"][0])
	require.Equal(t, "修改这只猫的背景", form.Value["prompt"][0])
	require.Len(t, form.File["image"], 1)
}

func TestOpenAIImageEditsJSONInputKeepsJSONWhenAutoConvertDisabled(t *testing.T) {
	disabled := false
	info := &relaycommon.RelayInfo{
		RelayMode:       relayconstant.RelayModeImagesEdits,
		RequestURLPath:  "/v1/images/edits",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ApiType:        constant.APITypeOpenAI,
			ChannelBaseUrl: "https://duomiapi.com",
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				ImageAutoConvertJSONEditToMultipart: &disabled,
			},
		},
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := &Adaptor{}
	converted, err := adaptor.ConvertImageRequest(c, info, dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这只猫的背景",
		Image:  []byte(`["data:image/png;base64,ZmFrZS1pbWFnZQ=="]`),
	})
	require.NoError(t, err)
	_, ok := converted.(dto.ImageRequest)
	require.True(t, ok)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)
	require.Equal(t, "application/json", c.Request.Header.Get("Content-Type"))
}

func multipartBoundary(t *testing.T, contentType string) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	return params["boundary"]
}
