package doubao

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

func buildSeedanceOptionsBody(t *testing.T, payload map[string]any, native bool) []byte {
	t.Helper()
	raw, err := common.Marshal(payload)
	require.NoError(t, err)
	path := "/v1/video/generations"
	if native {
		path = "/api/v3/contents/generations/tasks"
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{OriginModelName: "video-alias", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVolcEngine, UpstreamModelName: "ep-upstream", IsModelMapped: true}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "ep-upstream", gjson.GetBytes(body, "model").String())
	return body
}

func TestSeedanceNativeAndGenericPreserveOptionsAndContent(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, flag := range []bool{false, true} {
			options := map[string]any{
				"watermark": flag, "return_last_frame": flag, "generate_audio": flag, "camera_fixed": flag, "draft": flag,
				"seed": 0, "priority": 0, "frames": 121, "execution_expires_after": 3600,
				"callback_url": "https://example.com/callback", "safety_identifier": "hashed-user", "service_tier": "default", "output_format": "mov",
				"tools":   []any{map[string]any{"type": "web_search"}},
				"content": []any{map[string]any{"type": "text", "text": "first"}, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "asset://image"}, "role": "first_frame"}, map[string]any{"type": "text", "text": "second"}},
			}
			payload := map[string]any{"model": "public-alias", "prompt": "must not replace full content", "size": "720p", "metadata": options}
			if native {
				payload = map[string]any{"model": "public-alias", "resolution": "720p"}
				for key, value := range options {
					payload[key] = value
				}
			}
			body := buildSeedanceOptionsBody(t, payload, native)
			for key, value := range options {
				encoded, err := common.Marshal(value)
				require.NoError(t, err)
				got := gjson.GetBytes(body, key)
				require.True(t, got.Exists(), "%s omitted: %s", key, body)
				require.JSONEq(t, string(encoded), got.Raw, key)
			}
			require.Equal(t, "720p", gjson.GetBytes(body, "resolution").String())
		}
		body := buildSeedanceOptionsBody(t, map[string]any{"model": "public-alias", "content": []any{map[string]any{"type": "draft_task", "draft_task": map[string]any{"id": "draft-upstream"}}}, "metadata": map[string]any{"content": []any{map[string]any{"type": "draft_task", "draft_task": map[string]any{"id": "draft-upstream"}}}}}, native)
		require.Equal(t, "draft-upstream", gjson.GetBytes(body, "content.0.draft_task.id").String())
	}
}

func TestSeedanceGenericShorthandAndOmittedOptions(t *testing.T) {
	body := buildSeedanceOptionsBody(t, map[string]any{"model": "public-alias", "metadata": map[string]any{"resolution": "1080p", "video_url": []string{"asset://v1", "asset://v2"}, "audio_url": []string{"asset://a1", "asset://a2"}}}, false)
	require.Equal(t, int64(4), gjson.GetBytes(body, "content.#").Int())
	require.Equal(t, "asset://a2", gjson.GetBytes(body, "content.3.audio_url.url").String())
	require.Equal(t, "1080p", gjson.GetBytes(body, "resolution").String())
	for _, key := range []string{"seed", "priority", "watermark", "return_last_frame", "generate_audio", "camera_fixed", "draft", "frames", "output_format", "tools"} {
		require.False(t, gjson.GetBytes(body, key).Exists(), key)
	}
}

func TestSeedanceGenericLastFrameAndTerminalFailures(t *testing.T) {
	a := &TaskAdaptor{}
	body, err := a.ConvertToOpenAIVideo(&model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Data: []byte(`{"status":"succeeded","content":{"video_url":"https://example.com/video.mp4","last_frame_url":"https://example.com/last.png"},"usage":{"completion_tokens":100,"total_tokens":100,"tool_usage":{"web_search":0}},"seed":0,"generate_audio":false,"output_format":"mov"}`)})
	require.NoError(t, err)
	require.Equal(t, "https://example.com/last.png", gjson.GetBytes(body, "metadata.last_frame_url").String())
	require.Equal(t, "0", gjson.GetBytes(body, "metadata.usage.tool_usage.web_search").Raw)
	for _, status := range []string{"expired", "cancelled", "failed"} {
		raw, err := common.Marshal(map[string]any{"status": status})
		require.NoError(t, err)
		result, err := a.ParseTaskResult(raw)
		require.NoError(t, err)
		require.Equal(t, string(model.TaskStatusFailure), result.Status)
		require.NotEmpty(t, result.Reason)
	}
}

func TestSeedanceEstimateUsesForwardedSizeAndMedia(t *testing.T) {
	for _, tc := range []struct {
		content   any
		wantRatio float64
	}{
		{nil, 31.0 / 46.0},
		{[]any{}, 31.0 / 46.0},
		{[]any{map[string]any{"type": "text", "text": "ignore shortcut video"}}, 51.0 / 46.0},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("task_request", relaycommon.TaskSubmitReq{Size: "1080p", Duration: 5, Metadata: map[string]any{"resolution": "720p", "content": tc.content, "video_url": "asset://video"}})
		ratios := (&TaskAdaptor{}).EstimateBilling(c, &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-0-260128"})
		require.InDelta(t, tc.wantRatio, ratios["video_input"], 0.00001)
	}
}
