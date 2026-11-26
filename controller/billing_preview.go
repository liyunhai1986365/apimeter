package controller

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// PreviewBillingExpression prices sample upstream usage at group ratio 1.
// Draft expressions have no request body, response body or headers.
func PreviewBillingExpression(c *gin.Context) {
	var sample struct {
		Expression    string `json:"expression" binding:"required,max=16384"`
		Format        string `json:"format" binding:"oneof=openai anthropic"`
		Prompt        int    `json:"p" binding:"min=0,max=1000000000"`
		Completion    int    `json:"c" binding:"min=0,max=1000000000"`
		CacheRead     int    `json:"cr" binding:"min=0,max=1000000000"`
		CacheWrite    int    `json:"cc" binding:"min=0,max=1000000000"`
		CacheWrite12h int    `json:"cc12h" binding:"min=0,max=1000000000"`
		CacheWrite1h  int    `json:"cc1h" binding:"min=0,max=1000000000"`
		ImageIn       int    `json:"img" binding:"min=0,max=1000000000"`
		ImageOut      int    `json:"img_o" binding:"min=0,max=1000000000"`
		AudioIn       int    `json:"ai" binding:"min=0,max=1000000000"`
		AudioOut      int    `json:"ao" binding:"min=0,max=1000000000"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	if err := c.ShouldBindJSON(&sample); err != nil {
		common.ApiError(c, err)
		return
	}
	expression := strings.TrimSpace(sample.Expression)
	used, err := billingexpr.CompileUsedVars(expression)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	usage := dto.Usage{
		UsageSemantic: sample.Format,
		PromptTokens:  sample.Prompt, CompletionTokens: sample.Completion,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: sample.CacheRead, CachedCreationTokens: sample.CacheWrite,
			ImageTokens: sample.ImageIn, AudioTokens: sample.AudioIn,
		},
		CompletionTokenDetails: dto.OutputTokenDetails{ImageTokens: sample.ImageOut, AudioTokens: sample.AudioOut},
	}
	claude := sample.Format == "anthropic"
	if claude && sample.CacheWrite12h != 0 {
		common.ApiErrorMsg(c, "12-hour writes require OpenAI usage")
		return
	}
	if claude {
		usage.ClaudeCacheCreation5mTokens = sample.CacheWrite
		usage.ClaudeCacheCreation1hTokens = sample.CacheWrite1h
		usage.PromptTokensDetails.CachedCreationTokens += sample.CacheWrite1h
	} else if sample.CacheWrite1h != 0 {
		common.ApiErrorMsg(c, "OpenAI usage does not report one-hour cache writes separately")
		return
	}
	if !claude && sample.CacheWrite12h > 0 {
		usage.PromptTokensDetails.CachedCreationTokens += sample.CacheWrite12h
		usage.PromptTokensDetails.CacheWriteTokensDetails = &dto.CacheWriteTokenDetails{TTL30mTokens: sample.CacheWrite, TTL12hTokens: sample.CacheWrite12h}
	}
	result, err := billingexpr.ComputeTieredQuota(&billingexpr.BillingSnapshot{
		ExprString: expression, ExprHash: billingexpr.ExprHashString(expression),
		ExprVersion: billingexpr.ExprVersion(expression), QuotaPerUnit: common.QuotaPerUnit, GroupRatio: 1,
	}, service.BuildTieredTokenParams(&usage, claude, used))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"quota": result.ActualQuotaAfterGroup, "tier": result.MatchedTier})
}
