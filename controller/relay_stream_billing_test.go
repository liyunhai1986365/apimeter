package controller

import (
	"errors"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestInterruptedStreamNeverRetriesOrFallsBack(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	err := relaycommon.WithStreamUsage(types.NewOpenAIError(errors.New("incomplete function arguments"), types.ErrorCodeBadResponse, 502), &dto.Usage{PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120})
	err = service.NormalizeViolationFeeError(err)
	require.False(t, shouldRetryWithTokenGroupPlan(c, err, 3, true))
	require.False(t, canTryFallbackModel(c, err))
}
