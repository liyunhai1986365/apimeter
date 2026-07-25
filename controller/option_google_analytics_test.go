package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestUpdateOptionRejectsInvalidGoogleAnalyticsMeasurementID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/api/option/", UpdateOption)
	req := httptest.NewRequest(
		http.MethodPut,
		"/api/option/",
		strings.NewReader(`{"key":"GoogleAnalyticsId","value":"<script>alert(1)</script>"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]interface{}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.Equal(t, false, body["success"])
	require.Contains(t, body["message"], "G-6B94BX72EW")
}

func TestGoogleAnalyticsMeasurementIDPattern(t *testing.T) {
	require.True(t, googleAnalyticsMeasurementIDPattern.MatchString("G-6B94BX72EW"))
	require.True(t, googleAnalyticsMeasurementIDPattern.MatchString("G-ABC123"))
	require.False(t, googleAnalyticsMeasurementIDPattern.MatchString("UA-12345-1"))
	require.False(t, googleAnalyticsMeasurementIDPattern.MatchString("G-ABC 123"))
}
