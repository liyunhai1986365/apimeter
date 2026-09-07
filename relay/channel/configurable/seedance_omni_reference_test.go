package configurable

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSeedanceOmniReferenceAcrossProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, profileID := range []string{
		"seedance2-service-inference", "doubao-seedance-max-service-inference",
		"doubao-seedance-2", "doubao-seedance-2-api-assets", "seedance2-modelsell", "seedance2-ark-task-assets",
	} {
		for _, mode := range []string{"native", "generic", "generic_metadata"} {
			for _, taskType := range []string{"", "auto", "reference", "edit", "extend"} {
				t.Run(profileID+"/"+mode+"/"+taskType, func(t *testing.T) {
					info := seedanceRelayInfo("doubao-seedance-2-5-260628")
					info.ChannelSetting.Protocol.ProfileID = profileID
					// Alias recognition must survive both validation passes.
					info.OriginModelName, info.UpstreamModelName = "video-alias", "ep-upstream"
					content := []any{
						map[string]any{"type": "text", "text": "Edit the reference video"},
						map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "asset://reference"}},
						map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": "asset://style"}},
					}
					duration := 5
					if taskType == "edit" {
						duration = -1
					}
					payload := map[string]any{"model": "video-alias", "duration": duration, "ratio": "adaptive", "content": content, "generate_audio": false, "watermark": false}
					path := "/api/v3/contents/generations/tasks"
					if mode != "native" {
						path = "/v1/video/generations"
						payload = map[string]any{"model": "video-alias", "prompt": "Edit the reference video", "duration": duration,
							"metadata": map[string]any{"content": content, "ratio": "adaptive", "generate_audio": false, "watermark": false}}
					}
					if taskType != "" {
						if mode == "generic_metadata" {
							payload["metadata"].(map[string]any)["omni_reference_task_type"] = taskType
							delete(payload["metadata"].(map[string]any), "ratio")
							payload["metadata"].(map[string]any)["aspect_ratio"] = "adaptive"
							if taskType == "edit" {
								delete(payload, "duration")
								payload["seconds"] = "-1"
							}
						} else {
							payload["omni_reference_task_type"] = taskType
						}
					}
					body, err := common.Marshal(payload)
					require.NoError(t, err)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
					c.Request.Header.Set("Content-Type", "application/json")
					a := &TaskAdaptor{}
					a.Init(info)
					require.Nil(t, a.ValidateRequestAndSetAction(c, info))
					parsed, err := relaycommon.GetTaskRequest(c)
					require.NoError(t, err)
					require.Nil(t, relaycommon.ValidateTaskDurationBoundsForRelay(parsed, info))
					reader, err := a.BuildRequestBody(c, info)
					require.NoError(t, err)
					upstream, err := io.ReadAll(reader)
					require.NoError(t, err)
					require.Equal(t, taskType != "", gjson.GetBytes(upstream, "omni_reference_task_type").Exists())
					require.Equal(t, taskType, gjson.GetBytes(upstream, "omni_reference_task_type").String())
					require.Equal(t, int64(duration), gjson.GetBytes(upstream, "duration").Int())
					require.Equal(t, "adaptive", gjson.GetBytes(upstream, "ratio").String())
					expectedContent, err := common.Marshal(content)
					require.NoError(t, err)
					require.JSONEq(t, string(expectedContent), gjson.GetBytes(upstream, "content").Raw)
					require.Equal(t, "false", gjson.GetBytes(upstream, "generate_audio").Raw)
					estimatedSeconds := float64(duration)
					if duration == -1 {
						estimatedSeconds = relaycommon.MaxSeedanceTaskDurationSeconds
					}
					require.Equal(t, estimatedSeconds, a.EstimateBilling(c, info)["seconds"])
				})
			}
		}
	}
}

func TestSeedanceNativeRejectsInvalidOmniReferenceType(t *testing.T) {
	for _, taskType := range []string{"", "invalid"} {
		info := seedanceServiceInferenceRelayInfo("dreamina-seedance-2-5-260628-max")
		body, err := common.Marshal(map[string]any{"model": info.OriginModelName, "omni_reference_task_type": taskType,
			"content": []any{map[string]any{"type": "text", "text": "test"}}, "duration": 5})
		require.NoError(t, err)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		a := &TaskAdaptor{}
		a.Init(info)
		taskErr := a.ValidateRequestAndSetAction(c, info)
		require.NotNil(t, taskErr)
		require.Equal(t, "invalid_request", taskErr.Code)
	}
}
