package doubao

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOmniReferenceTaskForwarding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{"native", "generic", "generic_metadata"} {
		for _, taskType := range []string{"auto", "reference", "edit", "extend"} {
			t.Run(mode+"/"+taskType, func(t *testing.T) {
				info := &relaycommon.RelayInfo{OriginModelName: "video-alias", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeVolcEngine, UpstreamModelName: "ep-upstream"}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
				duration := 5
				if taskType == "edit" {
					duration = -1
				}
				content := []any{map[string]any{"type": "text", "text": "Edit the reference video"}, map[string]any{"type": "video_url", "role": "reference_video", "video_url": map[string]any{"url": "asset://source"}}}
				body := map[string]any{"model": "video-alias", "duration": duration, "omni_reference_task_type": taskType, "content": content, "ratio": "adaptive", "watermark": false}
				path := "/api/v3/contents/generations/tasks"
				if mode != "native" {
					path = "/v1/video/generations"
					body = map[string]any{"model": "video-alias", "prompt": "Edit the reference video", "duration": duration, "omni_reference_task_type": taskType, "metadata": map[string]any{"content": content, "ratio": "adaptive", "watermark": false}}
					if mode == "generic_metadata" {
						delete(body, "omni_reference_task_type")
						metadata := body["metadata"].(map[string]any)
						metadata["omni_reference_task_type"] = taskType
						delete(metadata, "ratio")
						metadata["aspect_ratio"] = "adaptive"
						delete(metadata, "content")
						metadata["video_url"] = []string{"asset://source"}
						delete(body, "duration")
						body["seconds"] = "5"
						if duration == -1 {
							body["seconds"] = "-1"
						}
					}
				}
				raw, err := common.Marshal(body)
				require.NoError(t, err)
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
				c.Request.Header.Set("Content-Type", "application/json")
				a := &TaskAdaptor{}
				a.Init(info)
				require.Nil(t, a.ValidateRequestAndSetAction(c, info))
				reader, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				upstream, err := io.ReadAll(reader)
				require.NoError(t, err)
				require.Equal(t, taskType, gjson.GetBytes(upstream, "omni_reference_task_type").String())
				require.Equal(t, int64(duration), gjson.GetBytes(upstream, "duration").Int())
				require.Equal(t, "adaptive", gjson.GetBytes(upstream, "ratio").String())
				require.Equal(t, "asset://source", gjson.GetBytes(upstream, `content.#(role=="reference_video").video_url.url`).String())
				require.Equal(t, "false", gjson.GetBytes(upstream, "watermark").Raw)
				estimate := float64(duration)
				if duration == -1 {
					estimate = relaycommon.MaxSeedanceTaskDurationSeconds
				}
				require.Equal(t, estimate, a.EstimateBilling(c, info)["seconds"])
			})
		}
	}
}
