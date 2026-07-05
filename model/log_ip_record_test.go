package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupLogIPRecordTest(t *testing.T) {
	t.Helper()

	originalDB := DB
	originalLogDB := LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalLogConsumeEnabled := common.LogConsumeEnabled

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))

	DB = db
	LOG_DB = db
	common.RedisEnabled = false
	common.LogConsumeEnabled = true
	gin.SetMode(gin.TestMode)

	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})
}

func newLogIPRecordContext() *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	c.Request = req
	c.Set("username", "alice")
	return c
}

func TestRecordConsumeLogRecordsIPByDefaultWhenUserSettingOmitsFlag(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	id := RecordConsumeLog(newLogIPRecordContext(), 1, RecordConsumeLogParams{Content: "consume"})

	require.NotZero(t, id)
	var log Log
	require.NoError(t, LOG_DB.First(&log, id).Error)
	require.Equal(t, "203.0.113.10", log.Ip)
}

func TestRecordConsumeLogOmitsIPWhenUserSettingDisablesIPLog(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: `{"record_ip_log":false}`}).Error)

	id := RecordConsumeLog(newLogIPRecordContext(), 1, RecordConsumeLogParams{Content: "consume"})

	require.NotZero(t, id)
	var log Log
	require.NoError(t, LOG_DB.First(&log, id).Error)
	require.Empty(t, log.Ip)
}

func TestRecordErrorLogRecordsIPByDefaultWhenUserSettingOmitsFlag(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	RecordErrorLog(newLogIPRecordContext(), 1, 2, "gpt-test", "prod-token", "failed", 3, 4, false, "default", nil)

	var log Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeError).First(&log).Error)
	require.Equal(t, "203.0.113.10", log.Ip)
}

func TestRecordErrorLogOmitsIPWhenUserSettingDisablesIPLog(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: `{"record_ip_log":false}`}).Error)

	RecordErrorLog(newLogIPRecordContext(), 1, 2, "gpt-test", "prod-token", "failed", 3, 4, false, "default", nil)

	var log Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeError).First(&log).Error)
	require.Empty(t, log.Ip)
}

func TestCreateErrorLogReturnsCreatedLogID(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	ctx := newLogIPRecordContext()
	logID := CreateErrorLog(ctx, 1, 2, "gpt-test", "prod-token", "failed", 3, 4, false, "default", map[string]interface{}{
		"request_log_lookup": map[string]interface{}{"request_id": "req-log"},
	})

	require.NotZero(t, logID)
	var log Log
	require.NoError(t, LOG_DB.First(&log, logID).Error)
	require.Equal(t, "failed", log.Content)
	require.Contains(t, log.Other, "request_log_lookup")
}
