package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
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

func TestShouldRetryChannelPolicyOverridesGlobalPolicy(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:        "global retry 500",
			Action:      operation_setting.RetryPolicyActionRetry,
			StatusCodes: "500",
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, 29)
	common.SetContextKey(c, constant.ContextKeyChannelType, 1)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		RetryPolicyRules: []dto.RetryPolicyRule{
			{
				Name:   "channel skips private ip",
				Action: operation_setting.RetryPolicyActionSkipRetry,
				MessageContains: []string{
					"private ip",
				},
			},
		},
	})
	c.Set("original_model", "gpt-image-2")

	err := types.NewOpenAIError(
		errors.New("download image failed: private ip rejected"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	require.False(t, shouldRetry(c, err, 1))
}

func TestShouldRetryGlobalPolicyOverridesStatusCodeFallback(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:   "global retry private ip",
			Action: operation_setting.RetryPolicyActionRetry,
			Models: []string{
				"gpt-image-2",
			},
			MessageContains: []string{
				"private ip",
			},
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c.Set("original_model", "gpt-image-2")

	err := types.NewOpenAIError(
		errors.New("download image failed: private ip rejected"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	require.True(t, shouldRetry(c, err, 1))
}
