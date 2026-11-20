package middleware

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

var streamRateLimitTestUsers atomic.Int64

func TestModelRateLimitDoesNotCountFailedHTTP200Streams(t *testing.T) {
	_, redisClient := useRateLimitMiniRedis(t)
	for _, useRedis := range []bool{false, true} {
		for _, failed := range []bool{false, true} {
			t.Run(fmt.Sprintf("redis=%t/failed=%t", useRedis, failed), func(t *testing.T) {
				userID := 610000 + int(streamRateLimitTestUsers.Add(1))
				router := gin.New()
				router.Use(func(c *gin.Context) { c.Set("id", userID) })
				if useRedis {
					router.Use(redisRateLimitHandler(60, 0, 1))
				} else {
					router.Use(memoryRateLimitHandler(60, 0, 1))
				}
				router.GET("/stream", func(c *gin.Context) {
					helper.SetEventStreamHeaders(c)
					require.NoError(t, helper.PingData(c))
					common.SetContextKey(c, constant.ContextKeyRelayFailed, failed)
				})
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/stream", nil))
				require.Equal(t, http.StatusOK, recorder.Code)
				if useRedis {
					count, err := redisClient.LLen(context.Background(), fmt.Sprintf("rateLimit:%s:%d", ModelRequestRateLimitSuccessCountMark, userID)).Result()
					require.NoError(t, err)
					want := int64(1)
					if failed {
						want = 0
					}
					require.Equal(t, want, count)
				} else {
					allowed := inMemoryRateLimiter.Request(ModelRequestRateLimitSuccessCountMark+strconv.Itoa(userID), 1, 60)
					require.Equal(t, failed, allowed, "only a successful stream should occupy the success counter")
				}
			})
		}
	}
}
