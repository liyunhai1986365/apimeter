package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryRespectsSelectedChannelRetrySwitch(t *testing.T) {
	originalRetryTimes := common.RetryTimes
	common.RetryTimes = 1
	t.Cleanup(func() {
		common.RetryTimes = originalRetryTimes
	})

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, false)

	err := types.NewOpenAIError(
		errors.New("upstream overloaded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	require.False(t, shouldRetry(c, err, 1))
}
