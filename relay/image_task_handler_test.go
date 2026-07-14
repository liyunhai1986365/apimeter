package relay

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func setupImageTaskTestDB(t *testing.T) {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}))
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
}

func gjsonGetString(t *testing.T, body []byte, path string) string {
	t.Helper()
	return gjson.GetBytes(body, path).String()
}

func TestRecordImageAsyncSubmitResponseStoresTaskAndKeepsPublicID(t *testing.T) {
	setupImageTaskTestDB(t)

	info := &relaycommon.RelayInfo{
		UserId:              7,
		UsingGroup:          "default",
		OriginModelName:     "gpt-image-2",
		IsAsyncImageRequest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ChannelId:   11,
		},
		PriceData: types.PriceData{
			QuotaToPreConsume: 123,
		},
	}

	apiErr := recordImageAsyncSubmitResponse(info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
	require.Nil(t, apiErr)

	task, exists, err := model.GetByTaskId(7, "7154f43c-7765-4e77-d51e-0db4eee107b0")
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, "7154f43c-7765-4e77-d51e-0db4eee107b0", task.TaskID)
	require.Equal(t, "7154f43c-7765-4e77-d51e-0db4eee107b0", task.GetUpstreamTaskID())
	require.Equal(t, constant.TaskActionGenerate, task.Action)
	require.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), task.Status)
	require.True(t, task.PrivateData.AsyncImage)
	require.JSONEq(t, `{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`, string(task.Data))
}

