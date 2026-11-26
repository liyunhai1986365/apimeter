package model

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}, &BillingUsageItem{}))

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

func TestRecordConsumeLogWritesUsageToOriginalDatabase(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)
	original := LOG_DB
	replacement, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	queryStarted, resume, inserted := make(chan struct{}), make(chan struct{}), make(chan struct{})
	require.NoError(t, original.Callback().Query().Before("gorm:query").Register("test:pause_log_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			close(queryStarted)
			<-resume
		}
	}))
	require.NoError(t, original.Callback().Create().After("gorm:create").Register("test:usage_inserted", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*BillingUsageItem); ok {
			close(inserted)
		}
	}))
	logID := RecordConsumeLog(newLogIPRecordContext(), 1, RecordConsumeLogParams{Quota: 850})
	require.NotZero(t, logID)
	select {
	case <-queryStarted:
	case <-time.After(5 * time.Second):
		close(resume)
		t.Fatal("background log lookup did not start")
	}
	LOG_DB = replacement
	close(resume)
	select {
	case <-inserted:
	case <-time.After(5 * time.Second):
		t.Fatal("usage item did not reach original database")
	}
	var usage BillingUsageItem
	require.NoError(t, original.Where("log_id = ?", logID).First(&usage).Error)
	require.EqualValues(t, 850, usage.SettlementAmount)
}

func TestCreateErrorLogRecordsIPByDefault(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)

	id := CreateErrorLog(newLogIPRecordContext(), 1, 10, "gpt-4o", "prod-token", "upstream failed", 3, 2, false, "default", map[string]interface{}{"status_code": 502})

	require.NotZero(t, id)
	var log Log
	require.NoError(t, LOG_DB.First(&log, id).Error)
	require.Equal(t, LogTypeError, log.Type)
	require.Equal(t, "203.0.113.10", log.Ip)
}

func TestCreateErrorLogOmitsIPWhenUserSettingDisablesIPLog(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: `{"record_ip_log":false}`}).Error)

	id := CreateErrorLog(newLogIPRecordContext(), 1, 10, "gpt-4o", "prod-token", "upstream failed", 3, 2, false, "default", nil)

	require.NotZero(t, id)
	var log Log
	require.NoError(t, LOG_DB.First(&log, id).Error)
	require.Empty(t, log.Ip)
}

func TestRecordConsumeLogForcePersistsWhenConsumeLoggingDisabled(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)
	common.LogConsumeEnabled = false

	require.Zero(t, RecordConsumeLog(newLogIPRecordContext(), 1, RecordConsumeLogParams{Content: "ordinary"}))
	logID := RecordConsumeLog(newLogIPRecordContext(), 1, RecordConsumeLogParams{Force: true, Content: "agent settlement"})

	require.NotZero(t, logID)
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestRecordTaskBillingLogForcePersistsWhenConsumeLoggingDisabled(t *testing.T) {
	setupLogIPRecordTest(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "alice", Password: "password", Setting: "{}"}).Error)
	common.LogConsumeEnabled = false

	require.Zero(t, RecordTaskBillingLog(RecordTaskBillingLogParams{UserId: 1, LogType: LogTypeConsume, Content: "ordinary"}))
	logID := RecordTaskBillingLog(RecordTaskBillingLogParams{Force: true, UserId: 1, LogType: LogTypeConsume, Content: "agent settlement"})

	require.NotZero(t, logID)
	var count int64
	require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}
