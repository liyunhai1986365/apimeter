package logger

import (
	"bytes"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

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