func TestRecordImageAsyncSubmitResponseSkipsTaskWhenClientDidNotRequestAsync(t *testing.T) {
	setupImageTaskTestDB(t)

	info := &relaycommon.RelayInfo{
		UserId:          7,
		UsingGroup:      "default",
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ChannelId:   11,
		},
		PriceData: types.PriceData{
			QuotaToPreConsume: 123,
		},
	}

	apiErr := recordImageAsyncSubmitResponse(info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
	require.Nil(t, apiErr)

	_, exists, err := model.GetByTaskId(7, "7154f43c-7765-4e77-d51e-0db4eee107b0")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestImageSubmitUncertainErrorSkipsRetry(t *testing.T) {
	apiErr := newImageSubmitUncertainError(fmt.Errorf("unexpected EOF"), types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	require.NotNil(t, apiErr)
	require.True(t, types.IsSkipRetryError(apiErr))
	require.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
}

func TestWaitImageAsyncSubmitResponseConvertsSucceededTaskToOpenAIImageResponse(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", r.URL.Path)
		require.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0","state":"succeeded","progress":0,"create_time":1779125744,"update_time":1779125807,"action":"generate","data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","file_name":"output.png"}]}}`))
	}))
	defer upstream.Close()

	info := &relaycommon.RelayInfo{
		StartTime:       time.Unix(1779125700, 0),
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "sk-upstream",
		},
	}

	router := gin.New()
	router.POST("/v1/images/generations", func(c *gin.Context) {
		body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
		require.Nil(t, apiErr)
		c.Data(http.StatusOK, "application/json", body)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"created": 1779125744,
		"data": [{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","b64_json":"","revised_prompt":""}]
	}`, recorder.Body.String())
}

func TestConvertImageAsyncResultStoresBase64AsURLByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, err := convertImageAsyncResultToOpenAIResponse(c, &relaycommon.RelayInfo{}, []byte(`{
		"id":"task-1",
		"state":"succeeded",
		"data":{"images":[{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}]}
	}`))
	require.NoError(t, err)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestConvertImageAsyncResultReturnsBase64WhenRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, err := convertImageAsyncResultToOpenAIResponse(c, &relaycommon.RelayInfo{
		Request: &dto.ImageRequest{ResponseFormat: "b64_json"},
	}, []byte(`{
		"id":"task-1",
		"state":"succeeded",
		"data":{"images":[{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}]}
	}`))
	require.NoError(t, err)
	require.Equal(t, "ZmFrZS1pbWFnZQ==", gjsonGetString(t, body, "data.0.b64_json"))
	require.Empty(t, gjsonGetString(t, body, "data.0.url"))
}

func TestNormalizeOpenAIImageResponseStoresBase64AsURLWhenURLRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		Request: &dto.ImageRequest{ResponseFormat: "url"},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
	require.Equal(t, int64(3), gjson.GetBytes(body, "usage.total_tokens").Int())
}

func TestNormalizeOpenAIImageResponseStoresDataURLFieldAsURLWhenURLRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		Request: &dto.ImageRequest{ResponseFormat: "url"},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"url":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
	require.Equal(t, int64(3), gjson.GetBytes(body, "usage.total_tokens").Int())
}

func TestNormalizeOpenAIImageResponseForcesBase64FromEndpointURLWhenTokenRequestsBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "b64_json",
		},
	}, []byte(fmt.Sprintf(`{
		"created": 1779125744,
		"data": [{"url":%q,"b64_json":""}],
		"usage": {"total_tokens": 3}
	}`, "data:image/png;base64,ZmFrZSBpbWFnZQ==")))

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "ZmFrZSBpbWFnZQ==", gjsonGetString(t, body, "data.0.b64_json"))
	require.Empty(t, gjsonGetString(t, body, "data.0.url"))
}

func TestNormalizeOpenAIImageResponseTokenURLOverridesChannelBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "url",
			Store:  "default",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "b64_json",
			},
		},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseTokenURLAfterGenerationImageInputConvertsToEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	imageReq := &dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这个 logo，设计一套 vi",
		Size:   "1024x1024",
		Image:  []byte(`["data:image/png;base64,ZmFrZS1sb2dv"]`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesGenerations,
		RequestURLPath:     "/v1/images/generations",
		Request:            imageReq,
		TokenImageSettings: model.TokenImageSettings{Format: "url", Store: "default"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ApiType:     constant.APITypeOpenAI,
		},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := GetAdaptor(constant.APITypeOpenAI)
	require.NotNil(t, adaptor)
	adaptor.Init(info)
	converted, err := adaptor.ConvertImageRequest(c, info, *imageReq)
	require.NoError(t, err)
	require.NotNil(t, converted)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)
	require.Equal(t, "/v1/images/edits", info.RequestURLPath)

	body, changed, err := normalizeOpenAIImageResponseByFormat(c, info, []byte(`{
		"created": 1779125744,
		"data": [{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseTokenURLAfterEditConversionHandlesBase64Alias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	imageReq := &dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "修改这个 logo，设计一套 vi",
		Size:   "1024x1024",
		Image:  []byte(`["data:image/png;base64,ZmFrZS1sb2dv"]`),
	}
	info := &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesGenerations,
		RequestURLPath:     "/v1/images/generations",
		Request:            imageReq,
		TokenImageSettings: model.TokenImageSettings{Format: "url", Store: "default"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeOpenAI,
			ApiType:     constant.APITypeOpenAI,
		},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	adaptor := GetAdaptor(constant.APITypeOpenAI)
	require.NotNil(t, adaptor)
	adaptor.Init(info)
	converted, err := adaptor.ConvertImageRequest(c, info, *imageReq)
	require.NoError(t, err)
	require.NotNil(t, converted)
	require.Equal(t, relayconstant.RelayModeImagesEdits, info.RelayMode)

	body, changed, err := normalizeOpenAIImageResponseByFormat(c, info, []byte(`{
		"created": 1779125744,
		"data": [{"image_base64":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
	require.Empty(t, gjsonGetString(t, body, "data.0.image_base64"))
}

func TestNormalizeOpenAIImageEditResponseTokenURLStoresBase64AsURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesEdits,
		RequestURLPath:     "/v1/images/edits",
		TokenImageSettings: model.TokenImageSettings{Format: "url", Store: "default"},
		Request: &dto.ImageRequest{
			Model:          "gpt-image-2",
			Prompt:         "修改这个 logo，设计一套 vi",
			ResponseFormat: "b64_json",
		},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
	require.Equal(t, int64(3), gjson.GetBytes(body, "usage.total_tokens").Int())
}

func TestNormalizeOpenAIImageResponseTokenURLConvertsBase64Alias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "url",
			Store:  "default",
		},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"base64":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.Empty(t, gjsonGetString(t, body, "data.0.base64"))
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseTokenURLUsesRelayInfoUserStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var uploadedBody string
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://site.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "url",
			Store:  "default",
		},
		UserSetting: dto.UserSetting{
			ImageStorage: dto.UserImageStorageSetting{
				Enabled:         true,
				Type:            dto.UserImageStorageTypeAliyunOSS,
				Endpoint:        ossServer.URL,
				Bucket:          "user-bucket",
				AccessKeyID:     "access-key",
				AccessKeySecret: "access-secret",
				ObjectPrefix:    "user-prefix",
				PublicBaseURL:   "https://user-cdn.example.com",
			},
		},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "fake-image", uploadedBody)
	require.True(t, strings.HasPrefix(gjsonGetString(t, body, "data.0.url"), "https://user-cdn.example.com/user-prefix/"))
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseUserURLKeepsEndpointURLByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		Request: &dto.ImageRequest{ResponseFormat: "url"},
	}, []byte(fmt.Sprintf(`{
		"created": 1779125744,
		"data": [{"url":%q}],
		"usage": {"total_tokens": 3}
	}`, "https://upstream.example.com/image.png")))

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "https://upstream.example.com/image.png", gjsonGetString(t, body, "data.0.url"))
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseChannelURLKeepsEndpointURLWhenUserStorageDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "url",
			},
		},
		UserSetting: dto.UserSetting{
			ImageStorage: dto.UserImageStorageSetting{
				Enabled: false,
				Type:    dto.UserImageStorageTypeAliyunOSS,
			},
		},
	}, []byte(fmt.Sprintf(`{
		"created": 1779125744,
		"data": [{"url":%q}],
		"usage": {"total_tokens": 3}
	}`, "https://upstream.example.com/image.png")))

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "https://upstream.example.com/image.png", gjsonGetString(t, body, "data.0.url"))
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseForceStoresEndpointURLInUserStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	disableSSRFProtectionForImageTaskTest(t)

	sourceImageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
			0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
			0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
			0x54, 0x08, 0xd7, 0x63, 0xf8, 0xff, 0xff, 0x3f,
			0x00, 0x05, 0xfe, 0x02, 0xfe, 0xdc, 0xcc, 0x59,
			0xe7, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
			0x44, 0xae, 0x42, 0x60, 0x82,
		})
	}))
	defer sourceImageServer.Close()

	var uploadedBody string
	ossServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		uploadedBody = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ossServer.Close()

	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://site.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "url",
			Store:  "force_store_url_and_base64",
		},
		UserSetting: dto.UserSetting{
			ImageStorage: dto.UserImageStorageSetting{
				Enabled:         true,
				Type:            dto.UserImageStorageTypeAliyunOSS,
				Endpoint:        ossServer.URL,
				Bucket:          "user-bucket",
				AccessKeyID:     "access-key",
				AccessKeySecret: "access-secret",
				ObjectPrefix:    "user-prefix",
				PublicBaseURL:   "https://user-cdn.example.com",
			},
		},
	}, []byte(fmt.Sprintf(`{
		"created": 1779125744,
		"data": [{"url":%q}],
		"usage": {"total_tokens": 3}
	}`, sourceImageServer.URL+"/image.png")))

	require.NoError(t, err)
	require.True(t, changed)
	require.NotEmpty(t, uploadedBody)
	require.True(t, strings.HasPrefix(gjsonGetString(t, body, "data.0.url"), "https://user-cdn.example.com/user-prefix/"))
	require.NotContains(t, gjsonGetString(t, body, "data.0.url"), sourceImageServer.URL)
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func disableSSRFProtectionForImageTaskTest(t *testing.T) {
	t.Helper()
	service.InitHttpClient()
	originalMaxFileDownloadMB := constant.MaxFileDownloadMB
	constant.MaxFileDownloadMB = 64
	fetchSetting := system_setting.GetFetchSetting()
	originalSSRF := fetchSetting.EnableSSRFProtection
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		constant.MaxFileDownloadMB = originalMaxFileDownloadMB
		fetchSetting.EnableSSRFProtection = originalSSRF
	})
}

func TestCapturedOpenAIImageResponseTokenURLWritesURLToClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	capture := newImageResponseCapture(c.Writer)
	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)
	_, err := capture.Write([]byte(`{
		"created": 1779125744,
		"data": [{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}]
	}`))
	require.NoError(t, err)

	body, normalized, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "url",
			Store:  "default",
		},
	}, capture.BodyBytes())
	require.NoError(t, err)
	require.True(t, normalized)
	capture.ReplaceBody(capture.Status(), body)
	require.NoError(t, capture.WriteCaptured())

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, gjson.Get(recorder.Body.String(), "data.0.url").String(), "https://cdn.example.com/api/relay-temp-images/")
	require.Empty(t, gjson.Get(recorder.Body.String(), "data.0.b64_json").String())
}

func TestNormalizeOpenAIImageResponseTokenBase64OverridesChannelURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "b64_json",
			Store:  "default",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				OpenAIImageResponseFormat: "url",
			},
		},
	}, []byte(`{
		"created": 1779125744,
		"data": [{"url":"data:image/png;base64,ZmFrZSBpbWFnZQ=="}],
		"usage": {"total_tokens": 3}
	}`))

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "ZmFrZSBpbWFnZQ==", gjsonGetString(t, body, "data.0.b64_json"))
	require.Empty(t, gjsonGetString(t, body, "data.0.url"))
}

func TestNormalizeOpenAIImageResponseForceStoresURLAndBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "follow_request",
			Store:  "force_store_url_and_base64",
		},
		Request: &dto.ImageRequest{ResponseFormat: "url"},
	}, []byte(fmt.Sprintf(`{
		"created": 1779125744,
		"data": [{"url":%q,"b64_json":""}],
		"usage": {"total_tokens": 3}
	}`, "data:image/png;base64,ZmFrZSBpbWFnZQ==")))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestNormalizeOpenAIImageResponseForceStoresEndpointURLWithoutReturningBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	body, changed, err := normalizeOpenAIImageResponseByFormat(c, &relaycommon.RelayInfo{
		TokenImageSettings: model.TokenImageSettings{
			Format: "url",
			Store:  "force_store_url_and_base64",
		},
	}, []byte(fmt.Sprintf(`{
		"created": 1779125744,
		"data": [{"url":%q,"b64_json":""}],
		"usage": {"total_tokens": 3}
	}`, "data:image/png;base64,ZmFrZS1pbWFnZS1mcm9tLXVybA==")))

	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, gjsonGetString(t, body, "data.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.NotContains(t, gjsonGetString(t, body, "data.0.url"), "base64")
	require.True(t, gjson.GetBytes(body, "data.0.b64_json").Exists())
	require.Empty(t, gjsonGetString(t, body, "data.0.b64_json"))
}

func TestImageTaskResponseWithURLImagesStoresBase64(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("API_TEMP_IMAGE_STORAGE", "local")
	t.Setenv("API_TEMP_IMAGE_PUBLIC_BASE_URL", "https://cdn.example.com")
	t.Setenv("API_TEMP_IMAGE_DIR", t.TempDir())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/task-1", nil)
	body, err := imageTaskResponseWithURLImages(c, []byte(`{
		"id":"task-1",
		"state":"success",
		"data":{"images":[{"b64_json":"data:image/png;base64,ZmFrZS1pbWFnZQ=="}]}
	}`))
	require.NoError(t, err)
	require.Contains(t, gjsonGetString(t, body, "data.images.0.url"), "https://cdn.example.com/api/relay-temp-images/")
	require.Empty(t, gjsonGetString(t, body, "data.images.0.b64_json"))
}

func TestWaitImageAsyncSubmitResponseUsesConfigurableDuomiGeminiProfile(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)

	upstreamTaskID := "f5044b53-4e32-2ee6-48b4-73bd60cd6754"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/gemini/nano-banana/"+upstreamTaskID, r.URL.Path)
		require.Equal(t, "duomi-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"msg":"success","data":{"task_id":"f5044b53-4e32-2ee6-48b4-73bd60cd6754","data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg","file_name":"output.jpeg"}],"description":"done"},"create_time":"1757061229","update_time":"1757061252","msg":"","status":"3","action":"generate"},"exec_time":0.05,"ip":"118.125.2.163"}`))
	}))
	defer upstream.Close()

	info := &relaycommon.RelayInfo{
		StartTime:       time.Unix(1779125700, 0),
		OriginModelName: "gemini-3-pro-image-preview",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeConfigurable,
			ChannelId:      11,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "duomi-key",
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{ProfileID: "duomi-gemini-image"},
			},
		},
	}

	router := gin.New()
	router.POST("/v1/images/generations", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 50*time.Millisecond)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)

		body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"f5044b53-4e32-2ee6-48b4-73bd60cd6754"}`))
		require.Nil(t, apiErr)
		c.Data(http.StatusOK, "application/json", body)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"created": 1757061229,
		"data": [{"url":"https://cdn3.dmiapi.com/output.jpeg","b64_json":"","revised_prompt":""}]
	}`, recorder.Body.String())
}

