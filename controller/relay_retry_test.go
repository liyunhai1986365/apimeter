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
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openRelayRetryEventTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	model.InitColForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Workspace{}, &model.Log{}, &model.RetryRouteEvent{}))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

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

func TestRelayAttemptRequestStateRestoresOriginalRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Add("X-Client-Header", "one")
	c.Request.Header.Add("X-Client-Header", "two")

	info := &relaycommon.RelayInfo{
		RelayMode:              relayconstant.RelayModeImagesGenerations,
		RequestURLPath:         "/v1/images/generations",
		RelayFormat:            types.RelayFormatOpenAIImage,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAIImage},
		PriceData: types.PriceData{
			OtherRatios: map[string]float64{"original": 1.5},
		},
	}
	state := captureRelayAttemptRequestState(c, info)

	info.RelayMode = relayconstant.RelayModeImagesEdits
	info.RequestURLPath = "/v1/images/edits"
	info.IsStream = true
	info.RequestConversionChain = append(info.RequestConversionChain, types.RelayFormatGemini)
	info.FinalRequestRelayFormat = types.RelayFormatGemini
	info.RuntimeHeadersOverride = map[string]interface{}{"X-Upstream": "changed"}
	info.UseRuntimeHeadersOverride = true
	info.ParamOverrideAudit = []string{"changed"}
	info.PriceData.OtherRatios["original"] = 9
	info.PriceData.OtherRatios["failed_channel"] = 2
	info.UpstreamRequestBodySize = 42
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=changed")
	c.Request.Header.Set("X-Client-Header", "changed")

	state.restore(c, info)

	require.Equal(t, relayconstant.RelayModeImagesGenerations, info.RelayMode)
	require.Equal(t, "/v1/images/generations", info.RequestURLPath)
	require.False(t, info.IsStream)
	require.Equal(t, []types.RelayFormat{types.RelayFormatOpenAIImage}, info.RequestConversionChain)
	require.Empty(t, info.FinalRequestRelayFormat)
	require.False(t, info.UseRuntimeHeadersOverride)
	require.Nil(t, info.RuntimeHeadersOverride)
	require.Nil(t, info.ParamOverrideAudit)
	require.Equal(t, map[string]float64{"original": 1.5}, info.PriceData.OtherRatios)
	require.Zero(t, info.UpstreamRequestBodySize)
	require.Equal(t, "application/json", c.Request.Header.Get("Content-Type"))
	require.Equal(t, []string{"one", "two"}, c.Request.Header.Values("X-Client-Header"))
}

