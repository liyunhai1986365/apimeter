package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestReadinessChecksProbeDependenciesIndependently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	checked := make(chan struct{}, 2)
	mainError := errors.New("ping database: pool exhausted")
	healthy := func(ctx context.Context) error {
		checked <- struct{}{}
		return ctx.Err()
	}
	results := runReadinessChecks(ctx, []readinessProbe{
		{name: "database", check: func(ctx context.Context) error {
			// The main database cannot finish until both independent checks
			// run. A serial implementation would exhaust the shared deadline.
			for range 2 {
				select {
				case <-checked:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return mainError
		}},
		{name: "log_database", check: healthy},
		{name: "redis", check: healthy},
	})
	require.Len(t, results, 3)
	require.ErrorIs(t, results[0].err, mainError)
	require.NoError(t, results[1].err)
	require.NoError(t, results[2].err)
	require.NoError(t, ctx.Err())
}

func TestReadinessChecksRespectOverallDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	returned := make(chan struct{})
	defer func() {
		close(release)
		select {
		case <-returned:
		case <-time.After(time.Second):
			t.Error("late readiness probe did not return")
		}
	}()
	done := make(chan []readinessResult, 1)
	go func() {
		done <- runReadinessChecks(ctx, []readinessProbe{{name: "database", check: func(context.Context) error {
			// Simulate a driver that does not return promptly on cancellation.
			<-release
			close(returned)
			return nil
		}}})
	}()
	select {
	case results := <-done:
		require.Len(t, results, 1)
		require.ErrorIs(t, results[0].err, context.DeadlineExceeded)
		require.Equal(t, "database", results[0].name)
	case <-time.After(time.Second):
		t.Fatal("readiness exceeded its overall deadline")
	}
}

func TestGetReadinessSharedDatabaseIsCheckedOnce(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:controller-readiness?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	var reads atomic.Int32
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:readiness_reads", func(tx *gorm.DB) {
		reads.Add(1)
	}))
	setReadinessTestDatabases(t, db, db)
	response := invokeReadiness(t)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int32(1), reads.Load(), "shared main/log database should run one writable-state query")
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Checks map[string]string `json:"checks"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	require.True(t, body.Success)
	require.Equal(t, map[string]string{"database": "ok", "log_database": "ok", "redis": "disabled"}, body.Data.Checks)
}

func TestGetReadinessDoesNotExposeDependencyErrors(t *testing.T) {
	setReadinessTestDatabases(t, nil, nil)
	response := invokeReadiness(t)
	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Checks map[string]string `json:"checks"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &body))
	require.False(t, body.Success)
	require.Equal(t, map[string]string{"database": "unavailable", "log_database": "unavailable", "redis": "disabled"}, body.Data.Checks)
	require.NotContains(t, response.Body.String(), "database is not initialized")
}

func setReadinessTestDatabases(t *testing.T, db, logDB *gorm.DB) {
	t.Helper()
	originalDB, originalLogDB, originalRedisEnabled := model.DB, model.LOG_DB, common.RedisEnabled
	model.DB, model.LOG_DB, common.RedisEnabled = db, logDB, false
	t.Cleanup(func() {
		model.DB, model.LOG_DB, common.RedisEnabled = originalDB, originalLogDB, originalRedisEnabled
	})
}

func invokeReadiness(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	GetReadiness(c)
	return response
}