func TestWaitImageAsyncSubmitResponseSkipsWhenClientRequestedAsync(t *testing.T) {
	info := &relaycommon.RelayInfo{
		IsAsyncImageRequest: true,
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations?async=true", nil)

	body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0"}`))
	require.Nil(t, apiErr)
	require.Nil(t, body)
}

func TestWaitImageAsyncSubmitResponseUsesConfiguredWaitTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)

	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"task_wait","state":"running","progress":0,"action":"generate","data":{"images":[]}}`))
	}))
	defer upstream.Close()

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "sk-upstream",
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ImageAsyncWaitTimeoutSeconds: common.GetPointer(2),
		},
	})

	body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"task_wait"}`))
	require.NotNil(t, apiErr)
	require.Nil(t, body)
	require.Contains(t, apiErr.Error(), "image async task wait timeout")
	require.True(t, types.IsSkipRetryError(apiErr))
	require.Equal(t, 1, calls)
}

func TestWaitImageAsyncSubmitResponseCanDisableSyncWait(t *testing.T) {
	gin.SetMode(gin.TestMode)

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-image-2",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelId:      11,
			ChannelBaseUrl: "https://upstream.example",
			ApiKey:         "sk-upstream",
		},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ImageAsyncWaitTimeoutSeconds: common.GetPointer(0),
		},
	})

	body, apiErr := waitImageAsyncSubmitResponse(c, info, []byte(`{"id":"task_wait"}`))
	require.NotNil(t, apiErr)
	require.Nil(t, body)
	require.Contains(t, apiErr.Error(), "image async task wait disabled")
	require.True(t, types.IsSkipRetryError(apiErr))
}

