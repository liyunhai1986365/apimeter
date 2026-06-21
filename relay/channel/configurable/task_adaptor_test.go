package configurable

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestTaskAdaptorBuildsMappedSubmitRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"video-model","prompt":"a cat","size":"720x1280","seconds":"4","metadata":{"seed":7}}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if _, err := common.GetBodyStorage(c); err != nil {
		t.Fatalf("cache body: %v", err)
	}

	info := &relaycommon.RelayInfo{
		OriginModelName: "video-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeConfigurable,
			ChannelBaseUrl:    "https://example.com",
			ApiKey:            "sk-test",
			UpstreamModelName: "upstream-video-model",
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "generic-video-json",
				},
			},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if err := adaptor.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatalf("validate request: %v", err)
	}

	url, err := adaptor.BuildRequestURL(info)
	if err != nil {
		t.Fatalf("build url: %v", err)
	}
	if url != "https://example.com/v1/videos" {
		t.Fatalf("unexpected url: %s", url)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	mappedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read mapped body: %v", err)
	}
	if got := gjson.GetBytes(mappedBody, "model").String(); got != "upstream-video-model" {
		t.Fatalf("unexpected model: %s, body=%s", got, mappedBody)
	}
	if got := gjson.GetBytes(mappedBody, "prompt").String(); got != "a cat" {
		t.Fatalf("unexpected prompt: %s, body=%s", got, mappedBody)
	}
	if got := gjson.GetBytes(mappedBody, "metadata.seed").Int(); got != 7 {
		t.Fatalf("unexpected seed: %d, body=%s", got, mappedBody)
	}
}

func TestBuildConfiguredResponseUsesMultipleFallbackSources(t *testing.T) {
	responseBody, err := BuildConfiguredResponse(ResponseConfig{
		Fields: []FieldMapping{
			{To: "code", Value: 0},
			{To: "message", Value: "ok"},
			{
				To:            "data.Id",
				From:          "data.Id",
				FallbackFrom:  "id",
				FallbackFroms: []string{"asset_id"},
			},
		},
	}, []byte(`{"asset_id":"asset-from-material"}`), nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"code":0,"message":"ok","data":{"Id":"asset-from-material"}}`, string(responseBody))
}

func TestBuildConfiguredResponsePrefersPrimarySourceBeforeFallbacks(t *testing.T) {
	responseBody, err := BuildConfiguredResponse(ResponseConfig{
		Fields: []FieldMapping{
			{
				To:            "data.Id",
				From:          "data.Id",
				FallbackFrom:  "id",
				FallbackFroms: []string{"asset_id"},
			},
		},
	}, []byte(`{"data":{"Id":"asset-from-api"},"id":"asset-from-id","asset_id":"asset-from-material"}`), nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"data":{"Id":"asset-from-api"}}`, string(responseBody))
}

