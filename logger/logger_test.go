package logger

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func isolateLogger(t *testing.T) {
	t.Helper()
	setupLogLock.Lock()
	originalCount := logCount
	logCount = 0
	setupLogLock.Unlock()
	originalDir := *common.LogDir
	*common.LogDir = ""
	currentLogPathMu.Lock()
	originalPath, originalFile := currentLogPath, currentLogFile
	currentLogPath, currentLogFile = "", nil
	currentLogPathMu.Unlock()
	common.LogWriterMu.Lock()
	originalWriter, originalErrorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	gin.DefaultWriter, gin.DefaultErrorWriter = io.Discard, io.Discard
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		waitForLoggerSetup(t)
		*common.LogDir = originalDir
		currentLogPathMu.Lock()
		testFile := currentLogFile
		currentLogPath, currentLogFile = originalPath, originalFile
		currentLogPathMu.Unlock()
		common.LogWriterMu.Lock()
		gin.DefaultWriter, gin.DefaultErrorWriter = originalWriter, originalErrorWriter
		if testFile != nil {
			_ = testFile.Close()
		}
		common.LogWriterMu.Unlock()
		setupLogLock.Lock()
		logCount = originalCount
		setupLogLock.Unlock()
	})
}

func waitForLoggerSetup(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		setupLogLock.Lock()
		defer setupLogLock.Unlock()
		return !setupLogWorking
	}, 5*time.Second, time.Millisecond)
}

func writeConcurrentLogs(workers, messages int) {
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for message := 0; message < messages; message++ {
				LogInfo(nil, fmt.Sprintf("worker=%d message=%d", worker, message))
			}
		}(worker)
	}
	wg.Wait()
}

func TestConcurrentLoggingCountsEveryMessage(t *testing.T) {
	isolateLogger(t)
	writeConcurrentLogs(16, 100)
	setupLogLock.Lock()
	count, working := logCount, setupLogWorking
	setupLogLock.Unlock()
	require.Equal(t, 1600, count)
	require.False(t, working)
}

func TestConcurrentLoggingReservesSingleRotation(t *testing.T) {
	isolateLogger(t)
	*common.LogDir = t.TempDir()
	SetupLogger()
	oldPath, oldFile := GetCurrentLogPath(), currentLogFile
	// Capture concurrent writes in the old file without duplicating test output.
	common.LogWriterMu.Lock()
	gin.DefaultWriter, gin.DefaultErrorWriter = oldFile, oldFile
	common.LogWriterMu.Unlock()
	LogInfo(nil, "before-rotation")

	// A different directory makes the rotation independent of clock resolution.
	*common.LogDir = t.TempDir()
	setupLogLock.Lock()
	logCount = maxLogCount - 1
	setupLogLock.Unlock()
	// Hold the file switch so all log calls overlap the pending rotation.
	currentLogPathMu.Lock()
	switchBlocked := true
	defer func() {
		if switchBlocked {
			currentLogPathMu.Unlock()
		}
	}()
	writeConcurrentLogs(16, 100)
	// An overlapping explicit setup must not release the async reservation.
	SetupLogger()
	setupLogLock.Lock()
	count, working := logCount, setupLogWorking
	setupLogLock.Unlock()
	require.True(t, working)
	require.Equal(t, 1598, count, "only the first threshold crossing resets the count")
	currentLogPathMu.Unlock()
	switchBlocked = false
	waitForLoggerSetup(t)

	require.NotEqual(t, oldPath, GetCurrentLogPath())
	_, err := oldFile.WriteString("must be closed")
	require.ErrorIs(t, err, os.ErrClosed)
	LogInfo(nil, "after-rotation")
	before, err := os.ReadFile(oldPath)
	require.NoError(t, err)
	require.Contains(t, string(before), "before-rotation")
	require.Equal(t, 1601, strings.Count(string(before), "\n"))
	after, err := os.ReadFile(GetCurrentLogPath())
	require.NoError(t, err)
	require.Contains(t, string(after), "after-rotation")
}

func TestRotationWithoutLogDirectoryReleasesReservation(t *testing.T) {
	isolateLogger(t)
	setupLogLock.Lock()
	logCount = maxLogCount
	setupLogLock.Unlock()
	LogInfo(nil, "rotation-without-file")
	waitForLoggerSetup(t)
	setupLogLock.Lock()
	count := logCount
	setupLogLock.Unlock()
	require.Zero(t, count)
	require.Empty(t, GetCurrentLogPath())
	common.LogWriterMu.RLock()
	writer, errorWriter := gin.DefaultWriter, gin.DefaultErrorWriter
	common.LogWriterMu.RUnlock()
	require.Equal(t, io.Discard, writer)
	require.Equal(t, io.Discard, errorWriter)
}

func TestStandardLogWritersRemainAvailableForExternalCollector(t *testing.T) {
	var infoOutput bytes.Buffer
	var errorOutput bytes.Buffer

	common.LogWriterMu.Lock()
	originalWriter := gin.DefaultWriter
	originalErrorWriter := gin.DefaultErrorWriter
	gin.DefaultWriter = &infoOutput
	gin.DefaultErrorWriter = &errorOutput
	common.LogWriterMu.Unlock()
	t.Cleanup(func() {
		common.LogWriterMu.Lock()
		gin.DefaultWriter = originalWriter
		gin.DefaultErrorWriter = originalErrorWriter
		common.LogWriterMu.Unlock()
	})

	LogInfo(nil, "normal-log-collector-marker")
	LogError(nil, "error-log-collector-marker")

	if output := infoOutput.String(); !strings.Contains(output, "[INFO]") || !strings.Contains(output, "normal-log-collector-marker") {
		t.Fatalf("standard info log was not written for collection: %q", output)
	}
	if output := errorOutput.String(); !strings.Contains(output, "[ERR]") || !strings.Contains(output, "error-log-collector-marker") {
		t.Fatalf("standard error log was not written for collection: %q", output)
	}
}
