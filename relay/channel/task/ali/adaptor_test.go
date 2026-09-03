package ali

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func testRelayInfo() *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
}

func TestConvertToAliRequestWan27I2VBuildsMediaFromImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:    "wan2.7-i2v",
		Prompt:   "animate the first frame",
		Image:    "https://example.com/first.png",
		Size:     "720p",
		Duration: 10,
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "wan2.7-i2v", aliReq.Model)
	require.Equal(t, "720P", aliReq.Parameters.Resolution)
	require.Equal(t, 10, aliReq.Parameters.Duration)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestModelListIncludesWan3Models(t *testing.T) {
	require.Contains(t, ModelList, "wan3.0-video")
	require.Contains(t, ModelList, "wan3.0-video-prime")
}

func TestConvertToWan3RequestSupportsNativeFieldsThroughMetadata(t *testing.T) {
	req := relaycommon.TaskSubmitReq{
		Model:  "wan3.0-video",
		Prompt: "edit video",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{"type": "reference_video", "url": "https://example.com/source.mp4"},
					map[string]interface{}{"type": "reference_audio", "url": "https://example.com/audio.mp3"},
				},
			},
			"parameters": map[string]interface{}{
				"resolution": "1080P", "ratio": "adaptive", "duration": 6,
				"audio": false, "seed": 0, "prompt_extend": false, "watermark": false,
			},
		},
	}

	wan3Req, err := (&TaskAdaptor{}).convertToWan3Request(testRelayInfo(), req)
	require.NoError(t, err)
	require.NoError(t, validateWan3Request(wan3Req))
	body, err := common.Marshal(wan3Req)
	require.NoError(t, err)
	require.Equal(t, int64(2), gjson.GetBytes(body, "input.media.#").Int())
	require.True(t, gjson.GetBytes(body, "parameters.audio").Exists())
	require.False(t, gjson.GetBytes(body, "parameters.audio").Bool())
	require.True(t, gjson.GetBytes(body, "parameters.seed").Exists())
	require.Equal(t, int64(0), gjson.GetBytes(body, "parameters.seed").Int())
	require.True(t, gjson.GetBytes(body, "parameters.prompt_extend").Exists())
	require.False(t, gjson.GetBytes(body, "parameters.prompt_extend").Bool())
}

func TestConvertToAliRequestWan27I2VBuildsFirstAndLastFrameFromImages(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "interpolate between frames",
		Images: []string{
			"https://example.com/first.png",
			"https://example.com/last.png",
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VPrefersImageBeforeImagesAndInputReference(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "use the direct image",
		Image:          " https://example.com/direct.png ",
		Images:         []string{"https://example.com/images-first.png", " https://example.com/images-last.png "},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/direct.png"},
		{Type: "last_frame", URL: "https://example.com/images-last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VFallsBackToFirstNonEmptyImage(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "skip blank images",
		Image:  " ",
		Images: []string{
			" ",
			" https://example.com/first.png ",
			" https://example.com/last.png ",
		},
		InputReference: "https://example.com/input-reference.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_frame", URL: "https://example.com/first.png"},
		{Type: "last_frame", URL: "https://example.com/last.png"},
	}, aliReq.Input.Media)
}

