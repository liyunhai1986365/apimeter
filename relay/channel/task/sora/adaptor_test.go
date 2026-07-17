package sora

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokVideoRequestUsesJSONAndPreservesReferences(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"grok-imagine-video",
		"prompt":"animate the references",
		"seconds":"6",
		"resolution":"720p",
		"aspect_ratio":"16:9",
		"custom_domain":"true",
		"reference_images":[{"url":"https://example.com/reference.png"}],
		"video":{"url":"https://example.com/reference.mp4"}
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json; charset=utf-8")
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeXai,
			ChannelBaseUrl:    "https://api.x.ai",
			ApiKey:            "xai-test",
			UpstreamModelName: "grok-imagine-video",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	require.Equal(t, constant.TaskActionGenerate, info.Action)

	requestBody, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	upstreamBody, err := io.ReadAll(requestBody)
	require.NoError(t, err)
	require.Equal(t, "grok-imagine-video", gjson.GetBytes(upstreamBody, "model").String())
	require.Equal(t, "720p", gjson.GetBytes(upstreamBody, "resolution").String())
	require.Equal(t, "16:9", gjson.GetBytes(upstreamBody, "aspect_ratio").String())
	require.Equal(t, "true", gjson.GetBytes(upstreamBody, "custom_domain").String())
	require.Equal(t, "https://example.com/reference.png", gjson.GetBytes(upstreamBody, "reference_images.0.url").String())
	require.Equal(t, "https://example.com/reference.mp4", gjson.GetBytes(upstreamBody, "video.url").String())

	upstreamRequest := httptest.NewRequest(http.MethodPost, "/v1/videos", requestBody)
	require.NoError(t, adaptor.BuildRequestHeader(c, upstreamRequest, info))
	require.Equal(t, "application/json", upstreamRequest.Header.Get("Content-Type"))
}

func TestGrokVideoRequestRejectsMultipart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", bytes.NewBufferString("model=grok-imagine-video"))
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=test")
	_, err := common.GetBodyStorage(c)
	require.NoError(t, err)

	info := &relaycommon.RelayInfo{
		OriginModelName: "grok-imagine-video",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video"},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{},
	}
	adaptor := &TaskAdaptor{}
	taskErr := adaptor.ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	require.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	require.Equal(t, "invalid_request", taskErr.Code)
}

func TestParseTaskResultUsesCompletedVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"task_upstream",
		"status":"completed",
		"video_url":"https://cdn.example.com/video.mp4"
	}`))

	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	require.Equal(t, "https://cdn.example.com/video.mp4", result.Url)
}

func TestParseTaskResultLeavesCompletedURLBlankWhenMissing(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"task_upstream",
		"status":"completed"
	}`))

	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	require.Empty(t, result.Url)
}
