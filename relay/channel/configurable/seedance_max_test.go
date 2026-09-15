package configurable

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func seedanceMaxRelayInfo(profileID string) *relaycommon.RelayInfo {
	info := seedanceRelayInfo("doubao-seedance-2-0-260128-max")
	if profileID == "seedance2-service-inference" {
		info = seedanceServiceInferenceRelayInfo("dreamina-seedance-2-0-260128-max")
	}
	info.ChannelMeta.ChannelBaseUrl = "https://model.service-inference.ai"
	info.ChannelMeta.ChannelSetting.Protocol.ProfileID = profileID
	info.TaskRelayInfo.PublicTaskID = "task_public"
	return info
}

func TestSeedanceMaxPreservesNativeContentAndOptions(t *testing.T) {
	for _, profileID := range []string{"doubao-seedance-max-service-inference", "seedance2-service-inference"} {
		t.Run(profileID, func(t *testing.T) {
			info := seedanceMaxRelayInfo(profileID)
			body := []byte(`{
		"model":"public-seedance-alias",
		"content":[
			{"type":"text","text":""},
			{"type":"text","text":"草莓参考 @Image1，开场 @Video1，运镜 @Video2，冲击力 @Video3"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/first.png"},"role":"first_frame"},
			{"type":"image_url","image_url":{"url":"asset://mva-e144dfc364a647f1"},"role":"reference_image"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/open.mp4"},"role":"reference_video"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/camera.mp4"},"role":"reference_video"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/impact.mp4"},"role":"reference_video"},
			{"type":"audio_url","audio_url":{"url":"https://cdn.example/audio.mp3"},"role":"reference_audio"},
			{"type":"image_url","image_url":{"url":"https://cdn.example/last.png"},"role":"last_frame"}
		],
		"duration":15,"resolution":"480p","aspect_ratio":"16:9","generate_audio":false,"seed":0
	}`)
			gin.SetMode(gin.TestMode)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			a := &TaskAdaptor{}
			a.Init(info)
			modelName, err := ExtractNativeModel(c, a.profile)
			require.NoError(t, err)
			require.Equal(t, "public-seedance-alias", modelName)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			parsed, err := relaycommon.GetTaskRequest(c)
			require.NoError(t, err)
			require.Equal(t, gjson.GetBytes(body, "content.1.text").String(), parsed.Prompt)
			require.Equal(t, 15, parsed.Duration)
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			got, err := io.ReadAll(reader)
			require.NoError(t, err)
			expected, err := sjson.SetBytes(body, "model", info.UpstreamModelName)
			require.NoError(t, err)
			require.JSONEq(t, string(expected), string(got))
			url, err := a.BuildRequestURL(info)
			require.NoError(t, err)
			require.Equal(t, "https://model.service-inference.ai/v2/video/generate", url)
			request := httptest.NewRequest(http.MethodPost, url, nil)
			require.NoError(t, a.BuildRequestHeader(c, request, info))
			require.Equal(t, "Bearer sk-test", request.Header.Get("Authorization"))
		})
	}
}

func TestSeedanceMaxGenericContentAndOptionalAudio(t *testing.T) {
	for _, profileID := range []string{"doubao-seedance-max-service-inference", "seedance2-service-inference"} {
		t.Run(profileID, func(t *testing.T) {
			info := seedanceMaxRelayInfo(profileID)
			profile, ok := GetProfile(info.ChannelSetting.Protocol.ProfileID)
			require.True(t, ok)
			var req relaycommon.TaskSubmitReq
			require.NoError(t, common.Unmarshal([]byte(`{
		"prompt":"fallback", "images":["https://cdn.example/fallback.png"],
		"metadata":{"content":[
			{"type":"text","text":"@Video1 @Video2"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/1.mp4"},"role":"reference_video"},
			{"type":"video_url","video_url":{"url":"https://cdn.example/2.mp4"},"role":"reference_video"}
		],"generate_audio":false,"aspect_ratio":"16:9"}
	}`), &req))
			body, err := buildMappedBody(profile.Video.Submit.Body.Fields, req, info)
			require.NoError(t, err)
			require.Equal(t, req.Metadata["content"], body["content"])
			require.Equal(t, false, body["generate_audio"])
			require.Equal(t, "16:9", body["ratio"])
			delete(req.Metadata, "content")
			delete(req.Metadata, "generate_audio")
			req.Metadata["video_url"] = []any{"https://cdn.example/1.mp4", "https://cdn.example/2.mp4", "https://cdn.example/3.mp4"}
			body, err = buildMappedBody(profile.Video.Submit.Body.Fields, req, info)
			require.NoError(t, err)
			require.NotContains(t, body, "generate_audio")
			encoded, err := common.Marshal(body)
			require.NoError(t, err)
			require.Len(t, gjson.GetBytes(encoded, `content.#(type=="video_url")#`).Array(), 3)
		})
	}
}

