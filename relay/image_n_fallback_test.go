package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldSplitImageNAfterError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	n := uint(2)
	request := &dto.ImageRequest{Model: "gpt-image-2", N: &n}
	info := &relaycommon.RelayInfo{}
	currentError := types.WithOpenAIError(types.OpenAIError{
		Message: "请求参数无法处理, 请检查提示词/尺寸/参考图",
		Type:    "invalid_value",
	}, http.StatusBadRequest)

	require.True(t, shouldSplitImageNAfterError(nil, relayconstant.RelayModeImagesGenerations, info, request, currentError))

	tests := []struct {
		name      string
		mutate    func(*relaycommon.RelayInfo, *dto.ImageRequest)
		relayMode int
		apiErr    *types.NewAPIError
	}{
		{
			name: "single image",
			mutate: func(_ *relaycommon.RelayInfo, req *dto.ImageRequest) {
				req.N = common.GetPointer(uint(1))
			},
			relayMode: relayconstant.RelayModeImagesGenerations,
			apiErr:    currentError,
		},
		{
			name: "above official maximum",
			mutate: func(_ *relaycommon.RelayInfo, req *dto.ImageRequest) {
				req.N = common.GetPointer(uint(11))
			},
			relayMode: relayconstant.RelayModeImagesGenerations,
			apiErr:    currentError,
		},
		{
			name: "async request",
			mutate: func(info *relaycommon.RelayInfo, _ *dto.ImageRequest) {
				info.IsAsyncImageRequest = true
			},
			relayMode: relayconstant.RelayModeImagesGenerations,
			apiErr:    currentError,
		},
		{
			name:      "image edit",
			mutate:    func(_ *relaycommon.RelayInfo, _ *dto.ImageRequest) {},
			relayMode: relayconstant.RelayModeImagesEdits,
			apiErr:    currentError,
		},
		{
			name:      "unrelated invalid value",
			mutate:    func(_ *relaycommon.RelayInfo, _ *dto.ImageRequest) {},
			relayMode: relayconstant.RelayModeImagesGenerations,
			apiErr: types.WithOpenAIError(types.OpenAIError{
				Message: "unsupported image size",
				Type:    "invalid_value",
			}, http.StatusBadRequest),
		},
		{
			name:      "non-400 response",
			mutate:    func(_ *relaycommon.RelayInfo, _ *dto.ImageRequest) {},
			relayMode: relayconstant.RelayModeImagesGenerations,
			apiErr: types.WithOpenAIError(types.OpenAIError{
				Message: "请求参数无法处理, 请检查提示词/尺寸/参考图",
				Type:    "invalid_value",
			}, http.StatusUnprocessableEntity),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testInfo := *info
			testRequest := *request
			tt.mutate(&testInfo, &testRequest)
			require.False(t, shouldSplitImageNAfterError(nil, tt.relayMode, &testInfo, &testRequest, tt.apiErr))
		})
	}

	workerRecorder := httptest.NewRecorder()
	workerContext, _ := gin.CreateTestContext(workerRecorder)
	workerContext.Set(localImageAsyncWorkerKey, true)
	workerInfo := *info
	workerInfo.IsAsyncImageRequest = true
	require.True(t, shouldSplitImageNAfterError(workerContext, relayconstant.RelayModeImagesGenerations, &workerInfo, request, currentError))
}

