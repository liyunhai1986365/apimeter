package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/imageconv"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestImageSafetyIdentifierPolicyAcrossDTOAdaptors(t *testing.T) {
	global := model_setting.GetGlobalSettings()
	original := global.PassThroughRequestEnabled
	t.Cleanup(func() { global.PassThroughRequestEnabled = original })
	for _, provider := range []struct {
		name    string
		apiType int
	}{
		{"OpenAI", constant.APITypeOpenAI},
		{"Volcengine", constant.APITypeVolcEngine},
		{"Zhipu", constant.APITypeZhipuV4},
	} {
		for _, policy := range []struct {
			name                                       string
			allow, channelPass, globalPass, noMetadata bool
		}{
			{name: "disabled"},
			{name: "allowed", allow: true},
			{name: "channel passthrough", channelPass: true},
			{name: "global passthrough", globalPass: true},
			{name: "missing metadata", noMetadata: true},
			{name: "global passthrough without metadata", globalPass: true, noMetadata: true},
		} {
			t.Run(provider.name+"/"+policy.name, func(t *testing.T) {
				global.PassThroughRequestEnabled = policy.globalPass
				info := &relaycommon.RelayInfo{
					RelayMode: relayconstant.RelayModeImagesGenerations,
					ChannelMeta: &relaycommon.ChannelMeta{
						ChannelSetting:       dto.ChannelSettings{PassThroughBodyEnabled: policy.channelPass},
						ChannelOtherSettings: dto.ChannelOtherSettings{AllowSafetyIdentifier: policy.allow},
					},
				}
				if policy.noMetadata {
					info.ChannelMeta = nil
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
				c.Request.Header.Set("Content-Type", "application/json")
				for _, identifier := range []string{"user-private", ""} {
					request := dto.ImageRequest{Model: "image-model", Prompt: "draw", SafetyIdentifier: common.GetPointer(identifier)}
					converted, err := GetAdaptor(provider.apiType).ConvertImageRequest(c, info, request)
					require.NoError(t, err)
					wire, err := common.Marshal(converted)
					require.NoError(t, err)
					field := gjson.GetBytes(wire, "safety_identifier")
					if policy.allow || policy.channelPass || policy.globalPass {
						require.True(t, field.Exists())
						require.Equal(t, identifier, field.String())
					} else {
						require.False(t, field.Exists())
					}
					require.Equal(t, identifier, *request.SafetyIdentifier, "must preserve the retry source")
					require.Equal(t, "image-model", gjson.GetBytes(wire, "model").String())
					require.Equal(t, "draw", gjson.GetBytes(wire, "prompt").String())
				}
			})
		}
	}
}

func TestImageNullMetadataDoesNotChangeOtherDTOAdaptors(t *testing.T) {
	for _, apiType := range []int{constant.APITypeOpenAI, constant.APITypeVolcEngine, constant.APITypeZhipuV4} {
		t.Run(fmt.Sprint(apiType), func(t *testing.T) {
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"image-model","prompt":"draw","complexity":null,"frame_count":null,"frame_duration_ms":null,"safety_identifier":null}`), &request))
			attempt, err := common.DeepCopy(&request)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			converted, err := GetAdaptor(apiType).ConvertImageRequest(c, &relaycommon.RelayInfo{
				RelayMode: relayconstant.RelayModeImagesGenerations,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelOtherSettings: dto.ChannelOtherSettings{AllowSafetyIdentifier: true},
				},
			}, *attempt)
			require.NoError(t, err)
			wire, err := common.Marshal(converted)
			require.NoError(t, err)
			require.JSONEq(t, `{"model":"image-model","prompt":"draw"}`, string(wire))
		})
	}
}

func TestLegacyImageEditMultipartPreservesNullableScalarSemantics(t *testing.T) {
	fields := []string{"background", "moderation", "output_format", "output_compression", "partial_images", "user"}
	for _, tc := range []struct {
		name, fields string
		want         map[string][]string
	}{
		{name: "omitted"},
		{name: "JSON null", fields: `,"background":null,"moderation":null,"output_format":null,"output_compression":null,"partial_images":null,"user":null`},
		{
			name:   "literal strings and zero",
			fields: `,"background":"null","moderation":"","output_format":"null","output_compression":0,"partial_images":0,"user":" user ","stream":false`,
			want: map[string][]string{
				"background": {"null"}, "moderation": {""}, "output_format": {"null"},
				"output_compression": {"0"}, "partial_images": {"0"}, "user": {" user "}, "stream": {"false"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request dto.ImageRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-image-2","prompt":"edit","image":"data:image/png;base64,`+imageTaskTestPNGBase64+`"`+tc.fields+`}`), &request))
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			body, err := imageconv.BuildOpenAIJSONEditMultipart(c, request)
			require.NoError(t, err)
			_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
			require.NoError(t, err)
			form, err := multipart.NewReader(body, params["boundary"]).ReadForm(1 << 20)
			require.NoError(t, err)
			defer form.RemoveAll()
			if tc.want == nil {
				for _, field := range fields {
					require.NotContains(t, form.Value, field)
				}
			} else {
				for field, values := range tc.want {
					require.Equal(t, values, form.Value[field], field)
				}
			}
			require.Equal(t, []string{"gpt-image-2"}, form.Value["model"])
			require.Equal(t, []string{"edit"}, form.Value["prompt"])
			require.Len(t, form.File["image"], 1)
		})
	}
}