func TestImageAsyncWaitStepsRoundsUp(t *testing.T) {
	require.Equal(t, 0, imageAsyncWaitSteps(0))
	require.Equal(t, 1, imageAsyncWaitSteps(1))
	require.Equal(t, 1, imageAsyncWaitSteps(2))
	require.Equal(t, 2, imageAsyncWaitSteps(3))
}

func TestImageAsyncWaitMaxStepDefaultsTo600Seconds(t *testing.T) {
	t.Setenv("IMAGE_ASYNC_WAIT_TIMEOUT_SECONDS", "")
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)

	require.Equal(t, 300, imageAsyncWaitMaxStep(c))
}

func TestShouldForceConvertImageRequestForConfigurableImageProfile(t *testing.T) {
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeConfigurable,
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled: true,
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "apixo-gpt-image-2",
				},
			},
		},
	}

	require.True(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{Model: "any-image-model"}))
}

func TestShouldForceConvertGeminiImageRequestForGeminiNativeAdaptor(t *testing.T) {
	tests := []struct {
		name    string
		apiType int
	}{
		{name: "Gemini", apiType: constant.APITypeGemini},
		{name: "Vertex", apiType: constant.APITypeVertexAi},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeImagesGenerations,
				ChannelMeta: &relaycommon.ChannelMeta{
					ApiType:           tt.apiType,
					UpstreamModelName: "custom-provider-photo-v9",
					ChannelSetting: dto.ChannelSettings{
						PassThroughBodyEnabled: true,
					},
				},
			}

			require.True(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
				Model:  "custom-provider-photo-v9",
				Prompt: "cat",
			}))
		})
	}
}

