package configurable

import (
	"bytes"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHNativeVideoPreservesRequest(t *testing.T) {
	for _, content := range []string{`[{"type":"text","text":"first"},{"type":"text","text":"second"}]`, `[{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/a.png","extra":0}}]`} {
		body := `{"model":"client-alias","content":` + content + `,"seed":0,"return_last_frame":false,"generate_audio":false,"duration":-1,"resolution":"1080p","execution_expires_after":0,"extension":{"large":9007199254740993,"small":0.0000000000000000001}}`
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/api/v3/contents/generations/tasks", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		info := seedanceMaxRelayInfo("seedance-yh")
		a := &TaskAdaptor{}
		a.Init(info)
		require.Nil(t, a.ValidateRequestAndSetAction(c, info))
		reader, err := a.BuildRequestBody(c, info)
		require.NoError(t, err)
		wire, err := io.ReadAll(reader)
		require.NoError(t, err)
		require.Equal(t, content, gjson.GetBytes(wire, "metadata.content").Raw)
		for _, key := range []string{"seed", "return_last_frame", "generate_audio", "duration", "resolution", "execution_expires_after", "extension"} {
			require.Equal(t, gjson.Get(body, key).Raw, gjson.GetBytes(wire, "metadata."+key).Raw, key)
		}
		if gjson.Get(content, "0.type").String() == "text" {
			require.Equal(t, "first\nsecond", gjson.GetBytes(wire, "prompt").String())
		} else {
			require.False(t, gjson.GetBytes(wire, "prompt").Exists())
		}
		require.Equal(t, info.UpstreamModelName, gjson.GetBytes(wire, "model").String())
	}
	for _, base := range []string{"https://example.com", "https://example.com/", "https://example.com/v1", "https://example.com/v1/"} {
		require.Equal(t, "https://example.com/v1/video/generations", YHURL(base, "/v1/video/generations"))
	}
}

func TestYHVideoResponseAndValidation(t *testing.T) {
	a := &TaskAdaptor{}
	a.Init(seedanceMaxRelayInfo("seedance-yh"))
	body := []byte(`{"code":"success","data":{"task_id":"yh-task","status":"SUCCESS","fail_reason":"https://example.com/result.mp4","data":{"content":{"video_url":"https://example.com/v.mp4","last_frame_url":"https://example.com/f.png","extra":false},"usage":{"total_tokens":100,"completion_tokens":90,"extra":9007199254740993},"duration":5,"seed":0,"generate_audio":false,"resolution":"1080p"}}}`)
	info, err := a.ParseTaskResult(body)
	require.NoError(t, err)
	require.Equal(t, 100, info.TotalTokens)
	require.Equal(t, 90, info.CompletionTokens)
	require.Empty(t, info.Reason)
	require.NoError(t, a.ValidateTaskStatus(body))
	require.NoError(t, a.ValidateTaskIdentity(body, info, "yh-task", ""))
	require.Error(t, a.ValidateTaskIdentity(body, info, "another", ""))
	result, err := a.ConvertToNativeFetchResponse(&model.Task{Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "yh-task"}, Properties: model.Properties{OriginModelName: "client-alias"}}, body)
	require.NoError(t, err)
	for _, key := range []string{"content", "usage", "duration", "seed", "generate_audio", "resolution"} {
		require.Equal(t, gjson.GetBytes(body, "data.data."+key).Raw, gjson.GetBytes(result, key).Raw, key)
	}
	require.Equal(t, "succeeded", gjson.GetBytes(result, "status").String())
	require.Error(t, a.ValidateTaskStatus([]byte(`{"data":{"status":"NEW_UNKNOWN_STATUS"}}`)))
}

func TestYHGenericMetadataPreservesPrecision(t *testing.T) {
	body := `{"model":"client-alias","prompt":"hello","metadata":{"seed":9007199254740993,"return_last_frame":false,"extension":{"small":0.0000000000000000001},"content":[{"type":"text","text":"actual","extension":9007199254740993}]}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	info := seedanceMaxRelayInfo("seedance-yh")
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	wire, err := io.ReadAll(reader)
	require.NoError(t, err)
	for _, field := range []string{"seed", "return_last_frame", "content", "extension"} {
		require.Equal(t, gjson.Get(body, "metadata."+field).Raw, gjson.GetBytes(wire, "metadata."+field).Raw)
	}
	require.Equal(t, "actual", gjson.GetBytes(wire, "prompt").String())
}
