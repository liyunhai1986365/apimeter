package controller

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHDraftStringMetadataKeepsOriginalChannel(t *testing.T) {
	var creates atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/v1/video/generations", r.URL.Path)
		require.Equal(t, "Bearer yh-test-key", r.Header.Get("Authorization"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, "cgt-draft-metadata", gjson.GetBytes(body, "metadata.content.0.draft_task.id").String())
		for _, field := range []string{"prompt", "metadata.duration", "metadata.ratio", "metadata.generate_audio"} {
			require.False(t, gjson.GetBytes(body, field).Exists(), field)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"task_id":"cgt-metadata-%d","status":"SUBMITTED"}`, creates.Add(1))
	}))
	defer upstream.Close()
	r := yhAssetTestRouter(t, upstream.URL)
	r.POST("/v1/videos", middleware.TokenAuth(), middleware.Distribute(), RelayTask)
	// A higher-priority candidate must not divert an owned draft to another
	// provider when its reference is inside JSON-encoded metadata.
	alternateURL := "http://127.0.0.1:1"
	alternative := &model.Channel{Id: 21, Type: constant.ChannelTypeConfigurable, Status: common.ChannelStatusEnabled, Key: "different-key", BaseURL: &alternateURL, Models: tgxRegressionModel, Group: "default"}
	alternative.SetSetting(relaydto.ChannelSettings{Protocol: &relaydto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
	require.NoError(t, model.DB.Create(alternative).Error)
	createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
	require.NoError(t, model.DB.Create(&model.Task{
		TaskID: "task-draft-metadata", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess,
		Properties:  model.Properties{OriginModelName: tgxRegressionModel},
		PrivateData: model.TaskPrivateData{SeedanceDraft: true, Key: "yh-test-key", UpstreamTaskID: "cgt-draft-metadata"},
	}).Error)
	metadata := `{"content":[{"type":"draft_task","draft_task":{"id":"task-draft-metadata"}}]}`
	for _, format := range []string{"json_string", "multipart"} {
		t.Run(format, func(t *testing.T) {
			var body bytes.Buffer
			contentType := "application/json"
			if format == "multipart" {
				writer := multipart.NewWriter(&body)
				require.NoError(t, writer.WriteField("model", tgxRegressionModel))
				require.NoError(t, writer.WriteField("metadata", metadata))
				require.NoError(t, writer.Close())
				contentType = writer.FormDataContentType()
			} else {
				encoded, err := common.Marshal(map[string]string{"model": tgxRegressionModel, "metadata": metadata})
				require.NoError(t, err)
				body.Write(encoded)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
			request.Header.Set("Authorization", "Bearer seedancetest1")
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		})
	}
	require.EqualValues(t, 2, creates.Load())
}
