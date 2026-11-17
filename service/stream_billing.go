package service

import (
	"errors"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// SettleInterruptedStream preserves incurred usage on a failed stream. Normal
// upstream errors have no marker and remain refundable. The billing session
// makes the controller's subsequent Refund a no-op after successful settlement.
func SettleInterruptedStream(c *gin.Context, info *relaycommon.RelayInfo, err error) {
	var streamErr *relaycommon.StreamUsageError
	if info == nil || !errors.As(err, &streamErr) {
		return
	}
	streamErr.SettleOnce(func(usage *dto.Usage) {
		// Follow the same pricing branch as the successful entry handler.
		audio := false
		if info.RelayMode == relayconstant.RelayModeResponses {
			audio = strings.HasPrefix(info.OriginModelName, "gpt-4o-audio")
		} else if info.RelayFormat == types.RelayFormatOpenAI {
			hasAudio := usage.PromptTokensDetails.AudioTokens > 0 || usage.CompletionTokenDetails.AudioTokens > 0
			audio = hasAudio && (ratio_setting.ContainsAudioRatio(info.OriginModelName) || ratio_setting.ContainsAudioCompletionRatio(info.OriginModelName))
		}
		if audio {
			postAudioConsumeQuota(c, info, usage, "流式响应中断，按已产生的用量结算", false)
		} else {
			postTextConsumeQuota(c, info, usage, []string{"流式响应中断，按已产生的用量结算"}, false)
		}
	})
}
