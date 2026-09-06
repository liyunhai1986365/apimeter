package common

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestTaskDurationBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newContext := func(body string) (*gin.Context, *RelayInfo) {
		request := httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		context, _ := gin.CreateTestContext(httptest.NewRecorder())
		context.Request = request
		return context, &RelayInfo{TaskRelayInfo: &TaskRelayInfo{}}
	}

	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "huge duration is rejected", body: `{"model":"sora-2","prompt":"a cat","duration":9999999999}`, wantErr: true},
		{name: "huge seconds string is rejected", body: `{"model":"sora-2","prompt":"a cat","seconds":"9999999999"}`, wantErr: true},
		{name: "negative duration is rejected", body: `{"model":"sora-2","prompt":"a cat","duration":-8}`, wantErr: true},
		{name: "normal duration is accepted", body: `{"model":"sora-2","prompt":"a cat","seconds":"8"}`},
		{name: "seedance duration above old limit is accepted", body: `{"model":"dreamina-seedance-2-0-260128","prompt":"a cat","duration":26}`},
		{name: "seedance duration at limit is accepted", body: `{"model":"dreamina-seedance-2-0-260128","prompt":"a cat","duration":30}`},
		{name: "seedance seconds at limit is accepted", body: `{"model":"doubao-seedance-2.0-pro","prompt":"a cat","seconds":"30"}`},
		{name: "seedance duration over limit is rejected", body: `{"model":"dreamina-seedance-2-0-260128","prompt":"a cat","duration":31}`, wantErr: true},
		{name: "seedance seconds over limit is rejected", body: `{"model":"doubao-seedance-2.0-pro","prompt":"a cat","seconds":"31"}`, wantErr: true},
		{name: "non seedance duration over 30 is accepted", body: `{"model":"other-video-model","prompt":"a cat","duration":31}`},
	}

	for _, tt := range tests {
		t.Run(tt.name+" multipart direct", func(t *testing.T) {
			context, info := newContext(tt.body)
			taskErr := ValidateMultipartDirect(context, info)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})

		t.Run(tt.name+" basic task request", func(t *testing.T) {
			context, info := newContext(tt.body)
			taskErr := ValidateBasicTaskRequest(context, info, constant.TaskActionGenerate)
			if tt.wantErr {
				require.NotNil(t, taskErr)
				require.Equal(t, "invalid_seconds", taskErr.Code)
				return
			}
			require.Nil(t, taskErr)
		})
	}
}

func TestTaskDurationBoundsUsesMappedSeedanceModel(t *testing.T) {
	req := TaskSubmitReq{
		Model:    "video-model-alias",
		Prompt:   "a cat",
		Duration: 30,
	}

	require.Nil(t, ValidateTaskDurationBounds(req, "dreamina-seedance-2-0-260128"))
	req.Duration = 31
	taskErr := ValidateTaskDurationBounds(req, "dreamina-seedance-2-0-260128")

	require.NotNil(t, taskErr)
	require.Equal(t, "invalid_seconds", taskErr.Code)
	require.Equal(t, "seconds must be between 1 and 30", taskErr.Message)
}