func TestConvertToAliRequestWan27I2VKeepsExplicitMetadataMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:          "wan2.7-i2v",
		Prompt:         "continue the clip",
		Image:          "https://example.com/direct.png",
		Images:         []string{"https://example.com/images-first.png", "https://example.com/images-last.png"},
		InputReference: "https://example.com/input-reference.png",
		Metadata: map[string]interface{}{
			"input": map[string]interface{}{
				"media": []interface{}{
					map[string]interface{}{
						"type": "first_clip",
						"url":  "https://example.com/input.mp4",
					},
				},
			},
		},
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, []AliVideoMedia{
		{Type: "first_clip", URL: "https://example.com/input.mp4"},
	}, aliReq.Input.Media)
	require.Empty(t, aliReq.Input.ImgURL)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"media"`)
	require.NotContains(t, string(body), `"img_url"`)
}

func TestConvertToAliRequestWan27I2VRequiresMedia(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.7-i2v",
		Prompt: "animate without a frame",
	}

	_, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "requires image"))
}

func TestConvertToAliRequestWan25I2VKeepsLegacyImgURL(t *testing.T) {
	adaptor := &TaskAdaptor{}
	req := relaycommon.TaskSubmitReq{
		Model:  "wan2.5-i2v-preview",
		Prompt: "animate the first frame",
		Image:  "https://example.com/first.png",
	}

	aliReq, err := adaptor.convertToAliRequest(testRelayInfo(), req)

	require.NoError(t, err)
	require.Equal(t, "https://example.com/first.png", aliReq.Input.ImgURL)
	require.Empty(t, aliReq.Input.Media)

	body, err := common.Marshal(aliReq)
	require.NoError(t, err)
	require.Contains(t, string(body), `"img_url"`)
	require.NotContains(t, string(body), `"media"`)
}

func wan3NativeContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("configurable_native_profile_id", "dashscope-wan3-video")
	return c, recorder
}

func TestWan3NativeRequestPreservesAllParameters(t *testing.T) {
	body := `{
		"model":"wan3.0-video",
		"input":{"prompt":"animate references","media":[
			{"type":"reference_image","url":"https://example.com/ref.png"},
			{"type":"reference_video","url":"https://example.com/ref.mp4"},
			{"type":"reference_audio","url":"https://example.com/ref.wav"}
		]},
		"parameters":{"resolution":"480P","ratio":"9:16","duration":-1,"audio":false,"seed":0,"prompt_extend":false,"watermark":false},
		"future_field":{"kept":true}
	}`
	c, _ := wan3NativeContext(t, body)
	info := testRelayInfo()
	info.OriginModelName = "wan3.0-video"
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	upstream, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.JSONEq(t, body, string(upstream))
	require.True(t, gjson.GetBytes(upstream, "parameters.audio").Exists())
	require.False(t, gjson.GetBytes(upstream, "parameters.audio").Bool())
	require.True(t, gjson.GetBytes(upstream, "parameters.seed").Exists())
	require.Equal(t, int64(0), gjson.GetBytes(upstream, "parameters.seed").Int())
	require.Equal(t, int64(-1), gjson.GetBytes(upstream, "parameters.duration").Int())
	require.True(t, gjson.GetBytes(upstream, "future_field.kept").Bool())

	ratios := adaptor.EstimateBilling(c, info)
	require.Equal(t, 30.0, ratios["seconds"])
	require.Equal(t, 1.0, ratios["resolution-480P"])
}

func TestWan3NativeRequestSupportsMediaOnlyFileInput(t *testing.T) {
	c, _ := wan3NativeContext(t, `{
		"model":"wan3.0-video-prime",
		"input":{"media":[{"type":"file","url":"https://example.com/source.zip"}]},
		"parameters":{"duration":2}
	}`)
	info := testRelayInfo()
	info.OriginModelName = "wan3.0-video-prime"
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
}

func TestWan3NativeRequestAllowsReferenceAndFileInputs(t *testing.T) {
	c, _ := wan3NativeContext(t, `{
		"model":"wan3.0-video",
		"input":{"media":[
			{"type":"reference_image","url":"https://example.com/ref.png"},
			{"type":"file","url":"https://example.com/story.pdf"}
		]}
	}`)
	info := testRelayInfo()
	info.OriginModelName = "wan3.0-video"
	require.Nil(t, (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info))
}

func TestWan3NativeRequestRejectsMutuallyExclusiveMedia(t *testing.T) {
	c, _ := wan3NativeContext(t, `{
		"model":"wan3.0-video",
		"input":{"media":[
			{"type":"first_frame","url":"https://example.com/first.png"},
			{"type":"reference_image","url":"https://example.com/ref.png"}
		]}
	}`)
	info := testRelayInfo()
	info.OriginModelName = "wan3.0-video"
	taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	require.Contains(t, taskErr.Message, "cannot be combined")
	require.JSONEq(t, `{"code":"InvalidParameter","message":"first_frame/last_frame cannot be combined with reference, file, or link media"}`, string(taskErr.RawBody))
}

func TestWan3UsageParsesFloatsAndNativeFetchPreservesPayload(t *testing.T) {
	response := []byte(`{
		"request_id":"req-1",
		"output":{"task_id":"upstream-1","task_status":"SUCCEEDED","submit_time":"s","scheduled_time":"q","end_time":"e","orig_prompt":"o","actual_prompt":"a","video_url":"https://example.com/out.mp4"},
		"usage":{"video_count":1,"duration":5.5,"input_video_duration":2.25,"output_video_duration":3.25,"fps":24,"SR":720,"ratio":"16:9"},
		"future_field":{"kept":true}
	}`)
	adaptor := &TaskAdaptor{}
	result, err := adaptor.ParseTaskResult(response)
	require.NoError(t, err)
	require.Equal(t, string(model.TaskStatusSuccess), result.Status)
	require.Equal(t, "https://example.com/out.mp4", result.Url)

	task := &model.Task{TaskID: "task_public", Data: response, Properties: model.Properties{OriginModelName: "wan3.0-video"}}
	native, err := adaptor.ConvertToNativeFetchResponse(task, response)
	require.NoError(t, err)
	require.Equal(t, "task_public", gjson.GetBytes(native, "output.task_id").String())
	require.True(t, gjson.GetBytes(native, "future_field.kept").Bool())
	require.Equal(t, 2.25, gjson.GetBytes(native, "usage.input_video_duration").Float())

	openAI, err := adaptor.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	require.Equal(t, "3.25", gjson.GetBytes(openAI, "seconds").String())
	require.Equal(t, "720P", gjson.GetBytes(openAI, "size").String())
	require.Equal(t, 24, int(gjson.GetBytes(openAI, "metadata.usage.fps").Int()))
	require.Equal(t, "req-1", gjson.GetBytes(openAI, "metadata.request_id").String())
}

func TestWan3NativeSubmitResponsePreservesAllFieldsAndPublicTaskID(t *testing.T) {
	c, recorder := wan3NativeContext(t, `{"model":"wan3.0-video","input":{"prompt":"cat"}}`)
	upstream := `{"request_id":"req-1","output":{"task_id":"upstream-1","task_status":"PENDING"},"future_field":{"kept":true}}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(upstream)),
	}
	info := testRelayInfo()
	info.PublicTaskID = "task_public"

	taskID, taskData, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	require.Equal(t, "upstream-1", taskID)
	require.JSONEq(t, upstream, string(taskData))
	require.Equal(t, "task_public", gjson.GetBytes(recorder.Body.Bytes(), "output.task_id").String())
	require.True(t, gjson.GetBytes(recorder.Body.Bytes(), "future_field.kept").Bool())
}

