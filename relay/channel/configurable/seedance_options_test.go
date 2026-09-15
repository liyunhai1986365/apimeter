package configurable

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

var seedanceOptionProfiles = []string{
	"doubao-seedance-2", "doubao-seedance-2-api-assets", "seedance2-modelsell",
	"seedance2-ark-task-assets", "seedance-tgxmaas", "seedance2-service-inference", "doubao-seedance-max-service-inference",
}

func TestSeedanceOptionsReachUpstreamAcrossProfiles(t *testing.T) {
	ginBody := func(t *testing.T, profile, path string, payload map[string]any) []byte {
		t.Helper()
		info := seedanceMaxRelayInfo(profile)
		raw, err := common.Marshal(payload)
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		c.Request.Header.Set("Content-Type", "application/json")
		a := &TaskAdaptor{}
		a.Init(info)
		require.Nil(t, a.ValidateRequestAndSetAction(c, info))
		reader, err := a.BuildRequestBody(c, info)
		require.NoError(t, err)
		body, err := io.ReadAll(reader)
		require.NoError(t, err)
		return body
	}
	for _, profile := range seedanceOptionProfiles {
		for _, flag := range []bool{false, true} {
			for _, mode := range []string{"generic", "native"} {
				t.Run(profile+"/"+mode+"/"+map[bool]string{true: "true", false: "false"}[flag], func(t *testing.T) {
					options := map[string]any{
						"watermark": flag, "return_last_frame": flag, "generate_audio": flag, "camera_fixed": flag, "draft": flag,
						"seed": 0, "priority": 0, "frames": 121, "execution_expires_after": 3600,
						"callback_url": "https://example.com/callback", "safety_identifier": "hashed-user",
						"service_tier": "default", "output_format": "mov", "tools": []any{map[string]any{"type": "web_search"}},
					}
					payload := map[string]any{"model": "public-video-alias", "prompt": "A scene", "size": "720p", "metadata": options}
					path := "/v1/video/generations"
					if mode == "native" {
						payload = map[string]any{"model": "public-video-alias", "content": []any{map[string]any{"type": "text", "text": "A scene"}}, "resolution": "720p"}
						for key, value := range options {
							payload[key] = value
						}
						path = "/api/v3/contents/generations/tasks"
					}
					body := ginBody(t, profile, path, payload)
					for key, value := range options {
						encoded, err := common.Marshal(value)
						require.NoError(t, err)
						got := gjson.GetBytes(body, key)
						require.True(t, got.Exists(), "%s omitted: %s", key, body)
						require.JSONEq(t, string(encoded), got.Raw, key)
					}
					require.Equal(t, "720p", gjson.GetBytes(body, "resolution").String())
					require.NotEqual(t, "public-video-alias", gjson.GetBytes(body, "model").String())
				})
			}
		}
		t.Run(profile+"/omitted", func(t *testing.T) {
			body := ginBody(t, profile, "/v1/video/generations", map[string]any{"model": "public-video-alias", "prompt": "A scene"})
			for _, key := range []string{"watermark", "return_last_frame", "generate_audio", "seed", "priority", "camera_fixed", "draft", "output_format", "frames", "tools", "callback_url", "service_tier", "execution_expires_after", "safety_identifier"} {
				require.False(t, gjson.GetBytes(body, key).Exists(), "%s default was injected", key)
			}
		})
		t.Run(profile+"/all shorthand media", func(t *testing.T) {
			body := ginBody(t, profile, "/v1/video/generations", map[string]any{"model": "public-video-alias", "metadata": map[string]any{
				"resolution": "1080p", "video_url": []string{"asset://v1", "asset://v2"}, "audio_url": []string{"asset://a1", "asset://a2"},
			}})
			require.Equal(t, int64(4), gjson.GetBytes(body, "content.#").Int())
			require.Equal(t, "asset://v2", gjson.GetBytes(body, "content.1.video_url.url").String())
			require.Equal(t, "asset://a2", gjson.GetBytes(body, "content.3.audio_url.url").String())
			require.Equal(t, "1080p", gjson.GetBytes(body, "resolution").String())
		})
		t.Run(profile+"/draft content", func(t *testing.T) {
			content := []any{map[string]any{"type": "draft_task", "draft_task": map[string]any{"id": "draft-upstream"}}}
			body := ginBody(t, profile, "/v1/video/generations", map[string]any{"model": "public-video-alias", "metadata": map[string]any{"content": content}})
			require.Equal(t, "draft-upstream", gjson.GetBytes(body, "content.0.draft_task.id").String())
		})
		t.Run(profile+"/terminal statuses", func(t *testing.T) {
			a := &TaskAdaptor{}
			a.Init(seedanceMaxRelayInfo(profile))
			for _, status := range []string{"expired", "cancelled", "canceled"} {
				snapshot := map[string]any{"id": "upstream", "status": status}
				payload := map[string]any{"id": "upstream", "status": status, "task": snapshot, "data": map[string]any{"task_id": "upstream", "status": status}}
				raw, err := common.Marshal(payload)
				require.NoError(t, err)
				result, err := a.ParseTaskResult(raw)
				require.NoError(t, err)
				require.Equal(t, string(model.TaskStatusFailure), result.Status)
			}
		})
	}
}

func TestOtherVideoProfilesStillRequirePrompt(t *testing.T) {
	info := seedanceMaxRelayInfo("generic-video-json")
	raw := []byte(`{"model":"other-video","metadata":{"audio_url":"asset://audio"}}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	a := &TaskAdaptor{}
	a.Init(info)
	require.NotNil(t, a.ValidateRequestAndSetAction(c, info))
	require.False(t, relaycommon.IsSeedanceVideoProfile("generic-video-json"))
}

func TestSeedanceGenericResultsPreserveNestedLastFrameAndZeroUsage(t *testing.T) {
	db := openConfigurableTaskAdaptorTestDB(t)
	for index, profile := range seedanceOptionProfiles {
		t.Run(profile, func(t *testing.T) {
			channel := model.Channel{Id: 9800 + index, Type: constant.ChannelTypeConfigurable, Status: common.ChannelStatusEnabled, Name: profile, Key: "test", Models: "seedance", Group: "default"}
			channel.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: profile}})
			require.NoError(t, db.Create(&channel).Error)
			task := &model.Task{TaskID: "task_public", ChannelId: channel.Id, Status: model.TaskStatusSuccess, Data: []byte(`{"task":{"status":"completed","outputs":["https://example.com/video.mov"],"usage":{"total_tokens":100},"metadata":{"content":{"last_frame_url":"https://example.com/last.png"},"output_format":"mov","seed":0,"priority":0,"generate_audio":false,"usage":{"completion_tokens":100,"tool_usage":{"web_search":0}},"secret":"must-not-leak"}}}`)}
			body, err := (&TaskAdaptor{}).ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			require.Equal(t, "https://example.com/last.png", gjson.GetBytes(body, "metadata.last_frame_url").String())
			require.Equal(t, "mov", gjson.GetBytes(body, "metadata.output_format").String())
			require.Equal(t, "0", gjson.GetBytes(body, "metadata.usage.tool_usage.web_search").Raw)
			require.Equal(t, "false", gjson.GetBytes(body, "metadata.generate_audio").Raw)
			require.NotContains(t, string(body), "must-not-leak")
		})
	}
}
