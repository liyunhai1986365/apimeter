package relay

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestImageHelperRetriesWithOriginalRequestFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	for _, tt := range []struct {
		name             string
		model            string
		generations      bool
		multipart        bool
		retryPassthrough bool
		sameChannel      bool
	}{
		{name: "JSON edits same channel", model: "gpt-image-2", sameChannel: true},
		{name: "JSON edits cross channel", model: "gpt-image-2.5-flare"},
		{name: "JSON generations converted to edits", model: "gpt-image-2.5-sunburst", generations: true},
		{name: "JSON generations fallback to passthrough", model: "gpt-image-2", generations: true, retryPassthrough: true},
		{name: "JSON edits fallback to passthrough", model: "gpt-image-2.5-sunburst", retryPassthrough: true},
		{name: "multipart edits cross channel", model: "gpt-image-2", multipart: true},
		{name: "multipart edits fallback to passthrough", model: "gpt-image-2.5-flare", multipart: true, retryPassthrough: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			request := &dto.ImageRequest{Model: tt.model, Prompt: "edit the image", N: common.GetPointer(uint(1))}
			var body []byte
			contentType := "application/json; charset=utf-8"
			if tt.multipart {
				var source bytes.Buffer
				writer := multipart.NewWriter(&source)
				require.NoError(t, writer.WriteField("model", request.Model))
				require.NoError(t, writer.WriteField("prompt", request.Prompt))
				file, err := writer.CreateFormFile("image", "input.png")
				require.NoError(t, err)
				_, err = file.Write([]byte("fake image"))
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				body = source.Bytes()
				contentType = writer.FormDataContentType()
			} else {
				request.Images = []byte(`["data:image/png;base64,ZmFrZSBpbWFnZQ=="]`)
				var err error
				body, err = common.Marshal(request)
				require.NoError(t, err)
			}

			type receivedRequest struct {
				path        string
				contentType string
				body        []byte
			}
			received := make(chan receivedRequest, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				payload, _ := io.ReadAll(r.Body)
				received <- receivedRequest{r.URL.String(), r.Header.Get("Content-Type"), payload}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"message":"upstream busy","type":"server_error","code":"service_unavailable"}}`))
			}))
			defer upstream.Close()

			path := "/v1/images/edits?client=retry-test"
			mode := relayconstant.RelayModeImagesEdits
			if tt.generations {
				path = "/v1/images/generations?client=retry-test"
				mode = relayconstant.RelayModeImagesGenerations
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", contentType)
			originalHeaders := c.Request.Header.Clone()
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			common.SetContextKey(c, constant.ContextKeyChannelKey, "test-upstream-key")
			common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
			defer common.CleanupBodyStorage(c)
			defer func() {
				if c.Request.MultipartForm != nil {
					_ = c.Request.MultipartForm.RemoveAll()
				}
			}()
			info := &relaycommon.RelayInfo{Request: request, OriginModelName: tt.model, RelayMode: mode, RequestURLPath: path}

			for attempt := 0; attempt < 2; attempt++ {
				passthrough := attempt == 1 && tt.retryPassthrough
				convert := !passthrough
				common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
					PassThroughBodyEnabled:                    passthrough,
					ImageAutoConvertGenerationWithImageToEdit: &convert,
					ImageAutoConvertJSONEditToMultipart:       &convert,
				})
				channelID := 11
				if attempt == 1 && !tt.sameChannel {
					channelID = 12
				}
				common.SetContextKey(c, constant.ContextKeyChannelId, channelID)
				// Replay the original body exactly as the controller's retry loop does.
				storage, err := common.GetBodyStorage(c)
				require.NoError(t, err)
				c.Request.Body = io.NopCloser(storage)
				apiErr := ImageHelper(c, info)
				require.NotNil(t, apiErr)
				require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
				require.Equal(t, "service_unavailable", string(apiErr.GetErrorCode()))
				require.Len(t, received, 1, "attempt must reach the upstream")
				got := <-received
				expectedPath := "/v1/images/edits?client=retry-test"
				if passthrough {
					expectedPath = path
					require.Equal(t, body, got.body)
					require.Equal(t, contentType, got.contentType)
				} else {
					mediaType, params, err := mime.ParseMediaType(got.contentType)
					require.NoError(t, err)
					require.Equal(t, "multipart/form-data", mediaType)
					form, err := multipart.NewReader(bytes.NewReader(got.body), params["boundary"]).ReadForm(1 << 20)
					require.NoError(t, err)
					require.Equal(t, []string{tt.model}, form.Value["model"])
					require.Equal(t, []string{request.Prompt}, form.Value["prompt"])
					require.Len(t, form.File["image"], 1)
					file, err := form.File["image"][0].Open()
					require.NoError(t, err)
					image, err := io.ReadAll(file)
					require.NoError(t, err)
					require.NoError(t, file.Close())
					require.Equal(t, []byte("fake image"), image)
					require.NoError(t, form.RemoveAll())
				}
				require.Equal(t, expectedPath, got.path)
				require.Equal(t, originalHeaders, c.Request.Header)
				require.Equal(t, mode, info.RelayMode)
				require.Equal(t, path, info.RequestURLPath)
			}
		})
	}
}