func TestSubmitSplitImageRequestsAggregatesOfficialResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	var requestCount atomic.Int32
	var releaseOnce sync.Once
	var concurrencyTimeout atomic.Bool
	allRequestsStarted := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := int(requestCount.Add(1))
		var body map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &body))
		require.Equal(t, float64(1), body["n"])
		require.Equal(t, "preserved", body["custom_option"])
		require.Equal(t, "gpt-image-2", body["model"])
		if requestNumber == 3 {
			releaseOnce.Do(func() { close(allRequestsStarted) })
		}
		select {
		case <-allRequestsStarted:
		case <-time.After(time.Second):
			concurrencyTimeout.Store(true)
		}
		if requestNumber == 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"generation failed","type":"server_error"}}`))
			return
		}

		response := fmt.Sprintf(`{
			"created":%d,
			"data":[{"b64_json":"image-%d"}],
			"output_format":"png",
			"quality":"medium",
			"size":"1024x1024",
			"usage":{
				"input_tokens":%d,
				"output_tokens":%d,
				"total_tokens":%d,
				"input_tokens_details":{"text_tokens":%d,"image_tokens":0},
				"output_tokens_details":{"image_tokens":%d,"text_tokens":0}
			}
		}`, 100+requestNumber, requestNumber, requestNumber+2, requestNumber+4, requestNumber*2+6, requestNumber+2, requestNumber+4)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response))
	}))
	defer upstream.Close()

	n := uint(3)
	request := &dto.ImageRequest{
		Model:  "gpt-image-2",
		Prompt: "draw a pond",
		N:      &n,
		Size:   "1024x1024",
		Extra: map[string]json.RawMessage{
			"custom_option": []byte(`"preserved"`),
		},
	}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeImagesGenerations,
		RequestURLPath: "/v1/images/generations",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:           constant.APITypeOpenAI,
			ChannelType:       constant.ChannelTypeOpenAI,
			ChannelBaseUrl:    upstream.URL,
			ApiKey:            "sk-upstream",
			UpstreamModelName: "gpt-image-2",
		},
		PriceData: types.PriceData{
			UsePrice:    true,
			OtherRatios: map[string]float64{"n": 3},
		},
		BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"model":"gpt-image-2","n":3}`)},
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	capture, usage, apiErr := submitSplitImageRequests(c, info, request, c.Writer, "")
	require.Nil(t, apiErr)
	require.NotNil(t, capture)
	require.Equal(t, int32(3), requestCount.Load())
	require.False(t, concurrencyTimeout.Load(), "split image requests did not start concurrently")
	require.Equal(t, uint(2), *request.N)
	require.Equal(t, float64(2), info.PriceData.OtherRatios["n"])
	require.Equal(t, int64(2), gjson.GetBytes(info.BillingRequestInput.Body, "n").Int())
	require.Equal(t, 8, usage.InputTokens)
	require.Equal(t, 12, usage.OutputTokens)
	require.Equal(t, 20, usage.TotalTokens)
	require.Equal(t, 8, usage.PromptTokens)
	require.Equal(t, 12, usage.CompletionTokens)
	require.Equal(t, 8, usage.InputTokensDetails.TextTokens)
	require.Equal(t, 12, usage.OutputTokensDetails.ImageTokens)

	body := capture.BodyBytes()
	require.Contains(t, []int64{101, 103}, gjson.GetBytes(body, "created").Int())
	require.ElementsMatch(t, []string{"image-1", "image-3"}, []string{
		gjson.GetBytes(body, "data.0.b64_json").String(),
		gjson.GetBytes(body, "data.1.b64_json").String(),
	})
	require.Equal(t, int64(8), gjson.GetBytes(body, "usage.input_tokens").Int())
	require.Equal(t, int64(12), gjson.GetBytes(body, "usage.output_tokens").Int())
	require.Equal(t, int64(20), gjson.GetBytes(body, "usage.total_tokens").Int())
	require.Equal(t, "png", gjson.GetBytes(body, "output_format").String())
	require.Equal(t, "medium", gjson.GetBytes(body, "quality").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(body, "size").String())
}