func TestImageParametersReachOpenAIJSONUpstream(t *testing.T) {
	var references []string
	for i := 0; i < 16; i++ {
		references = append(references, fmt.Sprintf(`{"image_url":"https://input.invalid/%d.png"}`, i))
	}
	for _, tc := range []struct {
		name, endpoint, fields, want string
		overrides                    map[string]any
	}{
		{
			name: "GIF", endpoint: "generations",
			fields: `"complexity":"low","frame_count":16,"frame_duration_ms":100,"output_format":"gif","quality":"max","size":"1536x864","safety_identifier":"user-hash"`,
		},
		{
			name: "SVG", endpoint: "generations",
			fields: `"output_format":"svg","svg":{"model":"author-model","reasoning":{"effort":"high"},"extension":9007199254740993}`,
		},
		{
			name: "16 URL references", endpoint: "edits",
			fields: `"images":[` + strings.Join(references, ",") + `],"complexity":"high"`,
		},
		{
			name: "file reference and mask", endpoint: "edits",
			fields: `"images":[{"file_id":"file-image"}],"mask":{"file_id":"file-mask"},"input_fidelity":"high"`,
		},
		{
			name: "data URL and markup", endpoint: "edits",
			fields: `"images":[{"image_url":"data:image/png;base64,` + imageTaskTestPNGBase64 + `"}],"markup":{"image_url":"https://input.invalid/markup.png"}`,
		},
		{
			name: "explicit empty and zero", endpoint: "generations",
			fields: `"complexity":"","frame_count":0,"frame_duration_ms":0,"safety_identifier":"","stream":false,"partial_images":0,"output_compression":0`,
		},
		{
			name: "override and delete", endpoint: "generations",
			fields: `"complexity":"low","frame_count":16,"frame_duration_ms":100`,
			want:   `"complexity":"high","frame_duration_ms":250`,
			overrides: map[string]any{"operations": []any{
				map[string]any{"mode": "set", "path": "complexity", "value": "high"},
				map[string]any{"mode": "set", "path": "frame_duration_ms", "value": 250},
				map[string]any{"mode": "delete", "path": "frame_count"},
			}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			received := make(chan []byte, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/v1/images/"+tc.endpoint, r.URL.Path)
				require.Contains(t, r.Header.Get("Content-Type"), "application/json")
				body, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				received <- body
				w.Header().Set("Content-Type", "application/json")
				// Request compatibility only: the mock returns a regular PNG.
				_, _ = fmt.Fprint(w, `{"data":[{"b64_json":"`+imageTaskTestPNGBase64+`"}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
			}))
			defer upstream.Close()
			ctx, _, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL,
				&dto.ImageRequest{Model: "image-alias", Prompt: "draw"})
			path := "/v1/images/" + tc.endpoint
			body := `{"model":"image-alias","prompt":"draw","unknown_extension":true,` + tc.fields + `}`
			ctx.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			if tc.endpoint == "edits" {
				info.RelayMode = relayconstant.RelayModeImagesEdits
			}
			info.RequestURLPath = path
			request, err := helper.GetAndValidOpenAIImageRequest(ctx, info.RelayMode)
			require.NoError(t, err)
			info.Request = request
			ctx.Set("model_mapping", `{"image-alias":"gpt-image-2.6-sunburst"}`)
			common.SetContextKey(ctx, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AllowSafetyIdentifier: true})
			common.SetContextKey(ctx, constant.ContextKeyChannelParamOverride, tc.overrides)
			require.Nil(t, ImageHelper(ctx, info))
			require.Len(t, received, 1)
			wire := <-received
			require.Equal(t, "gpt-image-2.6-sunburst", gjson.GetBytes(wire, "model").String())
			require.False(t, gjson.GetBytes(wire, "unknown_extension").Exists())
			want := tc.want
			if want == "" {
				want = tc.fields
			}
			var expected map[string]json.RawMessage
			require.NoError(t, common.Unmarshal([]byte("{"+want+"}"), &expected))
			for key, value := range expected {
				require.JSONEq(t, string(value), gjson.GetBytes(wire, key).Raw, key)
			}
			if tc.overrides != nil {
				require.False(t, gjson.GetBytes(wire, "frame_count").Exists())
			}
			if tc.name == "SVG" {
				require.Equal(t, "9007199254740993", gjson.GetBytes(wire, "svg.extension").Raw)
			}
			require.Equal(t, "image-alias", request.Model, "mapping must not mutate the retry source")
			require.Equal(t, 1, info.Billing.(*imageAsyncBillingStub).settleCalls)
		})
	}
}

func TestImageParametersLeaveRangeValidationToUpstream(t *testing.T) {
	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = io.WriteString(w, `{"error":{"message":"unsupported frame count","type":"invalid_request_error","code":"invalid_frame_count"}}`)
	}))
	defer upstream.Close()
	request := &dto.ImageRequest{Model: "gpt-image-2.6-sunburst", Prompt: "draw", FrameCount: common.GetPointer(-1)}
	c, _, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
	apiErr := ImageHelper(c, info)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusUnprocessableEntity, apiErr.StatusCode)
	require.Equal(t, "invalid_frame_count", string(apiErr.GetErrorCode()))
	require.Len(t, received, 1)
	require.EqualValues(t, -1, gjson.GetBytes(<-received, "frame_count").Int())
	require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls)
}

func TestImageNewParameterNullsAndCopy(t *testing.T) {
	for _, fields := range []string{
		`"svg":null,"markup":null`,
		`"complexity":"","frame_count":0,"frame_duration_ms":0,"safety_identifier":"","svg":{"reasoning":{"effort":"low"}},"markup":{"file_id":"file-markup"}`,
	} {
		var request dto.ImageRequest
		require.NoError(t, common.Unmarshal([]byte(`{"model":"m","prompt":"p",`+fields+`}`), &request))
		copy, err := common.DeepCopy(&request)
		require.NoError(t, err)
		wire, err := common.Marshal(copy)
		require.NoError(t, err)
		require.JSONEq(t, `{"model":"m","prompt":"p",`+fields+`}`, string(wire))
		if copy.Complexity != nil {
			*copy.Complexity = "high"
			*copy.FrameCount = 16
			copy.SVG[0] = ' '
			require.Empty(t, *request.Complexity)
			require.Zero(t, *request.FrameCount)
			require.True(t, bytes.HasPrefix(request.SVG, []byte("{")))
		}
	}
	var nulls dto.ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"m","prompt":"p","complexity":null,"frame_count":null,"frame_duration_ms":null,"safety_identifier":null}`), &nulls))
	wire, err := common.Marshal(nulls)
	require.NoError(t, err)
	require.JSONEq(t, `{"model":"m","prompt":"p"}`, string(wire))
}

func TestLocalImageAsyncWorkerKeepsNewParameters(t *testing.T) {
	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"b64_json":"`+imageTaskTestPNGBase64+`"}],"usage":{"input_tokens":10,"output_tokens":20,"total_tokens":30}}`)
	}))
	defer upstream.Close()
	request := &dto.ImageRequest{
		Model: "gpt-image-2.6-sunburst", Prompt: "draw", Complexity: common.GetPointer("low"),
		FrameCount: common.GetPointer(16), FrameDurationMS: common.GetPointer(100), SafetyIdentifier: common.GetPointer("user-hash"),
	}
	c, _, info := newImageResponsePolicyTestContext(t, constant.ChannelTypeOpenAI, upstream.URL, request)
	common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AllowSafetyIdentifier: true})
	info.InitChannelMeta(c)
	info.IsAsyncImageRequest = true
	body, err := common.Marshal(request)
	require.NoError(t, err)
	task, apiErr := insertLocalImageAsyncTask(info, body)
	require.Nil(t, apiErr)
	worker := cloneImageAsyncGinContext(c, body)
	// Run the real worker synchronously so test teardown cannot race it.
	runLocalImageAsyncTask(worker, info, task)
	require.Len(t, received, 1)
	wire := <-received
	require.Equal(t, "low", gjson.GetBytes(wire, "complexity").String())
	require.EqualValues(t, 16, gjson.GetBytes(wire, "frame_count").Int())
	require.EqualValues(t, 100, gjson.GetBytes(wire, "frame_duration_ms").Int())
	require.Equal(t, "user-hash", gjson.GetBytes(wire, "safety_identifier").String())
	saved, exists, err := model.GetByOnlyTaskId(task.TaskID)
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
	require.Equal(t, 30, saved.Quota)
	var user model.User
	require.NoError(t, model.DB.First(&user, 7).Error)
	require.Equal(t, 30, user.UsedQuota)
	require.Equal(t, 1, user.RequestCount)
	require.Zero(t, info.Billing.(*imageAsyncBillingStub).settleCalls, "the task worker settles through task billing only")
}
