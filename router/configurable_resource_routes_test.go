package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/controller"
	"github.com/gin-gonic/gin"
)

func TestConfigurableResourceRoutesAreRegisteredFromProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetVideoRouter(r)
	r.NoRoute(controller.RelayNotFound)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/material/assets", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable material create route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/material/assets/asset_123", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable material detail route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/assets/upload", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable assets upload alias route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/assets", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable assets create alias route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/assets", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable assets list alias route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/assets/asset_123", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable assets detail alias route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/assets/asset_123", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected configurable assets delete alias route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/asset-groups", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected service inference asset group create route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/asset-groups/group_123", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected service inference asset group detail route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/assets", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected service inference asset create route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	recorder = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/v1/assets/get", nil)
	r.ServeHTTP(recorder, req)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected service inference asset get route to be registered, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestConfigurableResourceRoutesAllowSharedPublicPathsAcrossProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("expected shared configurable resource routes to register once, got panic: %v", recovered)
		}
	}()
	SetVideoRouter(r)
}

func TestConfigurableResourceRoutesAreRegisteredWithFullAPIRouterOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetApiRouter(r)
	SetDashboardRouter(r)
	SetRelayRouter(r)
	SetVideoRouter(r)

	assertRouteRegistered := func(method string, path string) {
		t.Helper()
		for _, route := range r.Routes() {
			if route.Method == method && route.Path == path {
				return
			}
		}
		t.Fatalf("expected route %s %s to be registered in full router order", method, path)
	}

	assertRouteRegistered(http.MethodPost, "/api/assets/upload")
	assertRouteRegistered(http.MethodPost, "/api/volcengine_asset")
	assertRouteRegistered(http.MethodPost, "/api/assets")
	assertRouteRegistered(http.MethodGet, "/api/assets")
	assertRouteRegistered(http.MethodGet, "/api/assets/:id")
	assertRouteRegistered(http.MethodPost, "/material/assets")
	assertRouteRegistered(http.MethodPost, "/v2/db-sd-max/assets")
	assertRouteRegistered(http.MethodGet, "/v2/db-sd-max/assets/:id")
	assertRouteRegistered(http.MethodPost, "/v2/sd-max/assets")
	assertRouteRegistered(http.MethodGet, "/v2/sd-max/assets/:id")
}

func TestKlingConfigurableResourceRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetVideoRouter(r)

	assertRouteRegistered := func(method string, path string) {
		t.Helper()
		for _, route := range r.Routes() {
			if route.Method == method && route.Path == path {
				return
			}
		}
		t.Fatalf("expected route %s %s to be registered", method, path)
	}

	assertRouteRegistered(http.MethodPost, "/kling/text-to-video/kling-3.0-turbo")
	assertRouteRegistered(http.MethodPost, "/kling/image-to-video/kling-3.0-turbo")
	assertRouteRegistered(http.MethodPost, "/text-to-video/kling-3.0-turbo")
	assertRouteRegistered(http.MethodPost, "/image-to-video/kling-3.0-turbo")
	assertRouteRegistered(http.MethodGet, "/kling/tasks")
	assertRouteRegistered(http.MethodPost, "/kling/tasks")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/text2video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/text2video/:id")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/text2video")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/image2video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/image2video/:id")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/image2video")
	assertRouteRegistered(http.MethodPost, "/kling/v2/videos/text2video")
	assertRouteRegistered(http.MethodGet, "/kling/v2/videos/text2video/:task_id")
	assertRouteRegistered(http.MethodPost, "/kling/v2/videos/image2video")
	assertRouteRegistered(http.MethodGet, "/kling/v2/videos/image2video/:task_id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/omni-video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/omni-video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/omni-video/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/motion-control")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/motion-control")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/motion-control/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-image2video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/multi-image2video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/multi-image2video/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-elements/init-selection")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-elements/add-selection")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-elements/delete-selection")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-elements/clear-selection")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-elements/preview-selection")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/multi-elements")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/multi-elements")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/multi-elements/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/video-extend")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/video-extend")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/video-extend/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/identify-face")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/advanced-lip-sync")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/advanced-lip-sync")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/advanced-lip-sync/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/avatar/image2video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/avatar/image2video")
	assertRouteRegistered(http.MethodGet, "/kling/v1/videos/avatar/image2video/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/audio/tts")
	assertRouteRegistered(http.MethodPost, "/kling/v1/audio/text-to-audio")
	assertRouteRegistered(http.MethodGet, "/kling/v1/audio/text-to-audio")
	assertRouteRegistered(http.MethodGet, "/kling/v1/audio/text-to-audio/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/audio/video-to-audio")
	assertRouteRegistered(http.MethodGet, "/kling/v1/audio/video-to-audio")
	assertRouteRegistered(http.MethodGet, "/kling/v1/audio/video-to-audio/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/general/advanced-custom-elements")
	assertRouteRegistered(http.MethodGet, "/kling/v1/general/advanced-custom-elements/:id")
	assertRouteRegistered(http.MethodPost, "/kling/v1/general/advanced-presets-elements")
	assertRouteRegistered(http.MethodPost, "/kling/v1/general/delete-advanced-elements")
	assertRouteRegistered(http.MethodPost, "/kling/v1/general/custom-voices")
	assertRouteRegistered(http.MethodGet, "/kling/v1/general/custom-voices/:id")
	assertRouteRegistered(http.MethodGet, "/kling/v1/general/custom-voices")
	assertRouteRegistered(http.MethodGet, "/kling/v1/general/presets-voices")
	assertRouteRegistered(http.MethodPost, "/kling/v1/general/delete-voices")
	assertRouteRegistered(http.MethodPost, "/kling/v1/videos/image-recognize")
}

