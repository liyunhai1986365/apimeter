package volcengine

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type imageStreamEvent struct {
	Type  string    `json:"type"`
	Usage dto.Usage `json:"usage"`
}

func handleImageStreamResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(errors.New("invalid image stream response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	usage := &dto.Usage{}
	helper.StreamScannerHandler(c, resp, info, func(data string, result *helper.StreamResult) {
		var event imageStreamEvent
		if err := common.Unmarshal([]byte(data), &event); err == nil && event.Type == "image_generation.completed" {
			*usage = event.Usage
			if usage.InputTokens > 0 {
				usage.PromptTokens += usage.InputTokens
			}
			if usage.OutputTokens > 0 {
				usage.CompletionTokens += usage.OutputTokens
			}
		}
		if err := helper.StringData(c, data); err != nil {
			result.Stop(err)
		}
	})
	return usage, nil
}