func TestSeedanceMaxTaskLifecycleAndPublicResponses(t *testing.T) {
	for _, profileID := range []string{"doubao-seedance-max-service-inference", "seedance2-service-inference"} {
		t.Run(profileID, func(t *testing.T) {
			info := seedanceMaxRelayInfo(profileID)
			a := &TaskAdaptor{}
			a.Init(info)
			for status, expected := range map[string]model.TaskStatus{
				"preparing": model.TaskStatusQueued, "pending": model.TaskStatusQueued,
				"processing": model.TaskStatusInProgress, "completed": model.TaskStatusSuccess, "failed": model.TaskStatusFailure,
			} {
				t.Run(status, func(t *testing.T) {
					raw := []byte(`{"task":{"id":"mvt-upstream","status":"` + status + `"}}`)
					result, err := a.ParseTaskResult(raw)
					require.NoError(t, err)
					require.Equal(t, string(expected), result.Status)
				})
			}
			preparing := []byte(`{"task":{"id":"mvt-upstream","status":"preparing","model":"` + info.UpstreamModelName + `","duration_seconds":8,"outputs":[],"error":null,"created_at":"2026-09-06T12:50:47.836Z","completed_at":null,"prep":{"total":1,"active":0,"failed":0,"attempt":1}}}`)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
			id, stored, taskErr := a.DoResponse(c, &http.Response{Body: io.NopCloser(bytes.NewReader(preparing))}, info)
			require.Nil(t, taskErr)
			require.Equal(t, "mvt-upstream", id)
			require.JSONEq(t, string(preparing), string(stored))
			require.False(t, gjson.Get(recorder.Body.String(), "task").Exists())
			pending, ok := c.Get(NativeTaskSubmitResponseKey)
			require.True(t, ok)
			require.Empty(t, recorder.Body.String())
			require.JSONEq(t, `{"id":"mvt-upstream"}`, string(pending.([]byte)))
			preparingFetch, err := a.ConvertToNativeFetchResponse(&model.Task{TaskID: "task_public"}, preparing)
			require.NoError(t, err)
			require.Equal(t, "task_public", gjson.GetBytes(preparingFetch, "id").String())
			require.Equal(t, "queued", gjson.GetBytes(preparingFetch, "status").String())
			require.Equal(t, int64(8), gjson.GetBytes(preparingFetch, "duration").Int())
			require.False(t, gjson.GetBytes(preparingFetch, "prep").Exists())
			completed := []byte(`{"task":{"id":"mvt-upstream","status":"completed","outputs":["https://cdn.example/result.mp4"],"usage":{"completion_tokens":40594,"total_tokens":40594},"last_frame_url":"https://cdn.example/last.png"}}`)
			result, err := a.ParseTaskResult(completed)
			require.NoError(t, err)
			require.Equal(t, 40594, result.TotalTokens)
			require.Equal(t, 40594, result.CompletionTokens)
			require.Equal(t, "https://cdn.example/result.mp4", result.Url)
			public, err := a.ConvertToNativeFetchResponse(&model.Task{TaskID: "task_public"}, completed)
			require.NoError(t, err)
			require.False(t, gjson.GetBytes(public, "task").Exists())
			require.Equal(t, "task_public", gjson.GetBytes(public, "id").String())
			require.Equal(t, "succeeded", gjson.GetBytes(public, "status").String())
			require.Equal(t, "https://cdn.example/result.mp4", gjson.GetBytes(public, "content.video_url").String())
			require.Equal(t, int64(40594), gjson.GetBytes(public, "usage.total_tokens").Int())
			failed, err := a.ParseTaskResult([]byte(`{"task":{"id":"mvt-upstream","status":"failed","error":"Reference material @Image2 could not be prepared: url is not reachable"}}`))
			require.NoError(t, err)
			require.Equal(t, "Reference material @Image2 could not be prepared: url is not reachable", failed.Reason)
			failedFetch, err := a.ConvertToNativeFetchResponse(&model.Task{TaskID: "task_public"}, []byte(`{"task":{"id":"mvt-upstream","status":"failed","outputs":[],"error":"Reference unavailable","prep":{"total":1,"failed":1,"failedRefs":["@Image2"]}}}`))
			require.NoError(t, err)
			require.Equal(t, "task_public", gjson.GetBytes(failedFetch, "id").String())
			require.Equal(t, "failed", gjson.GetBytes(failedFetch, "status").String())
			require.Equal(t, "Reference unavailable", gjson.GetBytes(failedFetch, "error.message").String())
			require.False(t, gjson.GetBytes(failedFetch, "prep").Exists())
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v2/video/tasks/mvt-upstream", r.URL.Path)
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer sk-test", r.Header.Get("Authorization"))
				_, _ = w.Write(completed)
			}))
			defer server.Close()
			response, err := a.FetchTask(server.URL, "sk-test", map[string]any{"task_id": "mvt-upstream"}, "")
			require.NoError(t, err)
			defer response.Body.Close()
		})
	}
}

func TestSeedanceMaxLegacyNativeFetchHidesGatewayFields(t *testing.T) {
	for _, profileID := range []string{"doubao-seedance-max-service-inference", "seedance2-service-inference"} {
		t.Run(profileID, func(t *testing.T) {
			info := seedanceMaxRelayInfo(profileID)
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			task := &model.Task{
				TaskID: "task_public", Status: model.TaskStatusSuccess, CreatedAt: 1788699047,
				Properties: model.Properties{OriginModelName: info.UpstreamModelName},
			}
			body, err := adaptor.ConvertToNativeFetchResponse(task, []byte(`{"code":"success","data":{"task_id":"mvt-upstream","status":"SUCCESS","result_url":"https://cdn.example/stored.mp4","user_id":42,"channel_id":7,"quota":100,"private_data":{"key":"must-not-be-returned"}}}`))
			require.NoError(t, err)
			require.Equal(t, "task_public", gjson.GetBytes(body, "id").String())
			require.Equal(t, "succeeded", gjson.GetBytes(body, "status").String())
			require.Equal(t, "https://cdn.example/stored.mp4", gjson.GetBytes(body, "content.video_url").String())
			for _, field := range []string{"task", "code", "data", "user_id", "channel_id", "quota", "private_data"} {
				require.False(t, gjson.GetBytes(body, field).Exists(), field)
			}
			require.NotContains(t, string(body), "must-not-be-returned")
		})
	}
}
