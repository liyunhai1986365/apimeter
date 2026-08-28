package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRespondTaskErrorPreservesNativeProviderPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	raw := []byte(`{"request_id":"req-1","code":"InvalidParameter","message":"bad ratio","details":{"field":"ratio"}}`)

	respondTaskError(c, &taskdto.TaskError{
		StatusCode: http.StatusBadRequest,
		RawBody:    raw,
	})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.JSONEq(t, string(raw), recorder.Body.String())
}