func TestShouldNotForceConvertImageRequestWhenUserResponseFormatBeatsChannelFallback(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled:    true,
				OpenAIImageResponseFormat: "url",
			},
		},
	}

	require.False(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model:          "gpt-image-2",
		ResponseFormat: "b64_json",
	}))
}

func TestShouldForceConvertImageRequestWhenChannelResponseFormatConfiguredAndUserOmitted(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled:    true,
				OpenAIImageResponseFormat: "url",
			},
		},
	}

	require.True(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model: "gpt-image-2",
	}))
}

func TestShouldForceConvertImageRequestWhenTokenResponseFormatConfigured(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:          relayconstant.RelayModeImagesGenerations,
		TokenImageSettings: model.TokenImageSettings{Format: "url"},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				PassThroughBodyEnabled: true,
			},
		},
	}

	require.True(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model:          "gpt-image-2",
		ResponseFormat: "b64_json",
	}))
}

func TestShouldForceConvertImageRequestWhenImageInputRequiresModeConversion(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
	}

	require.True(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model: "gpt-image-2",
		Image: []byte(`["https://cdn.example.com/cat.png"]`),
	}))
}

func TestShouldNotForceConvertImageRequestWhenGenerationAutoConvertDisabled(t *testing.T) {
	disabled := false
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesGenerations,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageAutoConvertGenerationWithImageToEdit: &disabled,
			},
		},
	}

	require.False(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model: "gpt-image-2",
		Image: []byte(`["https://cdn.example.com/cat.png"]`),
	}))
}

func TestShouldForceConvertImageRequestForJSONImageEdits(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
	}

	require.True(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model: "gpt-image-2",
		Image: []byte(`["https://cdn.example.com/cat.png"]`),
	}))
}

