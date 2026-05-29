package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
)

func TestConfigurableNativeRoutesAreRegisteredFromProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetVideoRouter(r)
	r.NoRoute(controller.RelayNotFound)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/services/aigc/video-generation/video-synthesis", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable native submit route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/task_123", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable native fetch route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
