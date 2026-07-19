package middleware

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenMosaicEmbeddedAuthorizeRateLimitIsScopedPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	inMemoryRateLimiter = common.InMemoryRateLimiter{}
	t.Cleanup(func() {
		common.RedisEnabled = previousRedisEnabled
		inMemoryRateLimiter = common.InMemoryRateLimiter{}
	})

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
		require.Equal(t, http.StatusNoContent, request(7))
	}
	require.Equal(t, http.StatusTooManyRequests, request(7))
	require.Equal(t, http.StatusNoContent, request(8))
}