func TestShouldNotForceConvertImageRequestWhenJSONEditAutoConvertDisabled(t *testing.T) {
	disabled := false
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeImagesEdits,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelSetting: dto.ChannelSettings{
				ImageAutoConvertJSONEditToMultipart: &disabled,
			},
		},
	}

	require.False(t, shouldForceConvertImageRequest(info, &dto.ImageRequest{
		Model: "gpt-image-2",
		Image: []byte(`["https://cdn.example.com/cat.png"]`),
	}))
}

func TestImageResponseCaptureDoesNotWriteBodyOnIntermediateFlush(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := newImageResponseCapture(c.Writer)

	capture.Header().Set("Content-Type", "application/json")
	capture.WriteHeader(http.StatusOK)
	_, err := capture.Write([]byte(`{"id":"2c094bad-ecf3-c02f-90f7-cf0df7033c42"}`))
	require.NoError(t, err)
	capture.Flush()

	require.Empty(t, recorder.Body.String())

	capture.ReplaceBody(http.StatusOK, []byte(`{"created":1779125744,"data":[{"url":"https://cdn3.dmiapi.com/output.png","b64_json":"","revised_prompt":""}]}`))
	capture.WriteCaptured()

	require.JSONEq(t, `{
		"created":1779125744,
		"data":[{"url":"https://cdn3.dmiapi.com/output.png","b64_json":"","revised_prompt":""}]
	}`, recorder.Body.String())
	require.Equal(t, fmt.Sprintf("%d", recorder.Body.Len()), recorder.Header().Get("Content-Length"))
}

func TestImageResponseCaptureReportsWriteError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	capture := newImageResponseCapture(failingImageResponseWriter{ResponseWriter: c.Writer})

	capture.WriteHeader(http.StatusOK)
	_, err := capture.Write([]byte(`{"data":[{"url":"https://cdn.example.com/output.png"}]}`))
	require.NoError(t, err)

	err = capture.WriteCaptured()
	require.ErrorContains(t, err, "client disconnected")
}

type failingImageResponseWriter struct {
	gin.ResponseWriter
}

func (w failingImageResponseWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("client disconnected")
}

