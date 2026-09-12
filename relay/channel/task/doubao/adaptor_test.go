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
	"github.com/tidwall/gjson"
)

func TestTaskAdaptorAcceptsSeedanceNativeContentRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{
		"model":"doubao-seedance-2-0-fast-260128",
		"content":[{"type":"text","text":"一只金色柴犬在樱花树下奔跑"}],
		"resolution":"720p",
		"ratio":"16:9",
		"duration":5,
		"watermark":false
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if _, err := common.GetBodyStorage(c); err != nil {
		t.Fatalf("cache body: %v", err)
	}

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-fast-260128",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeVolcEngine,
			ChannelBaseUrl:    "https://ark.cn-beijing.volces.com",
			ApiKey:            "sk-test",
			UpstreamModelName: "doubao-seedance-2-0-fast-260128",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		t.Fatalf("validate native seedance request: %v", taskErr)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	upstreamBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := gjson.GetBytes(upstreamBody, "content.0.type").String(); got != "text" {
		t.Fatalf("unexpected content type: %s body=%s", got, upstreamBody)
	}
	if got := gjson.GetBytes(upstreamBody, "content.0.text").String(); got != "一只金色柴犬在樱花树下奔跑" {
		t.Fatalf("unexpected text content: %s body=%s", got, upstreamBody)
	}
	if got := gjson.GetBytes(upstreamBody, "duration").Int(); got != 5 {
		t.Fatalf("unexpected duration: %d body=%s", got, upstreamBody)
	}

	ratios := adaptor.EstimateBilling(c, info)
	if got := ratios["seconds"]; got != 5 {
		t.Fatalf("unexpected seconds ratio: %v", got)
	}
}

func TestTaskAdaptorReturnsOfficialSeedanceCreateResponse(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName: "doubao-seedance-2-0-260128",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
	}
	adaptor := &TaskAdaptor{}

	taskID, stored, taskErr := adaptor.DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"cgt-upstream"}`))),
	}, info)
	if taskErr != nil {
		t.Fatalf("do response failed: %v", taskErr)
	}
	if taskID != "cgt-upstream" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
	if gjson.GetBytes(stored, "id").String() != "cgt-upstream" {
		t.Fatalf("unexpected stored response: %s", stored)
	}
	got := recorder.Body.String()
	if got != `{"id":"task_public"}` {
		t.Fatalf("unexpected official create response: %s", got)
	}
	if gjson.GetBytes(recorder.Body.Bytes(), "task_id").Exists() ||
		gjson.GetBytes(recorder.Body.Bytes(), "object").Exists() {
		t.Fatalf("gateway-specific fields leaked into official create response: %s", got)
	}
}

func TestTaskAdaptorReturnsOfficialSeedanceQueryResponse(t *testing.T) {
	adaptor := &TaskAdaptor{}
	body, err := adaptor.ConvertToNativeFetchResponse(&model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		CreatedAt: 1789026244,
		Properties: model.Properties{
			OriginModelName: "doubao-seedance-2-0-260128",
		},
	}, []byte(`{
		"id":"cgt-upstream",
		"model":"doubao-seedance-2-0-260128",
		"status":"succeeded",
		"content":{"video_url":"https://cdn.example/result.mp4"},
		"created_at":1789026244,
		"updated_at":1789026397,
		"duration":5,
		"framespersecond":24,
		"ratio":"16:9",
		"resolution":"720p",
		"seed":73751,
		"usage":{"completion_tokens":108900,"total_tokens":108900}
	}`))
	if err != nil {
		t.Fatalf("convert query response failed: %v", err)
	}
	if gjson.GetBytes(body, "id").String() != "task_public" ||
		gjson.GetBytes(body, "status").String() != "succeeded" {
		t.Fatalf("unexpected official query identity/status: %s", body)
	}
	if gjson.GetBytes(body, "content.video_url").String() != "https://cdn.example/result.mp4" {
		t.Fatalf("unexpected official query content: %s", body)
	}
	if gjson.GetBytes(body, "created_at").Type != gjson.Number ||
		gjson.GetBytes(body, "updated_at").Type != gjson.Number {
		t.Fatalf("official timestamps must be numeric: %s", body)
	}
	for _, field := range []string{"task", "outputs", "metadata", "duration_seconds", "completed_at"} {
		if gjson.GetBytes(body, field).Exists() {
			t.Fatalf("non-official field %q leaked into query response: %s", field, body)
		}
	}
}
