package configurable

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Contracts checked against the official Ark docs on 2026-09-11:
// https://docs.volcengine.com/docs/82379/1520757 (create)
// https://docs.volcengine.com/docs/82379/1521309 (fetch)
func TestSeedanceNativeResponseContractAcrossProfiles(t *testing.T) {
	const officialTask = `{
		"id":"cgt-upstream", "model":"doubao-seedance-2-5-260628", "status":"succeeded",
		"content":{"video_url":"https://cdn.example/video.mov","last_frame_url":"https://cdn.example/last.png"},
		"created_at":1789026244, "updated_at":1789026397, "error":null,
		"duration":5, "framespersecond":24, "resolution":"720p", "ratio":"16:9",
		"seed":0, "generate_audio":false, "draft":false, "draft_task_id":"cgt-draft",
		"service_tier":"default", "execution_expires_after":172800,
		"safety_identifier":"hashed-user", "output_format":"mov",
		"tools":[{"type":"web_search"}],
		"usage":{"completion_tokens":108900,"total_tokens":108900,"tool_usage":{"web_search":0}}
	}`
	for _, profileID := range seedanceOptionProfiles {
		t.Run(profileID, func(t *testing.T) {
			info := seedanceMaxRelayInfo(profileID)
			a := &TaskAdaptor{}
			a.Init(info)
			require.Equal(t, "volcengine_video_task_create", a.profile.videoNative().Submit.ResponseFormat)
			require.Equal(t, "volcengine_video_task", a.profile.videoNative().Fetch.ResponseFormat)
			upstream := `{"id":"cgt-upstream"}`
			if profileID == "seedance2-service-inference" || profileID == "doubao-seedance-max-service-inference" {
				upstream = `{"task":{"id":"cgt-upstream","status":"preparing","outputs":[]}}`
			}
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
			id, stored, taskErr := a.DoResponse(c, &http.Response{
				StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(upstream)),
			}, info)
			require.Nil(t, taskErr)
			require.Equal(t, "cgt-upstream", id)
			require.JSONEq(t, upstream, string(stored))
			require.Equal(t, http.StatusOK, recorder.Code)
			pending, ok := c.Get(NativeTaskSubmitResponseKey)
			require.True(t, ok)
			require.Empty(t, recorder.Body.String())
			require.JSONEq(t, `{"id":"cgt-upstream"}`, string(pending.([]byte)))

			task := &model.Task{
				TaskID: "task_public", PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-upstream"}, Status: model.TaskStatusSuccess,
				Properties: model.Properties{OriginModelName: "doubao-seedance-2-5-260628"},
			}
			body, err := a.ConvertToNativeFetchResponse(task, []byte(officialTask))
			require.NoError(t, err)
			// Official responses retain the upstream ID and every field.
			expected := []byte(officialTask)
			require.JSONEq(t, string(expected), string(body))
		})
	}
}

func TestSeedanceNativeTerminalErrors(t *testing.T) {
	for _, status := range []string{"failed", "expired", "cancelled"} {
		t.Run(status, func(t *testing.T) {
			body, err := BuildVolcengineVideoTaskResponse(&model.Task{
				TaskID: "task_public", Status: model.TaskStatusFailure,
			}, []byte(`{"status":"`+status+`","error":{"code":"TaskExpired","message":"Task exceeded its deadline"}}`))
			require.NoError(t, err)
			require.Equal(t, status, gjson.GetBytes(body, "status").String())
			require.Equal(t, "TaskExpired", gjson.GetBytes(body, "error.code").String())
			require.Equal(t, "Task exceeded its deadline", gjson.GetBytes(body, "error.message").String())
			require.False(t, gjson.GetBytes(body, "content").Exists())
		})
	}
}

func TestSeedanceNativeFlatServiceInferenceSnapshot(t *testing.T) {
	body, err := BuildVolcengineVideoTaskResponse(&model.Task{
		TaskID: "task_public", Status: model.TaskStatusSuccess,
	}, []byte(`{
		"id":"mvt-upstream", "model":"public-model", "status":"completed",
		"outputs":["https://cdn.example/result.mp4"],"last_frame_url":"https://cdn.example/last.png",
		"duration_seconds":5,"created_at":"2026-09-06T12:50:47Z","completed_at":"2026-09-06T12:51:47Z",
		"usage":{"completion_tokens":100,"total_tokens":100},"error":null
	}`))
	require.NoError(t, err)
	require.Equal(t, "succeeded", gjson.GetBytes(body, "status").String())
	require.Equal(t, "https://cdn.example/last.png", gjson.GetBytes(body, "content.last_frame_url").String())
	require.Equal(t, "https://cdn.example/result.mp4", gjson.GetBytes(body, "content.video_url").String())
	require.Equal(t, int64(5), gjson.GetBytes(body, "duration").Int())
	require.Equal(t, int64(60), gjson.GetBytes(body, "updated_at").Int()-gjson.GetBytes(body, "created_at").Int())
	require.Equal(t, "null", gjson.GetBytes(body, "error").Raw)
	for _, field := range []string{"outputs", "last_frame_url", "duration_seconds", "completed_at"} {
		require.False(t, gjson.GetBytes(body, field).Exists(), field)
	}
}
