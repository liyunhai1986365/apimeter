package controller

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHVideoGatewayLifecycle(t *testing.T) {
	var phase, creates, queries atomic.Int32
	var submitted atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer yh-test-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			require.Equal(t, "/v1/video/generations", r.URL.Path)
			body, _ := io.ReadAll(r.Body)
			submitted.Store(body)
			count := creates.Add(1)
			if gjson.GetBytes(body, "metadata.content.0.draft_task.id").Exists() && gjson.GetBytes(body, "metadata.duration").Exists() {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":{"code":"InvalidParameter","message":"upstream rejected draft options"}}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"task_id":"cgt-yh-%d","status":"SUBMITTED"}`, count)
			return
		}
		queries.Add(1)
		require.Equal(t, "/v1/video/generations/cgt-yh-1", r.URL.Path)
		switch phase.Load() {
		case 0, 1:
			status := []int{402, 429}[phase.Load()]
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"message":"not available"}}`))
		case 2:
			_, _ = w.Write([]byte(`{"data":{"task_id":"foreign","status":"SUCCESS"}}`))
		case 3:
			_, _ = w.Write([]byte(`{"data":{"task_id":"cgt-yh-1","status":"new-status"}}`))
		case 4:
			_, _ = w.Write([]byte(`{"data":{"task_id":"cgt-yh-1","status":"IN_PROGRESS"}}`))
		default:
			_, _ = w.Write([]byte(`{"data":{"task_id":"cgt-yh-1","status":"SUCCESS","fail_reason":"https://example.com/result.mp4","data":{"content":{"video_url":"https://example.com/result.mp4"},"usage":{"total_tokens":10,"completion_tokens":9,"extra":9007199254740993},"duration":5,"draft":true,"generate_audio":false}}}`))
		}
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL+"/v1")
	payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"text","text":"mock"}],"draft":true,"seed":0,"duration":-1,"return_last_frame":false}`
	w := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "cgt-yh-1", gjson.GetBytes(w.Body.Bytes(), "id").String())
	require.Equal(t, "-1", gjson.GetBytes(submitted.Load().([]byte), "metadata.duration").Raw)
	path := "/api/v3/contents/generations/tasks/cgt-yh-1"
	require.NotEqual(t, 200, seedanceCall(r, "GET", path, "", 2).Code)
	require.Zero(t, queries.Load())
	for p, status := range []int{402, 429, 502, 502, 200} {
		phase.Store(int32(p))
		w = seedanceCall(r, "GET", path, "", 1)
		require.Equal(t, status, w.Code, w.Body.String())
	}
	task, exists, err := model.GetByTaskIDOrUpstreamID(1, "cgt-yh-1")
	require.NoError(t, err)
	require.True(t, exists)
	require.True(t, task.PrivateData.SeedanceDraft)
	phase.Store(5)
	seedancePollingFixture(t, true, 0)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); service.RunTaskPollingOnce(context.Background(), func(int, int) {}) }()
	go func() { defer wg.Done(); w = seedanceCall(r, "GET", path, "", 1) }()
	wg.Wait()
	require.Equal(t, 200, w.Code, w.Body.String())
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	quota := user.Quota
	for i := 0; i < 2; i++ {
		w = seedanceCall(r, "GET", path, "", 1)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "succeeded", gjson.GetBytes(w.Body.Bytes(), "status").String())
		require.Equal(t, "9007199254740993", gjson.GetBytes(w.Body.Bytes(), "usage.extra").Raw)
	}
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.Equal(t, quota, user.Quota, "terminal queries must not settle twice")
	for _, userID := range []int{1, 2} {
		listed := seedanceCall(r, "GET", "/api/v3/contents/generations/tasks?page_size=1&filter.status=succeeded", "", userID)
		require.Equal(t, 200, listed.Code, listed.Body.String())
		expected := int64(1)
		if userID == 2 {
			expected = 0
		}
		require.Equal(t, expected, gjson.GetBytes(listed.Body.Bytes(), "total").Int())
		require.NotContains(t, listed.Body.String(), "yh-test-key")
	}
	upgrade := fmt.Sprintf(`{"model":%q,"content":[{"type":"draft_task","draft_task":{"id":%q}}]}`, tgxRegressionModel, task.TaskID)
	before := creates.Load()
	require.NotEqual(t, 200, seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", upgrade, 2).Code)
	require.Equal(t, before, creates.Load())
	// Even if distribution initially selects another Seedance profile, draft
	// ownership must pin the original supplier before constructing a request.
	alternateURL := "http://127.0.0.1:1"
	alternative := &model.Channel{Id: 21, Type: constant.ChannelTypeConfigurable, Status: common.ChannelStatusEnabled, Key: "different-key", BaseURL: &alternateURL, Models: tgxRegressionModel, Group: "default"}
	alternative.SetSetting(relaydto.ChannelSettings{Protocol: &relaydto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
	require.NoError(t, model.DB.Create(alternative).Error)
	createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
	w = seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", upgrade, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	wire := submitted.Load().([]byte)
	require.Equal(t, "cgt-yh-1", gjson.GetBytes(wire, "metadata.content.0.draft_task.id").String())
	for _, field := range []string{"prompt", "metadata.duration", "metadata.ratio", "metadata.generate_audio"} {
		require.False(t, gjson.GetBytes(wire, field).Exists(), field)
	}
	// Preserve explicit options and let the provider enforce draft upgrade rules.
	upgrade = fmt.Sprintf(`{"model":%q,"content":[{"type":"draft_task","draft_task":{"id":%q}}],"duration":5,"ratio":"16:9","generate_audio":false}`, tgxRegressionModel, task.TaskID)
	before = creates.Load()
	w = seedanceCall(r, http.MethodPost, "/api/v3/contents/generations/tasks", upgrade, 1)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "upstream rejected draft options")
	require.Equal(t, before+1, creates.Load())
	wire = submitted.Load().([]byte)
	require.Equal(t, "cgt-yh-1", gjson.GetBytes(wire, "metadata.content.0.draft_task.id").String())
	require.Equal(t, "5", gjson.GetBytes(wire, "metadata.duration").Raw)
	require.Equal(t, "16:9", gjson.GetBytes(wire, "metadata.ratio").String())
	require.Equal(t, "false", gjson.GetBytes(wire, "metadata.generate_audio").Raw)
}
