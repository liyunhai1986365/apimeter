package service

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openRetryRouteEventRecorderTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	model.InitColForTest()

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Workspace{}, &model.RetryRouteEvent{}))
	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	return db
}

func TestBuildRetryRouteEventFromDecision(t *testing.T) {
	db := openRetryRouteEventRecorderTestDB(t)
	require.NoError(t, db.Create(&model.Workspace{Id: 501, UserId: 10, Name: "Project Alpha", Status: model.WorkspaceStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 20, UserId: 10, WorkspaceId: 501, Name: "prod-key", Key: "sk-prod"}).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Request = req
	c.Set(common.RequestIdKey, "req-1")
	c.Set(common.UpstreamRequestIdKey, "up-1")
	c.Set("relay_retry_index", 2)
	common.SetContextKey(c, constant.ContextKeyUserId, 10)
	common.SetContextKey(c, constant.ContextKeyUserName, "alice")
	common.SetContextKey(c, constant.ContextKeyTokenId, 20)
	common.SetContextKey(c, constant.ContextKeyChannelId, 30)
	common.SetContextKey(c, constant.ContextKeyChannelName, "primary")
	common.SetContextKey(c, constant.ContextKeyChannelType, 1)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o")
	common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")

	event := BuildRetryRouteEvent(c, operation_setting.RetryPolicyDecision{
		Matched:     true,
		ShouldRetry: true,
		Action:      operation_setting.RetryPolicyActionFailover,
		Source:      operation_setting.RetryPolicySourceGlobal,
		RuleName:    "backup",
	}, &types.NewAPIError{
		StatusCode: 429,
	})

	require.Equal(t, "req-1", event.RequestId)
	require.Equal(t, "up-1", event.UpstreamRequestId)
	require.Equal(t, 2, event.AttemptIndex)
	require.Equal(t, "backup", event.RuleName)
	require.Equal(t, "failover", event.Action)
	require.Equal(t, 30, event.SourceChannelId)
	require.Equal(t, "primary", event.SourceChannelName)
	require.Equal(t, "vip", event.SourceGroup)
	require.Equal(t, "gpt-4o", event.OriginalModel)
	require.Equal(t, "/v1/chat/completions", event.RequestPath)
	require.Equal(t, 429, event.StatusCode)
	require.Equal(t, 501, event.WorkspaceId)
	require.Equal(t, "Project Alpha", event.WorkspaceName)
}

func TestBuildRetryRouteEventStoresSampleRateInExtra(t *testing.T) {
	openRetryRouteEventRecorderTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req-observe")
	event := BuildRetryRouteEvent(c, operation_setting.RetryPolicyDecision{
		Matched:     true,
		ShouldRetry: true,
		Action:      operation_setting.RetryPolicyActionFailover,
		Source:      operation_setting.RetryPolicySourceGlobal,
		RuleName:    "backup",
		SampleRate:  35,
	}, &types.NewAPIError{
		StatusCode: 500,
	})

	var extra map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(event.Extra, &extra))
	require.Equal(t, float64(35), extra["sample_rate"])
}

func TestBuildRetryRouteEventDoesNotStoreRequestHash(t *testing.T) {
	openRetryRouteEventRecorderTestDB(t)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`))
	c.Set(common.RequestIdKey, "req-hash")

	event := BuildRetryRouteEvent(c, operation_setting.RetryPolicyDecision{
		Matched:     true,
		ShouldRetry: true,
		Action:      operation_setting.RetryPolicyActionFailover,
		Source:      operation_setting.RetryPolicySourceGlobal,
		RuleName:    "backup",
	}, &types.NewAPIError{
		StatusCode: 500,
	})

	require.Empty(t, event.RequestHash)
}

func TestRetryRouteEventContextTracksCurrentEvent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	SetCurrentRetryRouteEventID(c, 123)
	id, ok := GetCurrentRetryRouteEventID(c)

	require.True(t, ok)
	require.Equal(t, 123, id)
}

func TestRetryRouteWithoutEventsSkipsUpdatesAndPreservesOutcome(t *testing.T) {
	db := openRetryRouteEventRecorderTestDB(t)
	updates := 0
	const callback = "test:retry_route_empty_update"
	require.NoError(t, db.Callback().Update().Before("gorm:begin_transaction").Register(callback, func(*gorm.DB) {
		updates++
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callback)) })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, "req-no-events")
	for _, success := range []bool{true, false} {
		MarkRetryRouteFinal(c, success, "completed")
		outcome, exists := c.Get("relay_route_final_success")
		require.True(t, exists)
		require.Equal(t, success, outcome)
	}
	AttachRetryRouteLog(c, 101)
	require.Zero(t, updates, "requests without recorded retry events must not start an UPDATE")
}

func TestRetryRouteFinalUpdatesAllRecordedEvents(t *testing.T) {
	db := openRetryRouteEventRecorderTestDB(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, "req-final-events")
	// A matched stop decision is still an event, even without an actual retry.
	for _, shouldRetry := range []bool{true, false} {
		RecordRetryRouteDecision(c, operation_setting.RetryPolicyDecision{Matched: true, ShouldRetry: shouldRetry}, nil)
	}
	_, recorded := GetCurrentRetryRouteEventID(c)
	require.True(t, recorded)
	unrelated := &model.RetryRouteEvent{RequestId: "req-other-final"}
	require.NoError(t, model.RecordRetryRouteEvent(unrelated))

	for _, outcome := range []struct {
		success bool
		status  string
	}{{true, "success"}, {false, "failed"}} {
		MarkRetryRouteFinal(c, outcome.success, outcome.status)
		var events []model.RetryRouteEvent
		require.NoError(t, db.Where("request_id = ?", "req-final-events").Find(&events).Error)
		require.Len(t, events, 2)
		for _, event := range events {
			require.Equal(t, outcome.success, event.FinalSuccess)
			require.Equal(t, outcome.status, event.FinalStatus)
		}
	}
	require.NoError(t, db.First(unrelated, unrelated.Id).Error)
	require.Empty(t, unrelated.FinalStatus)
	require.False(t, unrelated.FinalSuccess)
}

func TestRetryRouteLogAttachesAllUnlinkedRecordedEvents(t *testing.T) {
	db := openRetryRouteEventRecorderTestDB(t)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(common.RequestIdKey, "req-log-events")
	for range 2 {
		RecordRetryRouteDecision(c, operation_setting.RetryPolicyDecision{Matched: true, ShouldRetry: true}, nil)
	}
	previouslyLinked := &model.RetryRouteEvent{RequestId: "req-log-events", LogId: 90}
	unrelated := &model.RetryRouteEvent{RequestId: "req-other-log"}
	require.NoError(t, model.RecordRetryRouteEvent(previouslyLinked))
	require.NoError(t, model.RecordRetryRouteEvent(unrelated))

	AttachRetryRouteLog(c, 101)
	AttachRetryRouteLog(c, 102)
	var events []model.RetryRouteEvent
	require.NoError(t, db.Where("request_id = ?", "req-log-events").Order("id").Find(&events).Error)
	require.Len(t, events, 3)
	require.Equal(t, 101, events[0].LogId)
	require.Equal(t, 101, events[1].LogId)
	require.Equal(t, 90, events[2].LogId, "an existing log association must be preserved")
	require.NoError(t, db.First(unrelated, unrelated.Id).Error)
	require.Zero(t, unrelated.LogId)
}
