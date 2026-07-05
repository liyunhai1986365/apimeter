package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
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

func TestShouldRetryPolicyMaxRetriesCapsRecoveryAttempts(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:        "codex encrypted recovery",
			Action:      operation_setting.RetryPolicyActionRetry,
			StatusCodes: "400",
			MessageContains: []string{
				"encrypted content",
			},
			RetryGroups: []string{"codex-primary"},
			MaxRetries:  1,
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c.Set("original_model", "gpt-5")

	err := types.NewOpenAIError(
		errors.New("status_code=400, The encrypted content gAAA could not be verified"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	require.True(t, shouldRetry(c, err, 2))
	c.Set("relay_retry_index", 1)
	require.False(t, shouldRetry(c, err, 1))
}

func TestShouldRetryChannelPolicyRecoveryOverridesAffinitySkip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{
		RetryPolicyRules: []dto.RetryPolicyRule{
			{
				Name:        "channel codex recovery",
				Action:      operation_setting.RetryPolicyActionRetry,
				StatusCodes: "400",
				MessageContains: []string{
					"encrypted content",
				},
				RetryGroups: []string{"codex-primary"},
				MaxRetries:  1,
			},
		},
	})
	c.Set("original_model", "gpt-5")
	c.Set("relay_retry_index", 0)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"prompt_cache_key":"codex-session"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	_, _ = service.GetPreferredChannelByAffinity(c, "gpt-5", "default")
	require.True(t, service.ShouldSkipRetryAfterChannelAffinityFailure(c))

	err := types.NewOpenAIError(
		errors.New("status_code=400, The encrypted content gAAA could not be verified"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadRequest,
	)

	require.True(t, shouldRetry(c, err, 1))

	recovery, ok := service.GetRetryPolicyRecovery(c)
	require.True(t, ok)
	require.Equal(t, operation_setting.RetryPolicySourceChannel, recovery.Source)
	require.Equal(t, []string{"codex-primary"}, recovery.RetryGroups)
	require.True(t, service.RetryPolicyRecoveryAllowsAffinityRetry(c))
}

func TestShouldRetryExistingRecoveryContextStillHonorsMaxRetries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c.Set("original_model", "gpt-5")
	service.SetRetryPolicyRecovery(c, operation_setting.RetryPolicyDecision{
		Matched:     true,
		ShouldRetry: true,
		Source:      operation_setting.RetryPolicySourceGlobal,
		RuleName:    "codex encrypted recovery",
		RetryGroups: []string{"codex-primary"},
		MaxRetries:  1,
	})
	c.Set("relay_retry_index", 1)

	err := types.NewOpenAIError(
		errors.New("upstream overloaded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	require.False(t, shouldRetry(c, err, 1))
}

func TestShouldRetryGlobalFailoverPolicySetsRecoveryTargets(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:        "global failover 500",
			Action:      operation_setting.RetryPolicyActionFailover,
			StatusCodes: "500",
			Targets: operation_setting.RetryPolicyTargets{
				Groups:      []string{"backup"},
				ChannelIDs:  []int{28},
				ChannelTags: []string{"stable"},
			},
			Strategy: operation_setting.RetryPolicyStrategy{
				MaxRetries:           2,
				ExcludeFailedChannel: true,
			},
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, 12)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c.Set("original_model", "gpt-5")
	c.Set("relay_retry_index", 0)

	err := types.NewOpenAIError(
		errors.New("upstream overloaded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	require.True(t, shouldRetry(c, err, 1))
	recovery, ok := service.GetRetryPolicyRecovery(c)
	require.True(t, ok)
	require.Equal(t, operation_setting.RetryPolicyActionFailover, recovery.Action)
	require.Equal(t, []string{"backup"}, recovery.RetryGroups)
	require.Equal(t, []int{28}, recovery.TargetChannelIDs)
	require.Equal(t, []string{"stable"}, recovery.TargetTags)
	require.Equal(t, 12, recovery.ExcludeChannelID)
	require.Equal(t, 2, recovery.MaxRetries)
}

func TestShouldRetryPolicyCanRouteChannelErrors(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:       "invalid key failover",
			Action:     operation_setting.RetryPolicyActionFailover,
			ErrorCodes: []types.ErrorCode{types.ErrorCodeChannelInvalidKey},
			Targets: operation_setting.RetryPolicyTargets{
				Groups: []string{"backup"},
			},
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, 12)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c.Set("original_model", "gpt-5")
	c.Set("relay_retry_index", 0)

	err := types.NewError(
		errors.New("channel invalid key"),
		types.ErrorCodeChannelInvalidKey,
	)

	require.True(t, shouldRetry(c, err, 1))
	recovery, ok := service.GetRetryPolicyRecovery(c)
	require.True(t, ok)
	require.Equal(t, operation_setting.RetryPolicyActionFailover, recovery.Action)
	require.Equal(t, []string{"backup"}, recovery.RetryGroups)
	require.Equal(t, 12, recovery.ExcludeChannelID)
}

func TestShouldRetryPolicyCanSkipChannelErrors(t *testing.T) {
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:       "invalid key stop",
			Action:     operation_setting.RetryPolicyActionSkipRetry,
			ErrorCodes: []types.ErrorCode{types.ErrorCodeChannelInvalidKey},
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})
	c.Set("original_model", "gpt-5")
	c.Set("relay_retry_index", 0)

	err := types.NewError(
		errors.New("channel invalid key"),
		types.ErrorCodeChannelInvalidKey,
	)

	require.False(t, shouldRetry(c, err, 1))
	_, ok := service.GetRetryPolicyRecovery(c)
	require.False(t, ok)
}
