package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var openMosaicRateLimitTestUsers atomic.Int64

func TestOpenMosaicEmbeddedAuthorizeRateLimitIsScopedPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
	})
	// Keep the limiter used by its cleanup goroutine alive. Unique users isolate
	// repeated test runs without replacing its store, mutex or expiration state.
	userID := 620000 + int(openMosaicRateLimitTestUsers.Add(2))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID, _ := strconv.Atoi(c.GetHeader("X-Test-User-ID"))
		c.Set("id", userID)
	})
	router.POST("/authorize", OpenMosaicEmbeddedAuthorizeRateLimit(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	request := func(userID int) int {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/authorize", nil)
		req.Header.Set("X-Test-User-ID", strconv.Itoa(userID))
		router.ServeHTTP(recorder, req)
		return recorder.Code
	}

	for range openMosaicEmbeddedAuthorizeRateLimitNum {
		require.Equal(t, http.StatusNoContent, request(userID))
	}
	require.Equal(t, http.StatusTooManyRequests, request(userID))
	require.Equal(t, http.StatusNoContent, request(userID+1))
}