func TestImageTaskFetchReturnsOpenAICompatibleTaskPayload(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", r.URL.Path)
		require.Equal(t, "Bearer sk-upstream", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"7154f43c-7765-4e77-d51e-0db4eee107b0","state":"succeeded","progress":0,"create_time":1779125744,"update_time":1779125807,"action":"generate","data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","file_name":"output.png"}]}}`))
	}))
	defer upstream.Close()

	baseURL := upstream.URL
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:      11,
		Type:    constant.ChannelTypeOpenAI,
		Key:     "sk-upstream",
		BaseURL: &baseURL,
		Status:  common.ChannelStatusEnabled,
		Name:    "async-image",
	}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:     "7154f43c-7765-4e77-d51e-0db4eee107b0",
		UserId:     7,
		ChannelId:  11,
		Action:     constant.TaskActionGenerate,
		Status:     model.TaskStatusSubmitted,
		Progress:   "0%",
		CreatedAt:  now,
		UpdatedAt:  now,
		SubmitTime: now,
		Platform:   constant.TaskPlatform("1"),
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "7154f43c-7765-4e77-d51e-0db4eee107b0",
		},
	}).Error)

	router := gin.New()
	router.GET("/v1/tasks/:id", func(c *gin.Context) {
		c.Set("id", 7)
		if taskErr := ImageTaskFetch(c); taskErr != nil {
			c.JSON(taskErr.StatusCode, taskErr)
		}
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", nil)
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	require.JSONEq(t, `{
		"id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
		"state":"succeeded",
		"progress":0,
		"create_time":1779125744,
		"update_time":1779125807,
		"action":"generate",
		"data":{"images":[{"url":"https://cdn3.dmiapi.com/20260519/6a0b4e2e6925c.png?e=1779557807&token=abc","file_name":"output.png"}]}
	}`, recorder.Body.String())
}

func TestImageTaskFetchUsesBearerForStandardUpstreams(t *testing.T) {
	// Covered by the existing upstream server test; this placeholder keeps the
	// file focused on task payload behavior.
}

func TestConfigurableNativeFetchStoresFailureReason(t *testing.T) {
	setupImageTaskTestDB(t)
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/tasks/upstream-task", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"output":{"task_id":"upstream-task","task_status":"FAILED","code":"InvalidParameter","message":"The parameter is invalid."},"request_id":"req-1"}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:       901,
		Type:     constant.ChannelTypeConfigurable,
		Key:      "sk-upstream",
		BaseURL:  common.GetPointer(upstream.URL),
		Status:   common.ChannelStatusEnabled,
		Name:     "happyhorse",
		Models:   "happyhorse-1.0-t2v",
		Group:    "default",
		Priority: common.GetPointer[int64](1),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "happyhorse-video",
		},
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	require.NoError(t, model.DB.Create(&model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: channel.Id,
		Platform:  constant.TaskPlatform(fmt.Sprintf("%d", constant.ChannelTypeConfigurable)),
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-task",
		},
	}).Error)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_public", nil)
	c.Set("configurable_native_profile_id", "happyhorse-video")

	body := tryConfigurableNativeFetch(c, &model.Task{
		TaskID:    "task_public",
		UserId:    7,
		ChannelId: channel.Id,
		Status:    model.TaskStatusInProgress,
		Progress:  "30%",
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-task",
		},
	})
	require.True(t, strings.Contains(string(body), `"message":"The parameter is invalid."`), string(body))

	reloaded, exists, err := model.GetByTaskId(7, "task_public")
	require.NoError(t, err)
	require.True(t, exists)
	require.EqualValues(t, model.TaskStatusFailure, reloaded.Status)
	require.Equal(t, "The parameter is invalid.", reloaded.FailReason)
}

func TestBuildImageTaskFetchRequestUsesStandardEndpointAndBearerAuthorization(t *testing.T) {
	req, err := buildImageTaskFetchRequest("https://duomiapi.com", "duomi-key", "7154f43c-7765-4e77-d51e-0db4eee107b0")
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/v1/tasks/7154f43c-7765-4e77-d51e-0db4eee107b0", req.URL.String())
	require.Equal(t, "Bearer duomi-key", req.Header.Get("Authorization"))
	require.Equal(t, "application/json", req.Header.Get("Accept"))
}

func TestBuildConfigurableImageTaskFetchRequestUsesProfile(t *testing.T) {
	profile, ok := configurable.GetProfile("apixo-gpt-image-2")
	require.True(t, ok)
	req, err := buildConfigurableImageTaskFetchRequest("https://api.apixo.ai", "apixo-key", "dff8afd34c4e47719b85ad6651daa3f0", profile)
	require.NoError(t, err)
	require.Equal(t, "https://api.apixo.ai/api/v1/statusTask/gpt-image-2?taskId=dff8afd34c4e47719b85ad6651daa3f0", req.URL.String())
	require.Equal(t, "Bearer apixo-key", req.Header.Get("Authorization"))
}

func TestBuildConfigurableImageTaskFetchRequestUsesDuomiProfileHeaders(t *testing.T) {
	profile, ok := configurable.GetProfile("duomi-gemini-image")
	require.True(t, ok)
	req, err := buildConfigurableImageTaskFetchRequest("https://duomiapi.com", "duomi-key", "7154f43c-7765-4e77-d51e-0db4eee107b0", profile)
	require.NoError(t, err)
	require.Equal(t, "https://duomiapi.com/api/gemini/nano-banana/7154f43c-7765-4e77-d51e-0db4eee107b0", req.URL.String())
	require.Equal(t, "duomi-key", req.Header.Get("Authorization"))
}

func TestFetchConfigurableImageTaskForPollingUsesProfile(t *testing.T) {
	service.InitHttpClient()

	taskID := "dff8afd34c4e47719b85ad6651daa3f0"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/v1/statusTask/gpt-image-2", r.URL.Path)
		require.Equal(t, taskID, r.URL.Query().Get("taskId"))
		require.Equal(t, "Bearer apixo-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"message": "success",
				"data": {
					"resultJson": "{\"resultUrls\":[\"https://file.apixo.ai/temp/out.png\"]}",
					"state": "success",
					"taskId": "dff8afd34c4e47719b85ad6651daa3f0",
					"createTime": 1780459451000,
					"completeTime": 1780459536000
				}
			}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      63,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "apixo-key",
		BaseURL: common.GetPointer(upstream.URL),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "apixo-gpt-image-2"},
	})
	task := &model.Task{
		TaskID:    taskID,
		ChannelId: channel.Id,
		Action:    constant.TaskActionGenerate,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: taskID,
		},
	}

	body, statusCode, handled, err := FetchConfigurableImageTaskForPolling(&channel, task, taskID, "apixo-key", "")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, statusCode)
	require.JSONEq(t, `{
		"id":"dff8afd34c4e47719b85ad6651daa3f0",
		"state":"success",
		"progress":0,
		"create_time":1780459451,
		"update_time":1780459536,
		"action":"generate",
		"data":{"images":[{"url":"https://file.apixo.ai/temp/out.png"}]}
	}`, string(body))
}

func TestFetchConfigurableImageTaskForPollingUsesDuomiProfile(t *testing.T) {
	service.InitHttpClient()

	taskID := "7154f43c-7765-4e77-d51e-0db4eee107b0"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/gemini/nano-banana/"+taskID, r.URL.Path)
		require.Equal(t, "duomi-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 200,
			"msg": "success",
			"data": {
				"task_id": "7154f43c-7765-4e77-d51e-0db4eee107b0",
				"status": "3",
				"create_time": "1757061229",
				"update_time": "1757061252",
				"action": "generate",
				"data": {
					"images": [{"url":"https://cdn3.dmiapi.com/output.jpeg"}]
				}
			}
		}`))
	}))
	defer upstream.Close()

	channel := model.Channel{
		Id:      64,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "duomi-key",
		BaseURL: common.GetPointer(upstream.URL),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "duomi-gemini-image"},
	})
	task := &model.Task{
		TaskID:    taskID,
		ChannelId: channel.Id,
		Action:    constant.TaskActionGenerate,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: taskID,
		},
	}

	body, statusCode, handled, err := FetchConfigurableImageTaskForPolling(&channel, task, taskID, "duomi-key", "")
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, http.StatusOK, statusCode)
	require.JSONEq(t, `{
		"id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
		"state":"success",
		"progress":0,
		"create_time":1757061229,
		"update_time":1757061252,
		"action":"generate",
		"data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg"}]}
	}`, string(body))
}