func TestConfigurableResourceCommonAssetsUploadDoesNotFallThroughInFullRouterOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetApiRouter(r)
	SetDashboardRouter(r)
	SetRelayRouter(r)
	SetVideoRouter(r)
	r.NoRoute(controller.RelayNotFound)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/assets/upload", nil)
	r.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusNotFound {
		t.Fatalf("expected /api/assets/upload to hit configurable asset route, got status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "欢迎使用") {
		t.Fatalf("expected /api/assets/upload not to fall through to welcome/status response, got body=%s", recorder.Body.String())
	}
}

func TestTgxMaasResourceRoutesRegistered(t *testing.T) {
	r := gin.New()
	SetVideoRouter(r)
	routes := map[string]bool{}
	for _, route := range r.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, endpoint := range []string{
		"POST /v1/private-avatar/groups", "POST /v1/private-avatar/groups/list",
		"GET /v1/private-avatar/groups/:group_id", "PATCH /v1/private-avatar/groups/:group_id", "DELETE /v1/private-avatar/groups/:group_id",
		"POST /v1/private-avatar/assets", "POST /v1/private-avatar/assets/list",
		"GET /v1/private-avatar/assets/:asset_id", "PATCH /v1/private-avatar/assets/:asset_id", "DELETE /v1/private-avatar/assets/:asset_id",
		"POST /v1/real-avatar/auth/session", "POST /v1/real-avatar/groups/from-token",
	} {
		if !routes[endpoint] {
			t.Errorf("missing TgxMaas route: %s", endpoint)
		}
	}
}

func TestTgxMaasCompatibilityRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	SetApiRouter(r)
	SetDashboardRouter(r)
	SetRelayRouter(r)
	SetVideoRouter(r)
	for _, tc := range []struct{ method, path string }{
		{"POST", "/?Action=CreateAsset&Version=2024-01-01"},
		{"POST", "/api/assets"}, {"POST", "/api/assets/upload"}, {"GET", "/api/assets"},
		{"GET", "/api/assets/asset_local"}, {"PATCH", "/api/assets/asset_local"}, {"DELETE", "/api/assets/asset_local"},
		{"POST", "/api/asset-groups"}, {"GET", "/api/asset-groups"},
		{"GET", "/api/asset-groups/ag_local"}, {"PATCH", "/api/asset-groups/ag_local"}, {"DELETE", "/api/asset-groups/ag_local"},
		{"POST", "/v1/asset-groups"}, {"GET", "/v1/asset-groups"},
		{"GET", "/v1/asset-groups/ag_local"}, {"PATCH", "/v1/asset-groups/ag_local"}, {"DELETE", "/v1/asset-groups/ag_local"},
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`)))
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
