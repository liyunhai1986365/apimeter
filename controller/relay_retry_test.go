package controller

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
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
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Workspace{}, &model.Log{}, &model.RetryRouteEvent{}, &model.ErrorRequestLog{}))
	model.DB = db
	model.LOG_DB = db
	model.InitErrorRequestLogQueue(100)

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

func TestProcessChannelErrorLinksRetryRouteEventToErrorLog(t *testing.T) {
	openRelayRetryEventTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = originalErrorLogEnabled })
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	event := &model.RetryRouteEvent{
		RequestId:       "req-error-log-link",
		RuleName:        "global failover 500",
		Action:          operation_setting.RetryPolicyActionFailover,
		Matched:         true,
		SourceChannelId: 10,
	}
	require.NoError(t, model.RecordRetryRouteEvent(event))

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Request.RemoteAddr = "203.0.113.10:12345"
	c.Set(common.RequestIdKey, "req-error-log-link")
	c.Set("id", 1)
	c.Set("username", "alice")
	c.Set("token_name", "prod-token")
	c.Set("token_id", 3)
	c.Set("original_model", "gpt-4o")
	c.Set("group", "default")
	c.Set("channel_id", 10)
	c.Set("channel_name", "primary")
	c.Set("channel_type", 1)
	service.SetCurrentRetryRouteEventID(c, event.Id)

	err := types.NewOpenAIError(
		errors.New("upstream exploded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)
	processChannelError(c, types.ChannelError{ChannelId: 10}, err)

	var log model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).First(&log).Error)
	require.Equal(t, "req-error-log-link", log.RequestId)
	require.Contains(t, log.Other, "retry_route_event_ids")
	require.Contains(t, log.Other, "request_log_lookup")

	var updated model.RetryRouteEvent
	require.NoError(t, model.DB.First(&updated, event.Id).Error)
	require.Equal(t, log.Id, updated.LogId)
}

func TestRecordRelayErrorLogCapturesRequestSnapshotForNonChannelError(t *testing.T) {
	openRelayRetryEventTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = originalErrorLogEnabled })
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}],"api_key":"sk-secret"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions?debug=1", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer sk-client")
	c.Request.RemoteAddr = "203.0.113.10:12345"
	storage, err := common.CreateBodyStorage([]byte(body))
	require.NoError(t, err)
	t.Cleanup(func() { _ = storage.Close() })
	c.Set(common.KeyBodyStorage, storage)
	c.Request.Body = io.NopCloser(storage)
	c.Set(common.RequestIdKey, "req-non-channel-error")
	c.Set("id", 1)
	c.Set("username", "alice")
	c.Set("token_name", "prod-token")
	c.Set("token_id", 3)
	c.Set("original_model", "gpt-4o")
	c.Set("group", "default")
	common.SetContextKey(c, constant.ContextKeyRequestStartTime, time.Now().Add(-2*time.Second))

	apiErr := types.NewErrorWithStatusCode(
		errors.New("invalid request payload"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
	)

	logID := recordRelayErrorLog(c, nil, apiErr)
	require.NotZero(t, logID)
	require.NoError(t, model.FlushQueuedErrorRequestLogsForTest())

	var log model.Log
	require.NoError(t, model.LOG_DB.First(&log, logID).Error)
	require.Equal(t, model.LogTypeError, log.Type)
	require.Equal(t, "req-non-channel-error", log.RequestId)
	require.Equal(t, "gpt-4o", log.ModelName)
	require.Equal(t, 0, log.ChannelId)
	require.Equal(t, 3, log.TokenId)

	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
	require.Equal(t, "/v1/chat/completions", other["request_path"])
	require.Equal(t, "POST", other["request_method"])
	require.Equal(t, "invalid_request", other["error_code"])
	require.Equal(t, float64(http.StatusBadRequest), other["status_code"])
	require.NotContains(t, other, "request_snapshot")
	require.NotEmpty(t, other["error_request_log_id"])
	require.NotEmpty(t, other["request_hash"])
	lookup, ok := other["request_log_lookup"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "req-non-channel-error", lookup["request_id"])

	var requestLog model.ErrorRequestLog
	require.NoError(t, model.LOG_DB.First(&requestLog, int(other["error_request_log_id"].(float64))).Error)
	require.Equal(t, log.Id, requestLog.LogId)
	require.Equal(t, "req-non-channel-error", requestLog.RequestId)
	require.Equal(t, "/v1/chat/completions", requestLog.RequestPath)
	require.Equal(t, "POST", requestLog.RequestMethod)
	require.Equal(t, "gpt-4o", requestLog.ModelName)
	require.NotEmpty(t, requestLog.RequestHash)
	require.Contains(t, requestLog.RequestBody, "gpt-4o")
	require.NotContains(t, requestLog.RequestBody, "sk-secret")
	require.NotContains(t, requestLog.RequestHeaders, "sk-client")
}

