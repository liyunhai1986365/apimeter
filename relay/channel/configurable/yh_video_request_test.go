package configurable

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func yhVideoRequestBody(t *testing.T, request *http.Request) []byte {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = request
	defer common.CleanupBodyStorage(c)
	// Distribution buffers the body before multipart validation consumes it.
	var routingFields map[string]any
	require.NoError(t, common.UnmarshalBodyReusable(c, &routingFields))
	info := seedanceMaxRelayInfo("seedance-yh")
	a := &TaskAdaptor{}
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	reader, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	wire, err := io.ReadAll(reader)
	require.NoError(t, err)
	return wire
}

func yhVideoJSONBody(t *testing.T, path, body string) []byte {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	return yhVideoRequestBody(t, request)
}

func TestYHEmptyContentKeepsShorthand(t *testing.T) {
	for _, empty := range []string{"[]", "null"} {
		t.Run(empty, func(t *testing.T) {
			body := `{"model":"client-alias","prompt":"animate this image","images":["https://example.com/image.jpg"],"metadata":{"content":` + empty + `}}`
			wire := yhVideoJSONBody(t, "/v1/videos", body)
			require.Equal(t, "animate this image", gjson.GetBytes(wire, "prompt").String())
			require.Equal(t, "animate this image", gjson.GetBytes(wire, "metadata.content.0.text").String())
			require.Equal(t, "https://example.com/image.jpg", gjson.GetBytes(wire, "metadata.content.1.image_url.url").String())
			require.Len(t, gjson.GetBytes(wire, "metadata.content").Array(), 2)
		})
	}
}

func TestYHGenericFieldPrecedence(t *testing.T) {
	for _, tc := range []struct{ name, fields, metadata, path, want string }{
		{"omni", `"omni_reference_task_type":"auto",`, `"omni_reference_task_type":"bogus"`, "omni_reference_task_type", `"auto"`},
		{"resolution", `"size":"720p",`, `"resolution":"1080p"`, "resolution", `"720p"`},
		{"empty_size", `"size":"",`, `"resolution":"1080p"`, "resolution", `"1080p"`},
		{"ratio", "", `"ratio":"16:9","aspect_ratio":"adaptive"`, "ratio", `"16:9"`},
		{"duration", `"duration":5,`, `"duration":10`, "duration", "5"},
		{"seconds", `"seconds":"5",`, `"duration":10`, "duration", "5"},
		{"metadata_duration", "", `"duration":5`, "duration", "5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"client-alias","prompt":"a scene",` + tc.fields + `"metadata":{` + tc.metadata + `}}`
			wire := yhVideoJSONBody(t, "/v1/videos", body)
			require.Equal(t, tc.want, gjson.GetBytes(wire, "metadata."+tc.path).Raw)
		})
	}
}

func TestYHOmniRatioFallback(t *testing.T) {
	for _, mode := range []struct{ name, duration string }{
		{"edit", "-1"},
		{"extend", "5"},
	} {
		for _, ratio := range []struct{ name, field string }{
			{"absent", ""},
			{"empty", `"ratio":"",`},
			{"null", `"ratio":null,`},
		} {
			t.Run(mode.name+"/"+ratio.name, func(t *testing.T) {
				body := `{"model":"client-alias","prompt":"adjust this clip","omni_reference_task_type":"` + mode.name + `","duration":` + mode.duration + `,"metadata":{` + ratio.field + `"aspect_ratio":"adaptive","video_url":"https://example.com/reference.mp4"}}`
				wire := yhVideoJSONBody(t, "/v1/videos", body)
				require.Equal(t, "adaptive", gjson.GetBytes(wire, "metadata.ratio").String())
			})
		}
	}
}

func TestYHMetadataFormatsPreservePrecision(t *testing.T) {
	metadata := `{"seed":9007199254740993,"return_last_frame":false,"generate_audio":false,"execution_expires_after":0,"extension":{"small":0.0000000000000000001},"content":[{"type":"text","text":"actual","extension":9007199254740993}]}`
	for _, format := range []string{"json_object", "json_string", "multipart", "form"} {
		t.Run(format, func(t *testing.T) {
			var body bytes.Buffer
			contentType := "application/json"
			switch format {
			case "json_object":
				body.WriteString(`{"model":"client-alias","prompt":"fallback","metadata":` + metadata + `}`)
			case "json_string":
				encoded, err := common.Marshal(metadata)
				require.NoError(t, err)
				body.WriteString(`{"model":"client-alias","prompt":"fallback","metadata":` + string(encoded) + `}`)
			case "multipart":
				writer := multipart.NewWriter(&body)
				require.NoError(t, writer.WriteField("model", "client-alias"))
				require.NoError(t, writer.WriteField("prompt", "fallback"))
				require.NoError(t, writer.WriteField("metadata", metadata))
				require.NoError(t, writer.Close())
				contentType = writer.FormDataContentType()
			case "form":
				body.WriteString(url.Values{"model": {"client-alias"}, "prompt": {"fallback"}, "metadata": {metadata}}.Encode())
				contentType = "application/x-www-form-urlencoded"
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
			request.Header.Set("Content-Type", contentType)
			wire := yhVideoRequestBody(t, request)
			for _, key := range []string{"seed", "return_last_frame", "generate_audio", "execution_expires_after", "extension", "content"} {
				require.Equal(t, gjson.Get(metadata, key).Raw, gjson.GetBytes(wire, "metadata."+key).Raw, key)
			}
			require.Equal(t, "actual", gjson.GetBytes(wire, "prompt").String())
		})
	}
}

func TestYHVideoSynchronizesProvidedText(t *testing.T) {
	media := `{"type":"image_url","role":"reference_image","image_url":{"url":"https://example.com/image.jpg","extra":9007199254740993}}`
	for _, mode := range []string{"generic", "native"} {
		for _, tc := range []struct{ name, content, prompt, wantText string }{
			{"prompt_with_media", "[" + media + "]", "animate this image", "animate this image"},
			{"content_text_wins", `[{"type":"text","text":"first"},` + media + `,{"type":"text","text":"second"}]`, "ignored", "first\nsecond"},
			{"media_without_text", "[" + media + "]", "", ""},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				prompt, err := common.Marshal(tc.prompt)
				require.NoError(t, err)
				path := "/v1/videos"
				body := `{"model":"client-alias","prompt":` + string(prompt) + `,"metadata":{"content":` + tc.content + `,"draft":true}}`
				if mode == "native" {
					path = "/api/v3/contents/generations/tasks"
					body = `{"model":"client-alias","prompt":` + string(prompt) + `,"content":` + tc.content + `,"draft":true}`
				}
				wire := yhVideoJSONBody(t, path, body)
				var texts []string
				for _, item := range gjson.GetBytes(wire, "metadata.content").Array() {
					if item.Get("type").String() == "text" {
						texts = append(texts, item.Get("text").String())
					} else {
						require.JSONEq(t, media, item.Raw)
					}
				}
				require.Equal(t, tc.wantText, strings.Join(texts, "\n"))
				require.Equal(t, tc.wantText, gjson.GetBytes(wire, "prompt").String())
				require.True(t, gjson.GetBytes(wire, "metadata.draft").Bool())
				if tc.wantText == "" {
					require.False(t, gjson.GetBytes(wire, "prompt").Exists(), "no placeholder should be added")
				}
				if tc.name != "prompt_with_media" {
					require.JSONEq(t, tc.content, gjson.GetBytes(wire, "metadata.content").Raw)
				}
			})
		}
	}
}
