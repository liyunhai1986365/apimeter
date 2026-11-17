package openai

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertImageEditRequestMultipart(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files []string
	}{
		{name: "image only", files: []string{"image"}},
		{name: "mask", files: []string{"image", "mask"}},
		{name: "markup", files: []string{"image", "markup"}},
		{name: "mask and markup", files: []string{"image", "mask", "markup"}},
		{name: "first optional file", files: []string{"image", "mask", "mask", "markup", "markup"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var source bytes.Buffer
			writer := multipart.NewWriter(&source)
			fields := map[string]string{
				"model":             "image-alias",
				"prompt":            "edit this image",
				"complexity":        "low",
				"frame_count":       "2",
				"frame_duration_ms": "0",
				"stream":            "true",
				"partial_images":    "3",
			}
			for name, value := range fields {
				require.NoError(t, writer.WriteField(name, value))
			}
			seen := make(map[string]bool)
			for _, name := range tc.files {
				filename, content := name+".png", "fake "+name
				if seen[name] {
					filename, content = "ignored.png", "ignored duplicate"
				}
				seen[name] = true
				part, err := writer.CreateFormFile(name, filename)
				require.NoError(t, err)
				_, err = io.WriteString(part, content)
				require.NoError(t, err)
			}
			require.NoError(t, writer.Close())

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(source.Bytes()))
			c.Request.Header.Set("Content-Type", writer.FormDataContentType())
			t.Cleanup(func() {
				if c.Request.MultipartForm != nil {
					require.NoError(t, c.Request.MultipartForm.RemoveAll())
				}
			})
			info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits}
			converted, err := (&Adaptor{}).ConvertImageRequest(c, info, dto.ImageRequest{
				Model:  "gpt-image-2.6-flare",
				Prompt: fields["prompt"],
			})
			require.NoError(t, err)
			body, ok := converted.(*bytes.Buffer)
			require.True(t, ok)
			form, err := multipart.NewReader(body, multipartBoundary(t, c.GetHeader("Content-Type"))).ReadForm(1 << 20)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, form.RemoveAll()) })

			fields["model"] = "gpt-image-2.6-flare"
			for name, value := range fields {
				require.Equal(t, []string{value}, form.Value[name], "field %s", name)
			}
			require.Len(t, form.File, len(seen))
			for name := range seen {
				require.Len(t, form.File[name], 1, "file %s", name)
				header := form.File[name][0]
				require.Equal(t, name+".png", header.Filename)
				require.Equal(t, "image/png", header.Header.Get("Content-Type"))
				file, err := header.Open()
				require.NoError(t, err)
				content, err := io.ReadAll(file)
				closeErr := file.Close()
				require.NoError(t, err)
				require.NoError(t, closeErr)
				require.Equal(t, "fake "+name, string(content))
			}
		})
	}
}
