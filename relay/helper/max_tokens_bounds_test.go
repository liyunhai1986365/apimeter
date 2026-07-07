package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestMaxTokensBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newJSONContext := func(body string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/relay", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")
		return c
	}

	const huge = "18446744073686646784"

	t.Run("openai max_tokens overflow rejected", func(t *testing.T) {
		_, err := GetAndValidateTextRequest(newJSONContext(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_tokens":`+huge+`}`), relayconstant.RelayModeChatCompletions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("openai max_completion_tokens overflow rejected", func(t *testing.T) {
		_, err := GetAndValidateTextRequest(newJSONContext(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":`+huge+`}`), relayconstant.RelayModeChatCompletions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("claude max_tokens overflow rejected", func(t *testing.T) {
		_, err := GetAndValidateClaudeRequest(newJSONContext(`{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens":` + huge + `}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("gemini maxOutputTokens overflow rejected", func(t *testing.T) {
		_, err := GetAndValidateGeminiRequest(newJSONContext(`{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":` + huge + `}}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxOutputTokens is invalid")
	})

	t.Run("responses max_output_tokens overflow rejected", func(t *testing.T) {
		_, err := GetAndValidateResponsesRequest(newJSONContext(`{"model":"gpt-4o","input":"hi","max_output_tokens":` + huge + `}`))
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_output_tokens is invalid")
	})
}