func TestRecordRelayErrorLogSkipsDuplicateForChannelError(t *testing.T) {
	openRelayRetryEventTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = originalErrorLogEnabled })
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.RemoteAddr = "203.0.113.10:12345"
	c.Set(common.RequestIdKey, "req-channel-dedup")
	c.Set("id", 1)
	c.Set("username", "alice")
	c.Set("token_name", "prod-token")
	c.Set("token_id", 3)
	c.Set("original_model", "gpt-4o")
	c.Set("group", "default")
	c.Set("channel_id", 10)
	c.Set("channel_name", "primary")
	c.Set("channel_type", 1)

	apiErr := types.NewOpenAIError(
		errors.New("upstream exploded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)
	channelErr := types.ChannelError{ChannelId: 10}

	firstID := recordRelayErrorLog(c, &channelErr, apiErr)
	secondID := recordRelayErrorLog(c, nil, apiErr)
	require.NoError(t, model.FlushQueuedErrorRequestLogsForTest())

	require.NotZero(t, firstID)
	require.Equal(t, firstID, secondID)
	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeError).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestRecordRelayErrorLogKeepsEachChannelAttempt(t *testing.T) {
	openRelayRetryEventTestDB(t)
	originalErrorLogEnabled := constant.ErrorLogEnabled
	constant.ErrorLogEnabled = true
	t.Cleanup(func() { constant.ErrorLogEnabled = originalErrorLogEnabled })
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o"}`))
	c.Request.RemoteAddr = "203.0.113.10:12345"
	c.Set(common.RequestIdKey, "req-channel-attempts")
	c.Set("id", 1)
	c.Set("username", "alice")
	c.Set("token_name", "prod-token")
	c.Set("token_id", 3)
	c.Set("original_model", "gpt-4o")
	c.Set("group", "default")

	c.Set("channel_id", 10)
	c.Set("channel_name", "primary")
	c.Set("channel_type", 1)
	firstErr := types.NewOpenAIError(
		errors.New("primary exploded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusInternalServerError,
	)
	firstChannel := types.ChannelError{ChannelId: 10}
	firstID := recordRelayErrorLog(c, &firstChannel, firstErr)

	c.Set("channel_id", 11)
	c.Set("channel_name", "backup")
	c.Set("channel_type", 1)
	secondErr := types.NewOpenAIError(
		errors.New("backup exploded"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusBadGateway,
	)
	secondChannel := types.ChannelError{ChannelId: 11}
	secondID := recordRelayErrorLog(c, &secondChannel, secondErr)
	finalID := recordRelayErrorLog(c, nil, secondErr)
	require.NoError(t, model.FlushQueuedErrorRequestLogsForTest())

	require.NotZero(t, firstID)
	require.NotZero(t, secondID)
	require.NotEqual(t, firstID, secondID)
	require.Equal(t, secondID, finalID)

	var logs []model.Log
	require.NoError(t, model.LOG_DB.Where("type = ?", model.LogTypeError).Order("id asc").Find(&logs).Error)
	require.Len(t, logs, 2)
	require.Equal(t, 10, logs[0].ChannelId)
	require.Equal(t, 11, logs[1].ChannelId)
	require.Contains(t, logs[0].Content, "primary exploded")
	require.Contains(t, logs[1].Content, "backup exploded")
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
