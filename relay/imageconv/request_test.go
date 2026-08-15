package imageconv

import (
	"bytes"
	"encoding/json"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBuildOpenAIJSONEditMultipartPreservesRequestFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	n := uint(0)
	watermark := false
	request := dto.ImageRequest{
		Model:             "gpt-image-2",
		Prompt:            "edit the image",
		N:                 &n,
		Size:              "1024x1024",
		Quality:           "high",
		ResponseFormat:    "url",
		Style:             json.RawMessage(`"natural"`),
		User:              json.RawMessage(`"user-1"`),
		Background:        json.RawMessage(`"transparent"`),
		Moderation:        json.RawMessage(`"low"`),
		OutputFormat:      json.RawMessage(`"png"`),
		OutputCompression: json.RawMessage(`75`),
		PartialImages:     json.RawMessage(`2`),
		InputFidelity:     json.RawMessage(`"high"`),
		Watermark:         &watermark,
		Image:             json.RawMessage(`"data:image/png;base64,ZmFrZQ=="`),
		Extra: map[string]json.RawMessage{
			"vendor_option": json.RawMessage(`{"mode":"fast"}`),
		},
	}

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	body, err := BuildOpenAIJSONEditMultipart(c, request)
	require.NoError(t, err)

	_, params, err := mime.ParseMediaType(c.Request.Header.Get("Content-Type"))
	require.NoError(t, err)
	form, err := multipart.NewReader(bytes.NewReader(body.Bytes()), params["boundary"]).ReadForm(1024 * 1024)
	require.NoError(t, err)
	defer form.RemoveAll()

	require.Equal(t, []string{"0"}, form.Value["n"])
	require.Equal(t, []string{"transparent"}, form.Value["background"])
	require.Equal(t, []string{"low"}, form.Value["moderation"])
	require.Equal(t, []string{"png"}, form.Value["output_format"])
	require.Equal(t, []string{"75"}, form.Value["output_compression"])
	require.Equal(t, []string{"2"}, form.Value["partial_images"])
	require.Equal(t, []string{"natural"}, form.Value["style"])
	require.Equal(t, []string{"user-1"}, form.Value["user"])
	require.Equal(t, []string{"false"}, form.Value["watermark"])
	require.JSONEq(t, `{"mode":"fast"}`, form.Value["vendor_option"][0])
	require.NotContains(t, form.Value, "image")
	require.Len(t, form.File["image"], 1)

	file, err := form.File["image"][0].Open()
	require.NoError(t, err)
	defer file.Close()
	var image bytes.Buffer
	_, err = image.ReadFrom(file)
	require.NoError(t, err)
	require.Equal(t, []byte("fake"), image.Bytes())

	var vendorOption map[string]string
	require.NoError(t, common.Unmarshal([]byte(form.Value["vendor_option"][0]), &vendorOption))
	require.Equal(t, "fast", vendorOption["mode"])
}

func TestImageReferenceCacheKeepsFailures(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	EnsureImageReferenceCache(c)

	_, _, _, firstErr := imageReferenceBytes(c, "not-valid-base64", 1)
	require.Error(t, firstErr)
	_, _, _, secondErr := imageReferenceBytes(c, "not-valid-base64", 1)
	require.Error(t, secondErr)
	require.Same(t, firstErr, secondErr)

	cache := imageReferenceCacheFromContext(c)
	require.Len(t, cache.entries, 1)
	require.Same(t, firstErr, cache.entries["not-valid-base64"].err)

	ResetImageReferenceFailures(c)
	require.Empty(t, cache.entries)
	_, _, _, retryErr := imageReferenceBytes(c, "not-valid-base64", 1)
	require.Error(t, retryErr)
	require.NotSame(t, firstErr, retryErr)
}