func TestSubmitSplitImageRequestsReturnsErrorWhenAllFail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	var requestCount atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"generation failed","type":"invalid_value"}}`))
	}))
	defer upstream.Close()

	n := uint(2)
	request := &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw a pond", N: &n}
	info := &relaycommon.RelayInfo{
		RelayMode:      relayconstant.RelayModeImagesGenerations,
		RequestURLPath: "/v1/images/generations",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiType:        constant.APITypeOpenAI,
			ChannelType:    constant.ChannelTypeOpenAI,
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "sk-upstream",
		},
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Request.Header.Set("Content-Type", "application/json")

	capture, usage, apiErr := submitSplitImageRequests(c, info, request, c.Writer, "")
	require.Nil(t, capture)
	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, int32(2), requestCount.Load())
	require.Equal(t, uint(2), *request.N)
}

func TestApplySplitImageSuccessCountDoesNotDoubleCountRatioBilling(t *testing.T) {
	n := uint(3)
	request := &dto.ImageRequest{N: &n}
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			OtherRatios: map[string]float64{"n": 3, "quality": 2},
		},
		BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"n":3,"quality":"high"}`)},
	}

	applySplitImageSuccessCount(nil, info, request, 2)

	require.Equal(t, uint(2), *request.N)
	_, hasN := info.PriceData.OtherRatios["n"]
	require.False(t, hasN)
	require.Equal(t, float64(2), info.PriceData.OtherRatios["quality"])
	require.Equal(t, int64(2), gjson.GetBytes(info.BillingRequestInput.Body, "n").Int())
}

func TestImageHelperFallsBackFromRejectedBatchToSingleSubmissions(t *testing.T) {
	setupImageTaskTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.User{}))
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "image-n-user", Quota: 10000, Status: common.UserStatusEnabled}).Error)
	service.InitHttpClient()
	gin.SetMode(gin.TestMode)

	var requestCount atomic.Int32
	images := []string{
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
		"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=",
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestNumber := int(requestCount.Add(1))
		var requestBody map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &requestBody))
		require.Equal(t, "gpt-image-2-count", requestBody["model"])
		n := int(requestBody["n"].(float64))
		w.Header().Set("Content-Type", "application/json")
		if n > 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"请求参数无法处理, 请检查提示词/尺寸/参考图","type":"invalid_value","param":"","code":null}}`))
			return
		}
		if requestNumber == 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":{"message":"generation failed","type":"server_error"}}`))
			return
		}
		_, _ = w.Write([]byte(fmt.Sprintf(`{"created":123,"data":[{"b64_json":"%s"}],"usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}`, images[requestNumber-2])))
	}))
	defer upstream.Close()

	n := uint(3)
	imageRequest := &dto.ImageRequest{
		Model:          "gpt-image-2-count",
		Prompt:         "draw a pond",
		N:              &n,
		Size:           "1024x1024",
		ResponseFormat: "b64_json",
	}
	info := &relaycommon.RelayInfo{
		UserId:           1,
		OriginModelName:  "gpt-image-2-count",
		CurrentModelName: "gpt-image-2-count",
		RelayMode:        relayconstant.RelayModeImagesGenerations,
		RequestURLPath:   "/v1/images/generations",
		Request:          imageRequest,
		Billing:          &imageAsyncBillingStub{},
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
			UsePrice:        true,
			OtherRatios:     map[string]float64{"n": 3},
		},
		BillingRequestInput: &billingexpr.RequestInput{Body: []byte(`{"model":"gpt-image-2-count","n":3}`)},
	}
	body := []byte(`{"model":"gpt-image-2-count","prompt":"draw a pond","n":3,"response_format":"b64_json","size":"1024x1024"}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
	common.SetContextKey(c, constant.ContextKeyChannelId, 11)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "sk-upstream")
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	storage, err := common.CreateBodyStorage(body)
	require.NoError(t, err)
	defer storage.Close()
	c.Set(common.KeyBodyStorage, storage)

	require.Nil(t, ImageHelper(c, info))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, int32(4), requestCount.Load())
	require.Equal(t, images[0], gjson.GetBytes(recorder.Body.Bytes(), "data.0.b64_json").String())
	require.Equal(t, images[2], gjson.GetBytes(recorder.Body.Bytes(), "data.1.b64_json").String())
	require.Equal(t, float64(2), info.PriceData.OtherRatios["n"])
	require.Equal(t, int64(2), gjson.GetBytes(info.BillingRequestInput.Body, "n").Int())
	require.Equal(t, int64(2), gjson.GetBytes(recorder.Body.Bytes(), "usage.input_tokens").Int())
	require.Equal(t, int64(4), gjson.GetBytes(recorder.Body.Bytes(), "usage.output_tokens").Int())
	require.Equal(t, int64(6), gjson.GetBytes(recorder.Body.Bytes(), "usage.total_tokens").Int())
}
