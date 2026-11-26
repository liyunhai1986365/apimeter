package configurable

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMagiProtocol(t *testing.T) {
	for _, provider := range []struct{ id, key, auth string }{
		{relaycommon.HiggsfieldMagiProfileID, "id:secret", "Key id:secret"},
		{relaycommon.ModelSellMagiProfileID, "gateway-key", "Bearer gateway-key"},
	} {
		t.Run(provider.id, func(t *testing.T) {
			for _, tc := range []struct{ mode, input, upstream string }{
				{"text-to-video", `{"model":"magi-2","prompt":"hello","seed":0,"generate_audio":false}`,
					`{"duration":5,"prompt":"hello","seed":0,"generate_audio":false}`},
				{"image-to-video", `{"model":"magi-2","image_url":"https://img.test/start","end_image_url":"https://img.test/end","duration":4}`,
					`{"duration":4,"image_url":"https://img.test/start","end_image_url":"https://img.test/end"}`},
				{"reference-to-video", `{"model":"magi-2","seconds":"30","metadata":{"audio_urls":["https://media.test/voice"]}}`,
					`{"duration":30,"audio_urls":["https://media.test/voice"]}`},
			} {
				t.Run(tc.mode, func(t *testing.T) {
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(tc.input))
					c.Request.Header.Set("Content-Type", "application/json")
					info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
						ApiKey: provider.key, ChannelBaseUrl: "https://upstream.test",
						ChannelSetting: dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: provider.id}},
					}, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
					a := &TaskAdaptor{}
					a.Init(info)
					require.Equal(t, "SandAI", a.profile.Family)
					require.Nil(t, a.ValidateRequestAndSetAction(c, info))
					target, err := a.BuildRequestURL(info)
					require.NoError(t, err)
					require.Equal(t, "https://upstream.test/sand/magi-2/"+tc.mode, target)
					req := httptest.NewRequest(http.MethodPost, target, nil)
					require.NoError(t, a.BuildRequestHeader(c, req, info))
					require.Equal(t, provider.auth, req.Header.Get("Authorization"))
					body, err := a.BuildRequestBody(c, info)
					require.NoError(t, err)
					data, err := io.ReadAll(body)
					require.NoError(t, err)
					require.JSONEq(t, tc.upstream, string(data))
					require.Empty(t, a.EstimateBilling(c, info), "a time-based pricing expression already includes the duration")
				})
			}
		})
	}
}

func TestMagiFetch(t *testing.T) {
	for _, provider := range []struct{ id, key, auth string }{
		{relaycommon.HiggsfieldMagiProfileID, "id:secret", "Key id:secret"},
		{relaycommon.ModelSellMagiProfileID, "gateway-key", "Bearer gateway-key"},
	} {
		t.Run(provider.id, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "GET", r.Method)
				require.Equal(t, "/requests/task-123/status", r.URL.Path)
				require.Equal(t, provider.auth, r.Header.Get("Authorization"))
				_, _ = io.WriteString(w, `{"request_id":"task-123","status":"completed","video":{"url":"https://media.test/output.mp4"}}`)
			}))
			defer api.Close()
			p, ok := GetProfile(provider.id)
			require.True(t, ok)
			a := &TaskAdaptor{profile: p}
			resp, err := a.FetchTask(api.URL, provider.key, map[string]any{"task_id": "task-123"}, "")
			require.NoError(t, err)
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			result, err := a.ParseTaskResult(b)
			require.NoError(t, err)
			require.Equal(t, string(model.TaskStatusSuccess), result.Status)
			require.Equal(t, "https://media.test/output.mp4", result.Url)
		})
	}
}

func TestModelSellImageFrames(t *testing.T) {
	frames := []string{"https://img.test/start", "https://img.test/end"}
	req := relaycommon.TaskSubmitReq{Images: frames}
	opts := magiOptions{}
	require.NoError(t, opts.normalize(&req, true))
	require.Equal(t, "image-to-video", req.Mode)
	require.Equal(t, frames[0], opts.ImageURL)
	require.Equal(t, frames[1], opts.EndImageURL)
}

func TestMagiNativeTaskResponse(t *testing.T) {
	for _, tc := range []struct {
		status                        model.TaskStatus
		data, reason, video, expected string
	}{
		{model.TaskStatusNotStart, "", "", "", `{"request_id":"task_public","status_url":"/requests/task_public/status","status":"queued"}`},
		{model.TaskStatusInProgress, "", "", "", `{"request_id":"task_public","status_url":"/requests/task_public/status","status":"in_progress"}`},
		{model.TaskStatusSuccess, `{"request_id":"upstream-private"}`, "", "https://media.test/out", `{"request_id":"task_public","status_url":"/requests/task_public/status","status":"completed","video":{"url":"https://media.test/out"}}`},
		{model.TaskStatusFailure, `{"status":"nsfw"}`, "moderation", "", `{"request_id":"task_public","status_url":"/requests/task_public/status","status":"nsfw","error":"moderation"}`},
		{model.TaskStatusFailure, `{"status":"canceled"}`, "", "", `{"request_id":"task_public","status_url":"/requests/task_public/status","status":"canceled"}`},
	} {
		body, err := MagiTaskResponse(&model.Task{TaskID: "task_public", Status: tc.status, Data: []byte(tc.data), FailReason: tc.reason, PrivateData: model.TaskPrivateData{ResultURL: tc.video}})
		require.NoError(t, err)
		require.JSONEq(t, tc.expected, string(body))
	}
}

func TestMagiRejectsInvalidRequest(t *testing.T) {
	for _, body := range []string{
		`{"prompt":"test","duration":31}`,
		`{"prompt":"test","duration":4,"seconds":"5"}`,
		`{"mode":"text-to-video","image_url":"https://img.test/start"}`,
		`{"end_image_url":"https://img.test/end"}`,
		`{"image_url":"https://img.test/start","audio_urls":["https://media.test/voice"]}`,
		`{"audio_urls":["a","b","c","d","e","f"]}`,
		`{"image_urls":["http://img.test/start"]}`,
		`{"prompt":"test","seed":4294967296}`,
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		taskErr := validateMagiVideo(c, &relaycommon.RelayInfo{}, "generate")
		require.NotNil(t, taskErr, body)
		require.Equal(t, http.StatusBadRequest, taskErr.StatusCode, body)
	}
}