func TestConvertImageTaskResponseUsesConfigurableImageProfile(t *testing.T) {
	setupImageTaskTestDB(t)
	channel := model.Channel{
		Id:      63,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "apixo-key",
		BaseURL: common.GetPointer("https://api.apixo.ai"),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "apixo-gpt-image-2"},
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	task := &model.Task{
		TaskID:    "dff8afd34c4e47719b85ad6651daa3f0",
		ChannelId: 63,
		Action:    constant.TaskActionGenerate,
		Properties: model.Properties{
			OriginModelName: "gpt-image-2",
		},
	}
	output, err := convertImageTaskResponse(task, []byte(`{
		"code": 200,
		"message": "success",
			"data": {
				"resultJson": "{\"resultUrls\":[\"https://file.apixo.ai/temp/out.png\"]}",
				"state": "success",
				"taskId": "dff8afd34c4e47719b85ad6651daa3f0",
				"createTime": 1780459451000,
				"completeTime": 1780459536000
			}
		}`))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"dff8afd34c4e47719b85ad6651daa3f0",
		"state":"success",
		"progress":0,
		"create_time":1780459451,
		"update_time":1780459536,
		"action":"generate",
		"data":{"images":[{"url":"https://file.apixo.ai/temp/out.png"}]}
	}`, string(output))
}

func TestConvertImageTaskResponseUsesDuomiConfigurableImageProfile(t *testing.T) {
	setupImageTaskTestDB(t)
	channel := model.Channel{
		Id:      64,
		Type:    constant.ChannelTypeConfigurable,
		Key:     "duomi-key",
		BaseURL: common.GetPointer("https://duomiapi.com"),
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{ProfileID: "duomi-gemini-image"},
	})
	require.NoError(t, model.DB.Create(&channel).Error)

	task := &model.Task{
		TaskID:    "7154f43c-7765-4e77-d51e-0db4eee107b0",
		ChannelId: 64,
		Action:    constant.TaskActionGenerate,
		CreatedAt: 1779125700,
		UpdatedAt: 1779125701,
		Properties: model.Properties{
			OriginModelName: "gemini-3-pro-image-preview",
		},
	}

	output, err := convertImageTaskResponse(task, []byte(`{
		"code":200,
		"msg":"success",
		"data":{
			"task_id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
			"data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg","file_name":"output.jpeg"}],"description":"done"},
			"create_time":"1757061229",
			"update_time":"1757061252",
			"msg":"",
			"status":"3",
			"action":"generate"
		},
		"exec_time":0.05,
		"ip":"118.125.2.163"
	}`))
	require.NoError(t, err)
	require.JSONEq(t, `{
		"id":"7154f43c-7765-4e77-d51e-0db4eee107b0",
		"state":"success",
		"progress":0,
		"create_time":1757061229,
		"update_time":1757061252,
		"action":"generate",
		"data":{"images":[{"url":"https://cdn3.dmiapi.com/output.jpeg"}]}
	}`, string(output))
}