func TestShouldRetryAllowsPendingUserTokenGroupWhenChannelRetryDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, false)
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})

	err := types.NewOpenAIError(
		errors.New("upstream overloaded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	require.True(t, shouldRetryWithTokenGroupPlan(c, err, 1, true))
	require.False(t, shouldRetryWithTokenGroupPlan(c, err, 1, false))
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

func TestShouldRetryRecordsRetryRouteEvent(t *testing.T) {
	openRelayRetryEventTestDB(t)
	origRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = origRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{
		{
			Name:        "global failover 500",
			Action:      operation_setting.RetryPolicyActionFailover,
			StatusCodes: "500",
			Targets: operation_setting.RetryPolicyTargets{
				Groups: []string{"backup"},
			},
			Strategy: operation_setting.RetryPolicyStrategy{
				MaxRetries: 1,
			},
		},
	}

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	c.Set(common.RequestIdKey, "req-retry-event")
	c.Set("relay_retry_index", 0)
	common.SetContextKey(c, constant.ContextKeyChannelRetryEnabled, true)
	common.SetContextKey(c, constant.ContextKeyChannelId, 10)
	common.SetContextKey(c, constant.ContextKeyChannelName, "primary")
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})

	err := types.NewOpenAIError(
		errors.New("upstream exploded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)

	require.True(t, shouldRetry(c, err, 1))

	events, total, getErr := model.GetRetryRouteEvents(model.RetryRouteEventQuery{RequestId: "req-retry-event", Page: 1, PageSize: 20})
	require.NoError(t, getErr)
	require.Equal(t, int64(1), total)
	require.Equal(t, "global failover 500", events[0].RuleName)
	require.Equal(t, operation_setting.RetryPolicyActionFailover, events[0].Action)
	require.Equal(t, 10, events[0].SourceChannelId)
	require.Equal(t, "gpt-4o", events[0].OriginalModel)
}

func TestProcessChannelErrorPersistsStructuredErrorWithoutRequestDetails(t *testing.T) {
	openRelayRetryEventTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = originalErrorLogEnabled })
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions?debug=1", strings.NewReader(`{"messages":[{"role":"user","content":"private prompt"}]}`))
	c.Request.Header.Set("Authorization", "Bearer sk-private")
	c.Request.RemoteAddr = "203.0.113.10:12345"
	c.Set(common.RequestIdKey, "req-structured-error")
	c.Set("id", 1)
	c.Set("username", "alice")
	c.Set("token_name", "prod-token")
	c.Set("token_id", 3)
	c.Set("original_model", "gpt-4o")
	c.Set("group", "default")
	c.Set("channel_id", 10)
	c.Set("channel_name", "primary")
	c.Set("channel_type", 1)
	event := &model.RetryRouteEvent{RequestId: "req-structured-error", Matched: true, RuleName: "failover"}
	require.NoError(t, model.RecordRetryRouteEvent(event))
	service.SetCurrentRetryRouteEventID(c, event.Id)

	err := types.NewOpenAIError(
		errors.New("upstream exploded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)
	processChannelError(c, types.ChannelError{ChannelId: 10}, err)

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).First(&log).Error)
	require.Equal(t, "req-structured-error", log.RequestId)
	require.Equal(t, "gpt-4o", log.ModelName)
	require.Equal(t, 10, log.ChannelId)
	require.Equal(t, "203.0.113.10", log.Ip)
	require.Contains(t, log.Content, "upstream exploded")

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	require.Equal(t, "/v1/chat/completions", other["request_path"])
	require.Equal(t, "POST", other["request_method"])
	require.Equal(t, "bad_response_status_code", other["error_code"])
	require.Contains(t, other, "retry_route_event_ids")
	require.NotContains(t, other, "request_hash")
	require.NotContains(t, other, "request_log_lookup")
	require.NotContains(t, other, "error_request_log_id")
	require.NotContains(t, log.Other, "private prompt")
	require.NotContains(t, log.Other, "sk-private")

	var updatedEvent model.RetryRouteEvent
	require.NoError(t, model.DB.First(&updatedEvent, event.Id).Error)
	require.Equal(t, log.Id, updatedEvent.LogId)
}

func TestRecordRelayErrorLogHonorsDisabledConfigAndNoRecordOption(t *testing.T) {
	openRelayRetryEventTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	t.Cleanup(func() { constant.ErrorLogEnabled = originalErrorLogEnabled })

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	constant.ErrorLogEnabled = false
	require.Zero(t, recordRelayErrorLog(c, nil, types.NewError(errors.New("disabled"), types.ErrorCodeBadResponse)))

	constant.ErrorLogEnabled = true
	require.Zero(t, recordRelayErrorLog(c, nil, types.NewError(errors.New("expected quota rejection"), types.ErrorCodeInsufficientUserQuota, types.ErrOptionWithNoRecordErrorLog())))

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.Zero(t, count)
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

func TestBuildRetryPolicyInputIncludesTokenWorkspace(t *testing.T) {
	db := openRelayRetryEventTestDB(t)
	require.NoError(t, db.Create(&model.Token{Id: 45, UserId: 1, WorkspaceId: 501, Name: "project-key", Key: "sk-project"}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(nil)
	common.SetContextKey(c, constant.ContextKeyTokenId, 45)

	input := buildRetryPolicyInput(c, types.NewOpenAIError(
		errors.New("upstream overloaded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	))

	require.Equal(t, 501, input.WorkspaceID)
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
