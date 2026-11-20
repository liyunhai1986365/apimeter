package gemini

import (
	"os"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	// Gin mode is global; configure it before parallel tests start.
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}
