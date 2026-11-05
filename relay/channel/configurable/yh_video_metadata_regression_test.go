package configurable

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHMetadataDurationFormats(t *testing.T) {
	for _, format := range []string{"json_object", "json_string", "form", "multipart"} {
		for _, duration := range []int{5, 12, 30, -1, 0} {
			t.Run(fmt.Sprintf("%s/%d", format, duration), func(t *testing.T) {
				md := fmt.Sprintf(`{"duration":%d,"service_tier":"flex"}`, duration)
				var body bytes.Buffer
				ct := "application/json"
				switch format {
				case "json_object":
					body.WriteString(`{"model":"client-alias","prompt":"scene","metadata":` + md + `}`)
				case "json_string":
					enc, err := common.Marshal(md)
					require.NoError(t, err)
					body.WriteString(`{"model":"client-alias","prompt":"scene","metadata":` + string(enc) + `}`)
				case "form":
					ct = "application/x-www-form-urlencoded"
					body.WriteString(url.Values{"model": {"client-alias"}, "prompt": {"scene"}, "metadata": {md}}.Encode())
				case "multipart":
					w := multipart.NewWriter(&body)
					require.NoError(t, w.WriteField("model", "client-alias"))
					require.NoError(t, w.WriteField("prompt", "scene"))
					require.NoError(t, w.WriteField("metadata", md))
					require.NoError(t, w.Close())
					ct = w.FormDataContentType()
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
				c.Request.Header.Set("Content-Type", ct)
				defer common.CleanupBodyStorage(c)
				var routing map[string]any
				require.NoError(t, common.UnmarshalBodyReusable(c, &routing))
				info := seedanceMaxRelayInfo("seedance-yh")
				a := &TaskAdaptor{}
				a.Init(info)
				require.Nil(t, a.ValidateRequestAndSetAction(c, info))
				req, err := relaycommon.GetTaskRequest(c)
				require.NoError(t, err)
				require.Equal(t, duration, req.Duration)
				want := float64(duration)
				if duration == -1 {
					want = 30
				}
				if duration == 0 {
					want = 4
				}
				require.Equal(t, want, a.EstimateBilling(c, info)["seconds"])
				r, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				wire, err := io.ReadAll(r)
				require.NoError(t, err)
				require.Equal(t, fmt.Sprint(duration), gjson.GetBytes(wire, "metadata.duration").Raw)
				require.Equal(t, "flex", c.GetString("yh_video_service_tier"))
			})
		}
	}
}
func TestYHServiceTierResetAndFallback(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	_, err := buildYHVideoRequest(c, []byte(`{"model":"m","content":[],"service_tier":"flex"}`), nil, "")
	require.NoError(t, err)
	require.Equal(t, "flex", c.GetString("yh_video_service_tier"))
	_, err = buildYHVideoRequest(c, []byte(`{"model":"m","content":[]}`), nil, "")
	require.NoError(t, err)
	require.Empty(t, c.GetString("yh_video_service_tier"))
	task := &model.Task{TaskID: "cgt-1", Status: model.TaskStatusQueued, PrivateData: model.TaskPrivateData{YHServiceTier: "flex"}}
	for _, body := range []string{``, `{"task_id":"cgt-1","status":"QUEUED"}`, `{"data":{"task_id":"cgt-1","status":"IN_PROGRESS","data":null}}`} {
		response, err := BuildYHVideoTaskResponse(task, []byte(body))
		require.NoError(t, err)
		require.Equal(t, "flex", gjson.GetBytes(response, "service_tier").String())
	}
	response, err := BuildYHVideoTaskResponse(task, []byte(`{"data":{"task_id":"cgt-1","status":"IN_PROGRESS","data":{"service_tier":"default"}}}`))
	require.NoError(t, err)
	require.Equal(t, "default", gjson.GetBytes(response, "service_tier").String())
}

func TestYHMetadataDurationValidationAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name, fields, value string
		want                int
		invalid             bool
	}{
		{"bounds", "", "31", 0, true},
		{"negative", "", "-2", 0, true},
		{"fraction", "", "5.5", 0, true},
		{"object", "", "{}", 0, true},
		{"null", "", "null", 0, false},
		{"string_integer", "", `"12"`, 12, false},
		{"top_level_duration", `"duration":5,`, "12", 5, false},
		{"top_level_seconds", `"seconds":"5",`, "12", 5, false},
		{"top_level_null", `"duration":null,`, "12", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"client-alias","prompt":"test",`+tc.fields+`"metadata":{"duration":`+tc.value+`}}`))
			c.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(c)
			info := seedanceMaxRelayInfo("seedance-yh")
			a := &TaskAdaptor{}
			a.Init(info)
			err := a.ValidateRequestAndSetAction(c, info)
			if tc.invalid {
				require.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			req, reqErr := relaycommon.GetTaskRequest(c)
			require.NoError(t, reqErr)
			require.Equal(t, tc.want, req.RequestedDuration())
		})
	}
	// Other Seedance suppliers retain their existing metadata interpretation.
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"client-alias","prompt":"test","metadata":{"duration":12}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)
	info := seedanceMaxRelayInfo("seedance-tgxmaas")
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	req, err := relaycommon.GetTaskRequest(c)
	require.NoError(t, err)
	require.Zero(t, req.Duration)
}

func TestYHMetadataEditDuration(t *testing.T) {
	body := `{"model":"client-alias","prompt":"test","metadata":{"duration":-1,"omni_reference_task_type":"edit","ratio":"adaptive","video_url":"https://example.com/video.mp4"}}`
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	defer common.CleanupBodyStorage(c)
	info := seedanceMaxRelayInfo("seedance-yh")
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info), "metadata duration=-1 is accepted for forwarding and must also satisfy edit validation")
}

func TestYHDurationForwardingAndPrecedence(t *testing.T) {
	for _, format := range []string{"json_object", "json_string", "form", "multipart"} {
		for _, tc := range []struct{ name, fields, want string }{
			{"zero_without_metadata", `"duration":0`, "0"},
			{"zero_with_metadata", `"duration":0,"metadata":{"duration":12}`, "0"},
			{"string_zero", `"duration":"0"`, "0"},
			{"seconds_zero", `"seconds":"0","metadata":{"duration":12}`, "0"},
			{"seconds_wins", `"duration":0,"seconds":"5","metadata":{"duration":12}`, "5"},
			{"blank_seconds_zero", `"duration":0,"seconds":" \t","metadata":{"duration":12}`, "0"},
			{"blank_seconds_nonzero", `"duration":5,"seconds":" \t","metadata":{"duration":12}`, "5"},
			{"capital_duration", `"Duration":5,"omni_reference_task_type":"auto","metadata":{"duration":12}`, "5"},
			{"capital_seconds", `"Seconds":"5","omni_reference_task_type":"auto","metadata":{"duration":12}`, "5"},
			{"upper_zero", `"DURATION":0,"METADATA":{"duration":12}`, "0"},
			{"mixed_seconds", `"sEcOnDs":"5","MeTaDaTa":{"duration":12,"seed":9007199254740993}`, "5"},
			{"absent", `"metadata":{"draft":true}`, ""},
			{"no_metadata", `"prompt":"test"`, ""},
			{"empty_seconds", `"Seconds":"","metadata":{"duration":12}`, ""},
			{"empty_metadata", `"duration":0,"metadata":""`, "0"},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				body := `{"model":"client-alias","prompt":"test",` + tc.fields + `}`
				if format == "json_object" {
					wire := yhVideoJSONBody(t, "/v1/videos", body)
					require.Equal(t, tc.want, gjson.GetBytes(wire, "metadata.duration").Raw)
					return
				}
				var fields map[string]any
				// Form fields are strings; retain their original metadata bytes so
				// the large integer exercises the raw metadata path too.
				require.NoError(t, common.Unmarshal([]byte(body), &fields))
				values := url.Values{}
				for key := range fields {
					values.Set(key, gjson.Get(body, key).String())
					if strings.EqualFold(key, "metadata") {
						fields[key] = values.Get(key)
					}
				}
				var data bytes.Buffer
				ct := "application/json"
				switch format {
				case "json_string":
					encoded, err := common.Marshal(fields)
					require.NoError(t, err)
					data.Write(encoded)
				case "form":
					ct = "application/x-www-form-urlencoded"
					data.WriteString(values.Encode())
				case "multipart":
					writer := multipart.NewWriter(&data)
					for key := range values {
						require.NoError(t, writer.WriteField(key, values.Get(key)))
					}
					require.NoError(t, writer.Close())
					ct = writer.FormDataContentType()
				}
				request := httptest.NewRequest(http.MethodPost, "/v1/videos", &data)
				request.Header.Set("Content-Type", ct)
				wire := yhVideoRequestBody(t, request)
				require.Equal(t, tc.want, gjson.GetBytes(wire, "metadata.duration").Raw)
				if tc.name == "mixed_seconds" {
					require.Equal(t, "9007199254740993", gjson.GetBytes(wire, "metadata.seed").Raw)
				}
			})
		}
	}
	for _, fields := range []string{
		`"Duration":null,"metadata":{"duration":12}`,
		`"Seconds":null,"metadata":{"duration":12}`,
		`"duration":5,"DURATION":null,"metadata":{"duration":12}`,
	} {
		wire := yhVideoJSONBody(t, "/v1/videos", `{"model":"client-alias","prompt":"test",`+fields+`}`)
		require.False(t, gjson.GetBytes(wire, "metadata.duration").Exists(), string(wire))
	}
	for _, fields := range []string{
		`"duration":5,"DURATION":0,"metadata":{"duration":12}`,
		`"Duration":0,"Seconds":null,"metadata":{"duration":12}`,
	} {
		wire := yhVideoJSONBody(t, "/v1/videos", `{"model":"client-alias","prompt":"test",`+fields+`}`)
		require.Equal(t, "0", gjson.GetBytes(wire, "metadata.duration").Raw)
	}
}