func TestTaskAdaptorBuildsHappyHorseTextToVideoRequest(t *testing.T) {
	body := buildHappyHorseRequestBody(t, []byte(`{
		"model":"happyhorse-1.0-t2v",
		"prompt":"一座微型城市在夜晚亮起灯光",
		"size":"720P",
		"seconds":"5",
		"metadata":{"ratio":"16:9","watermark":false,"seed":7}
	}`), "happyhorse-1.0-t2v")

	if got := gjson.GetBytes(body, "model").String(); got != "happyhorse-1.0-t2v" {
		t.Fatalf("unexpected model: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.prompt").String(); got != "一座微型城市在夜晚亮起灯光" {
		t.Fatalf("unexpected prompt: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.resolution").String(); got != "720P" {
		t.Fatalf("unexpected resolution: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.duration").Int(); got != 5 {
		t.Fatalf("unexpected duration: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.ratio").String(); got != "16:9" {
		t.Fatalf("unexpected ratio: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.watermark").Bool(); got {
		t.Fatalf("unexpected watermark: %v body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.seed").Int(); got != 7 {
		t.Fatalf("unexpected seed: %d body=%s", got, body)
	}
	if gjson.GetBytes(body, "input.media").Exists() {
		t.Fatalf("text-to-video should omit media, body=%s", body)
	}
}

func TestTaskAdaptorBuildsHappyHorseImageToVideoRequest(t *testing.T) {
	body := buildHappyHorseRequestBody(t, []byte(`{
		"model":"happyhorse-1.0-i2v",
		"prompt":"一只猫在草地上奔跑",
		"image":"https://example.com/first.png",
		"size":"720P",
		"duration":5
	}`), "happyhorse-1.0-i2v")

	if got := gjson.GetBytes(body, "input.media.#").Int(); got != 1 {
		t.Fatalf("unexpected media length: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.0.type").String(); got != "first_frame" {
		t.Fatalf("unexpected media type: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.0.url").String(); got != "https://example.com/first.png" {
		t.Fatalf("unexpected media url: %s body=%s", got, body)
	}
}

func TestTaskAdaptorBuildsHappyHorseReferenceToVideoRequest(t *testing.T) {
	body := buildHappyHorseRequestBody(t, []byte(`{
		"model":"happyhorse-1.0-r2v",
		"prompt":"[Image 1]中的人物拿起[Image 2]中的折扇",
		"images":["https://example.com/a.jpg","https://example.com/b.jpg"],
		"metadata":{"ratio":"4:3"}
	}`), "happyhorse-1.0-r2v")

	if got := gjson.GetBytes(body, "input.media.#").Int(); got != 2 {
		t.Fatalf("unexpected media length: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.0.type").String(); got != "reference_image" {
		t.Fatalf("unexpected first media type: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.1.url").String(); got != "https://example.com/b.jpg" {
		t.Fatalf("unexpected second media url: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.ratio").String(); got != "4:3" {
		t.Fatalf("unexpected ratio: %s body=%s", got, body)
	}
}

func TestTaskAdaptorBuildsHappyHorseVideoEditRequest(t *testing.T) {
	body := buildHappyHorseRequestBody(t, []byte(`{
		"model":"happyhorse-1.0-video-edit",
		"prompt":"让视频中的角色穿上参考图中的毛衣",
		"images":["https://example.com/ref.webp"],
		"metadata":{"video_url":"https://example.com/input.mp4","audio_setting":"origin"}
	}`), "happyhorse-1.0-video-edit")

	if got := gjson.GetBytes(body, "input.media.#").Int(); got != 2 {
		t.Fatalf("unexpected media length: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.0.type").String(); got != "video" {
		t.Fatalf("unexpected first media type: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.0.url").String(); got != "https://example.com/input.mp4" {
		t.Fatalf("unexpected video url: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "input.media.1.type").String(); got != "reference_image" {
		t.Fatalf("unexpected reference media type: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "parameters.audio_setting").String(); got != "origin" {
		t.Fatalf("unexpected audio_setting: %s body=%s", got, body)
	}
}

func TestTaskAdaptorBuildsSeedanceTextToVideoRequest(t *testing.T) {
	body := buildSeedanceRequestBody(t, []byte(`{
		"model":"doubao-seedance-2-0-260128",
		"prompt":"一只金色柴犬在樱花树下奔跑，镜头缓缓上升",
		"size":"480p",
		"seconds":"5",
		"metadata":{"ratio":"16:9","watermark":false,"generate_audio":true}
	}`), "doubao-seedance-2-0-260128")

	if got := gjson.GetBytes(body, "model").String(); got != "doubao-seedance-2-0-260128" {
		t.Fatalf("unexpected model: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.#").Int(); got != 1 {
		t.Fatalf("unexpected content length: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.0.type").String(); got != "text" {
		t.Fatalf("unexpected content type: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.0.text").String(); got != "一只金色柴犬在樱花树下奔跑，镜头缓缓上升" {
		t.Fatalf("unexpected text: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "resolution").String(); got != "480p" {
		t.Fatalf("unexpected resolution: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "duration").Int(); got != 5 {
		t.Fatalf("unexpected duration: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "ratio").String(); got != "16:9" {
		t.Fatalf("unexpected ratio: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "watermark").Bool(); got {
		t.Fatalf("unexpected watermark: %v body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "generate_audio").Bool(); !got {
		t.Fatalf("unexpected generate_audio: %v body=%s", got, body)
	}
}

func TestTaskAdaptorBuildsSeedanceAllModalitiesRequest(t *testing.T) {
	body := buildSeedanceRequestBody(t, []byte(`{
		"model":"doubao-seedance-2-0-fast-260128",
		"prompt":"使用图片的风格，把这个声音应用在女主说话的场景，并修改这个视频",
		"images":["https://example.com/reference_image.jpg"],
		"metadata":{
			"video_url":"https://example.com/reference-video.mp4",
			"audio_url":"https://example.com/reference_audio.mp3",
			"ratio":"9:16",
			"watermark":false
		},
		"duration":6
	}`), "doubao-seedance-2-0-fast-260128")

	if got := gjson.GetBytes(body, "content.#").Int(); got != 4 {
		t.Fatalf("unexpected content length: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.1.type").String(); got != "image_url" {
		t.Fatalf("unexpected image content type: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.1.image_url.url").String(); got != "https://example.com/reference_image.jpg" {
		t.Fatalf("unexpected image url: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.1.role").String(); got != "reference_image" {
		t.Fatalf("unexpected image role: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.2.video_url.url").String(); got != "https://example.com/reference-video.mp4" {
		t.Fatalf("unexpected video url: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.2.role").String(); got != "reference_video" {
		t.Fatalf("unexpected video role: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "content.3.audio_url.url").String(); got != "https://example.com/reference_audio.mp3" {
		t.Fatalf("unexpected audio url: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "duration").Int(); got != 6 {
		t.Fatalf("unexpected duration: %d body=%s", got, body)
	}
}

func TestTaskAdaptorBuildsSeedanceNativeOfficialRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"doubao-seedance-2-0-260128",
		"content":[
			{"type":"text","text":"让画面中的人物缓缓转身微笑"},
			{"type":"image_url","image_url":{"url":"https://example.com/photo.jpg"},"role":"reference_image"}
		],
		"resolution":"480p",
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

	info := seedanceRelayInfo("doubao-seedance-2-0-260128")
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if err := adaptor.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatalf("validate native request: %v", err)
	}
	ratios := adaptor.EstimateBilling(c, info)
	if got := ratios["seconds"]; got != 5 {
		t.Fatalf("unexpected billing seconds ratio: %v", got)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	mappedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read mapped body: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(mappedBody), bytes.TrimSpace(body)) {
		t.Fatalf("native official body should passthrough, got=%s want=%s", mappedBody, body)
	}
}

func TestTaskAdaptorNativePassthroughBodySurvivesUpstreamRequestClose(t *testing.T) {
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

	info := seedanceRelayInfo("doubao-seedance-2-0-fast-260128")
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if err := adaptor.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatalf("validate native request: %v", err)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "https://example.com/api/v3/contents/generations/tasks", common.ReaderOnly(reader))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if err := req.Body.Close(); err != nil {
		t.Fatalf("close upstream request body: %v", err)
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		t.Fatalf("body storage should remain reusable: %v", err)
	}
	got, err := storage.Bytes()
	if err != nil {
		t.Fatalf("read reusable body: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(body)) {
		t.Fatalf("unexpected cached body: %s", got)
	}
}

func TestTaskAdaptorParsesSeedanceResponses(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := seedanceRelayInfo("doubao-seedance-2-0-260128")
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{
		PublicTaskID: "task_public",
	}
	adaptor.Init(info)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"task_seedance","status":"queued","model":"doubao-seedance-2-0-260128","created_at":1779607183}`))),
	}
	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("do response error: %v", taskErr)
	}
	if taskID != "task_seedance" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
	if !bytes.Contains(taskData, []byte("task_seedance")) {
		t.Fatalf("expected original task data, got %s", taskData)
	}
	if got := gjson.GetBytes(recorder.Body.Bytes(), "status").String(); got != dto.VideoStatusQueued {
		t.Fatalf("unexpected public response status: %s body=%s", got, recorder.Body.String())
	}

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"task_seedance","status":"succeeded","model":"doubao-seedance-2-0-260128","content":{"video_url":"https://example.com/result.mp4"},"usage":{"completion_tokens":100858}}`))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Status != "SUCCESS" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Url != "https://example.com/result.mp4" {
		t.Fatalf("unexpected url: %s", result.Url)
	}
	if result.TotalTokens != 100858 {
		t.Fatalf("unexpected total tokens: %d", result.TotalTokens)
	}
}

func TestTaskAdaptorParsesUsageTotalTokensBeforeCompletionTokens(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "video-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeConfigurable,
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "generic-video-json",
				},
			},
		},
	}
	adaptor.Init(info)

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"task_generic","status":"completed","video":{"url":"https://example.com/result.mp4"},"usage":{"completion_tokens":100,"total_tokens":120}}`))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.CompletionTokens != 100 {
		t.Fatalf("unexpected completion tokens: %d", result.CompletionTokens)
	}
	if result.TotalTokens != 120 {
		t.Fatalf("unexpected total tokens: %d", result.TotalTokens)
	}
}

func TestTaskAdaptorBuildsHappyHorseNativeDashScopeRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"model":"happyhorse-1.0-i2v",
		"input":{
			"prompt":"一只猫在草地上奔跑",
			"media":[{"type":"first_frame","url":"https://example.com/first.png"}]
		},
		"parameters":{"resolution":"720P","duration":5,"watermark":false,"seed":7}
	}`)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if _, err := common.GetBodyStorage(c); err != nil {
		t.Fatalf("cache body: %v", err)
	}

	info := happyHorseRelayInfo("happyhorse-1.0-i2v")
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if err := adaptor.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatalf("validate native request: %v", err)
	}
	ratios := adaptor.EstimateBilling(c, info)
	if got := ratios["seconds"]; got != 5 {
		t.Fatalf("unexpected billing seconds ratio: %v", got)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	mappedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read mapped body: %v", err)
	}
	if got := gjson.GetBytes(mappedBody, "input.prompt").String(); got != "一只猫在草地上奔跑" {
		t.Fatalf("unexpected prompt: %s body=%s", got, mappedBody)
	}
	if got := gjson.GetBytes(mappedBody, "input.media.0.type").String(); got != "first_frame" {
		t.Fatalf("unexpected media type: %s body=%s", got, mappedBody)
	}
	if got := gjson.GetBytes(mappedBody, "input.media.0.url").String(); got != "https://example.com/first.png" {
		t.Fatalf("unexpected media url: %s body=%s", got, mappedBody)
	}
	if got := gjson.GetBytes(mappedBody, "parameters.duration").Int(); got != 5 {
		t.Fatalf("unexpected duration: %d body=%s", got, mappedBody)
	}
	if got := gjson.GetBytes(mappedBody, "parameters.watermark").Bool(); got {
		t.Fatalf("unexpected watermark: %v body=%s", got, mappedBody)
	}
}

func TestTaskAdaptorParsesSubmitAndFetchResponses(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{
		OriginModelName: "video-model",
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			PublicTaskID: "task_public",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    constant.ChannelTypeConfigurable,
			ChannelBaseUrl: "https://example.com",
			ApiKey:         "sk-test",
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "generic-video-json",
				},
			},
		},
	}
	adaptor.Init(info)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"id":"upstream-task","status":"queued","created_at":1710000000}`))),
	}
	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("do response error: %v", taskErr)
	}
	if taskID != "upstream-task" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
	if !bytes.Contains(taskData, []byte("upstream-task")) {
		t.Fatalf("expected original task data, got %s", taskData)
	}
	if got := gjson.GetBytes(recorder.Body.Bytes(), "id").String(); got != "task_public" {
		t.Fatalf("unexpected public response id: %s body=%s", got, recorder.Body.String())
	}

	result, err := adaptor.ParseTaskResult([]byte(`{"id":"upstream-task","status":"completed","progress":100,"video":{"url":"https://cdn.example/video.mp4"},"completed_at":1710000099}`))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Status != "SUCCESS" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Url != "https://cdn.example/video.mp4" {
		t.Fatalf("unexpected url: %s", result.Url)
	}
	if result.Progress != "100%" {
		t.Fatalf("unexpected progress: %s", result.Progress)
	}
}

func TestTaskAdaptorParsesHappyHorseResponses(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := happyHorseRelayInfo("happyhorse-1.0-t2v")
	info.TaskRelayInfo = &relaycommon.TaskRelayInfo{
		PublicTaskID: "task_public",
	}
	adaptor.Init(info)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"output":{"task_status":"PENDING","task_id":"dashscope-task"},"request_id":"req-1"}`))),
	}
	taskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		t.Fatalf("do response error: %v", taskErr)
	}
	if taskID != "dashscope-task" {
		t.Fatalf("unexpected upstream task id: %s", taskID)
	}
	if !bytes.Contains(taskData, []byte("dashscope-task")) {
		t.Fatalf("expected original task data, got %s", taskData)
	}
	if got := gjson.GetBytes(recorder.Body.Bytes(), "status").String(); got != dto.VideoStatusQueued {
		t.Fatalf("unexpected public response status: %s body=%s", got, recorder.Body.String())
	}

	result, err := adaptor.ParseTaskResult([]byte(`{"output":{"task_id":"dashscope-task","task_status":"SUCCEEDED","video_url":"https://dashscope-result/video.mp4"},"usage":{"duration":5}}`))
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}
	if result.Status != "SUCCESS" {
		t.Fatalf("unexpected status: %s", result.Status)
	}
	if result.Url != "https://dashscope-result/video.mp4" {
		t.Fatalf("unexpected url: %s", result.Url)
	}
	if result.Progress != "100%" {
		t.Fatalf("unexpected progress: %s", result.Progress)
	}

	failed, err := adaptor.ParseTaskResult([]byte(`{"output":{"task_id":"dashscope-task","task_status":"FAILED","code":"InvalidParameter","message":"The parameter is invalid."}}`))
	if err != nil {
		t.Fatalf("parse failed result: %v", err)
	}
	if failed.Status != "FAILURE" {
		t.Fatalf("unexpected failed status: %s", failed.Status)
	}
	if failed.Reason != "The parameter is invalid." {
		t.Fatalf("unexpected failed reason: %s", failed.Reason)
	}
}

func TestTaskAdaptorKeepsHappyHorseNativeFetchResponseNative(t *testing.T) {
	adaptor := &TaskAdaptor{}
	info := happyHorseRelayInfo("happyhorse-1.0-t2v")
	adaptor.Init(info)

	nativeFetch, err := adaptor.ConvertToNativeFetchResponse(&model.Task{
		TaskID: "task_public",
	}, []byte(`{
		"request_id":"req-2",
		"output":{
			"task_id":"dashscope-task",
			"task_status":"SUCCEEDED",
			"video_url":"https://dashscope-result/video.mp4"
		},
		"usage":{"duration":5,"ratio":"16:9"}
	}`))
	if err != nil {
		t.Fatalf("convert native fetch response: %v", err)
	}
	if got := gjson.GetBytes(nativeFetch, "output.task_id").String(); got != "task_public" {
		t.Fatalf("unexpected native task id: %s body=%s", got, nativeFetch)
	}
	if got := gjson.GetBytes(nativeFetch, "output.task_status").String(); got != "SUCCEEDED" {
		t.Fatalf("unexpected native status: %s body=%s", got, nativeFetch)
	}
	if got := gjson.GetBytes(nativeFetch, "output.video_url").String(); got != "https://dashscope-result/video.mp4" {
		t.Fatalf("unexpected native video url: %s body=%s", got, nativeFetch)
	}
	if gjson.GetBytes(nativeFetch, "status").Exists() {
		t.Fatalf("native fetch response should not be converted to generic video format, body=%s", nativeFetch)
	}
}

func TestTaskAdaptorConvertsStoredTaskToOpenAIVideo(t *testing.T) {
	adaptor := &TaskAdaptor{}
	task := newTestTaskForOpenAIVideo()
	body, err := adaptor.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("convert to openai video: %v", err)
	}
	if got := gjson.GetBytes(body, "id").String(); got != "task_public" {
		t.Fatalf("unexpected id: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "status").String(); got != dto.VideoStatusCompleted {
		t.Fatalf("unexpected status: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "metadata.url").String(); got != "https://cdn.example/video.mp4" {
		t.Fatalf("unexpected url metadata: %s body=%s", got, body)
	}
}

func TestTaskAdaptorConvertsHappyHorseStoredTaskToOpenAIVideo(t *testing.T) {
	now := time.Now().Unix()
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: now - 60,
		UpdatedAt: now,
		Data:      []byte(`{"output":{"task_id":"dashscope-task","task_status":"SUCCEEDED","video_url":"https://dashscope-result/video.mp4"}}`),
	}
	body, err := adaptor.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("convert to openai video: %v", err)
	}
	if got := gjson.GetBytes(body, "metadata.url").String(); got != "https://dashscope-result/video.mp4" {
		t.Fatalf("unexpected url metadata: %s body=%s", got, body)
	}
}

func TestTaskAdaptorConvertsCommonStoredTaskResultPathsToOpenAIVideo(t *testing.T) {
	now := time.Now().Unix()
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: now - 60,
		UpdatedAt: now,
		Data:      []byte(`{"data":{"video_url":"https://cdn.example/custom.mp4"}}`),
	}
	body, err := adaptor.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("convert to openai video: %v", err)
	}
	if got := gjson.GetBytes(body, "metadata.url").String(); got != "https://cdn.example/custom.mp4" {
		t.Fatalf("unexpected url metadata: %s body=%s", got, body)
	}
}

func TestTaskAdaptorKeepsNotStartStoredTaskStatusInOpenAIVideo(t *testing.T) {
	now := time.Now().Unix()
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusNotStart,
		Progress:  "0%",
		CreatedAt: now - 60,
		UpdatedAt: now,
		Data:      []byte(`{"output":{"task_id":"dashscope-task"}}`),
	}
	body, err := adaptor.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("convert to openai video: %v", err)
	}
	if got := gjson.GetBytes(body, "status").String(); got != string(model.TaskStatusNotStart) {
		t.Fatalf("unexpected status: %s body=%s", got, body)
	}
}

func TestTaskAdaptorAppliesConfiguredOpenAIVideoFetchResponseFields(t *testing.T) {
	db := openConfigurableTaskAdaptorTestDB(t)
	channel := model.Channel{
		Id:     9001,
		Type:   constant.ChannelTypeConfigurable,
		Key:    "sk-test",
		Status: common.ChannelStatusEnabled,
		Name:   "happyhorse",
		Models: "happyhorse-1.0-t2v",
		Group:  "default",
	}
	channel.SetSetting(dto.ChannelSettings{
		Protocol: &dto.ChannelProtocolSettings{
			ProfileID: "happyhorse-video",
		},
	})
	require.NoError(t, db.Create(&channel).Error)

	now := time.Now().Unix()
	adaptor := &TaskAdaptor{}
	task := &model.Task{
		TaskID:    "task_public",
		ChannelId: channel.Id,
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: now - 60,
		UpdatedAt: now,
		Properties: model.Properties{
			OriginModelName:   "happyhorse-1.0-t2v",
			UpstreamModelName: "happyhorse-1.0-t2v",
		},
		Data: []byte(`{
			"request_id":"req-1",
			"output":{"task_id":"dashscope-task","task_status":"SUCCEEDED","video_url":"https://dashscope-result/video.mp4"},
			"usage":{"duration":5,"ratio":"16:9"}
		}`),
	}
	body, err := adaptor.ConvertToOpenAIVideo(task)
	if err != nil {
		t.Fatalf("convert to openai video: %v", err)
	}
	if got := gjson.GetBytes(body, "metadata.url").String(); got != "https://dashscope-result/video.mp4" {
		t.Fatalf("unexpected url metadata: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "video_url").String(); got != "https://dashscope-result/video.mp4" {
		t.Fatalf("unexpected video_url: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "seconds").String(); got != "5" {
		t.Fatalf("unexpected seconds: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "metadata.request_id").String(); got != "req-1" {
		t.Fatalf("unexpected request_id metadata: %s body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "metadata.usage.duration").Int(); got != 5 {
		t.Fatalf("unexpected duration metadata: %d body=%s", got, body)
	}
	if got := gjson.GetBytes(body, "metadata.usage.ratio").String(); got != "16:9" {
		t.Fatalf("unexpected ratio metadata: %s body=%s", got, body)
	}
}

func newTestTaskForOpenAIVideo() *model.Task {
	now := time.Now().Unix()
	return &model.Task{
		TaskID:    "task_public",
		Status:    model.TaskStatusSuccess,
		Progress:  "100%",
		CreatedAt: now - 60,
		UpdatedAt: now,
		Data:      []byte(`{"id":"upstream-task","status":"completed","video":{"url":"https://cdn.example/video.mp4"}}`),
	}
}

func openConfigurableTaskAdaptorTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Channel{}))
	model.DB = db

	t.Cleanup(func() {
		model.DB = originalDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func buildHappyHorseRequestBody(t *testing.T, requestBody []byte, upstreamModel string) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	if _, err := common.GetBodyStorage(c); err != nil {
		t.Fatalf("cache body: %v", err)
	}

	info := happyHorseRelayInfo(upstreamModel)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if err := adaptor.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatalf("validate request: %v", err)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	mappedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read mapped body: %v", err)
	}
	return mappedBody
}

func happyHorseRelayInfo(upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: upstreamModel,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeConfigurable,
			ChannelBaseUrl:    "https://dashscope.aliyuncs.com",
			ApiKey:            "sk-test",
			UpstreamModelName: upstreamModel,
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "happyhorse-video",
				},
			},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func buildSeedanceRequestBody(t *testing.T, requestBody []byte, upstreamModel string) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", bytes.NewReader(requestBody))
	c.Request.Header.Set("Content-Type", "application/json")
	if _, err := common.GetBodyStorage(c); err != nil {
		t.Fatalf("cache body: %v", err)
	}

	info := seedanceRelayInfo(upstreamModel)
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	if err := adaptor.ValidateRequestAndSetAction(c, info); err != nil {
		t.Fatalf("validate request: %v", err)
	}

	reader, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		t.Fatalf("build body: %v", err)
	}
	mappedBody, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read mapped body: %v", err)
	}
	return mappedBody
}

func seedanceRelayInfo(upstreamModel string) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		OriginModelName: upstreamModel,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeConfigurable,
			ChannelBaseUrl:    "https://api.llm.jimuall.com",
			ApiKey:            "sk-test",
			UpstreamModelName: upstreamModel,
			ChannelSetting: dto.ChannelSettings{
				Protocol: &dto.ChannelProtocolSettings{
					ProfileID: "doubao-seedance-2",
				},
			},
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}