func TestWan3NativeSubmitErrorPreservesProviderPayload(t *testing.T) {
	c, _ := wan3NativeContext(t, `{"model":"wan3.0-video","input":{"prompt":"cat"}}`)
	upstream := `{"request_id":"req-err","code":"InvalidParameter","message":"bad ratio","details":{"field":"ratio"}}`
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Body:       io.NopCloser(strings.NewReader(upstream)),
	}

	_, _, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, testRelayInfo())
	require.NotNil(t, taskErr)
	require.JSONEq(t, upstream, string(taskErr.RawBody))
}

func TestWan3AdjustBillingUsesInputAndOutputDuration(t *testing.T) {
	task := &model.Task{
		Quota:      4000,
		Properties: model.Properties{OriginModelName: "wan3.0-video"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			OtherRatios: map[string]float64{"seconds": 5, "resolution-720P": 2},
		}},
		Data: []byte(`{"usage":{"input_video_duration":2.5,"output_video_duration":3.5,"SR":1080}}`),
	}

	quota := (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil)
	// base=4000/(5*2)=400; actual=400*(2.5+3.5)*4
	require.Equal(t, 9600, quota)
}

func TestWan3AdjustBillingSkipsFixedPerCallTask(t *testing.T) {
	task := &model.Task{
		Quota:      4000,
		Properties: model.Properties{OriginModelName: "wan3.0-video"},
		PrivateData: model.TaskPrivateData{BillingContext: &model.TaskBillingContext{
			PerCallBilling: true,
			OtherRatios:    map[string]float64{"seconds": 5, "resolution-720P": 2},
		}},
		Data: []byte(`{"usage":{"input_video_duration":2.5,"output_video_duration":3.5,"SR":1080}}`),
	}
	require.Zero(t, (&TaskAdaptor{}).AdjustBillingOnComplete(task, nil))
}
