package helper

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageRequestResponseModes(t *testing.T) {
	for _, async := range []string{"", "false", "true", "TRUE"} {
		for _, stream := range []string{"", "false", "true"} {
			for _, form := range []bool{false, true} {
				name := "json"
				if form {
					name = "multipart"
				}
				t.Run(name+"/async="+async+"/stream="+stream, func(t *testing.T) {
					var body bytes.Buffer
					mode, path, contentType := relayconstant.RelayModeImagesGenerations, "/v1/images/generations", "application/json"
					if form {
						mode, path = relayconstant.RelayModeImagesEdits, "/v1/images/edits"
						writer := multipart.NewWriter(&body)
						require.NoError(t, writer.WriteField("model", "gpt-image-2"))
						require.NoError(t, writer.WriteField("prompt", "cat"))
						if stream != "" {
							require.NoError(t, writer.WriteField("stream", stream))
						}
						require.NoError(t, writer.Close())
						contentType = writer.FormDataContentType()
					} else {
						body.WriteString(`{"model":"gpt-image-2","prompt":"cat"`)
						if stream != "" {
							body.WriteString(`,"stream":` + stream)
						}
						body.WriteString("}")
					}
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, path+"?async="+async, &body)
					c.Request.Header.Set("Content-Type", contentType)
					t.Cleanup(func() { common.CleanupBodyStorage(c) })
					request, err := GetAndValidOpenAIImageRequest(c, mode)
					require.NoError(t, err)
					require.Equal(t, async, c.Query("async"))
					if stream == "" {
						require.Nil(t, request.Stream)
					} else {
						require.NotNil(t, request.Stream)
						require.Equal(t, stream == "true", *request.Stream)
					}
				})
			}
		}
	}
}
