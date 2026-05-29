package conversion

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type Requirement struct {
	SourceMode RequestMode
	TargetMode RequestMode
	Conversion ConversionID
	Intent     MediaIntent
}

func (r Requirement) Empty() bool {
	return r.SourceMode == "" && r.TargetMode == "" && r.Conversion == ""
}

func (r Requirement) Supports(settings dto.ChannelSettings) bool {
	if r.Empty() {
		return true
	}
	if r.Conversion != "" {
		return SupportsConversion(settings, r.SourceMode, r.TargetMode, r.Conversion)
	}
	return NativeModeSupported(settings, r.SourceMode)
}

func (r Requirement) UnsupportedError(modelName string) error {
	if r.Conversion != "" {
		return fmt.Errorf("当前模型 %s 没有可用渠道支持 %s 请求模式或 %s 转化", modelName, r.SourceMode, r.Conversion)
	}
	if r.SourceMode != "" {
		return fmt.Errorf("当前模型 %s 没有可用渠道支持 %s 请求模式", modelName, r.SourceMode)
	}
	return fmt.Errorf("当前模型 %s 没有可用渠道支持当前请求模式", modelName)
}

func RequirementFromRequest(c *gin.Context, relayFormat types.RelayFormat, relayMode int, modelName string) Requirement {
	sourceMode := ModeFromRelay(relayFormat, relayMode)
	if c != nil && c.Request != nil {
		reqMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
		if reqMode != relayconstant.RelayModeUnknown {
			sourceMode = ModeFromRelay(types.RelayFormatOpenAI, reqMode)
		}
	}
	return RequirementFromMode(sourceMode, modelName)
}

func RequirementFromHTTPRequest(c *gin.Context, modelName string) Requirement {
	if c == nil || c.Request == nil {
		return Requirement{}
	}
	if c.Request.Method != http.MethodPost {
		return Requirement{}
	}
	relayMode := relayconstant.Path2RelayMode(c.Request.URL.Path)
	sourceMode := ModeFromRelay(types.RelayFormatOpenAI, relayMode)
	if strings.HasPrefix(c.Request.URL.Path, "/v1/video/generations") ||
		strings.HasPrefix(c.Request.URL.Path, "/v1/videos") {
		sourceMode = RequestModeOpenAIVideoGenerations
	}
	return RequirementFromMode(sourceMode, modelName)
}

func RequirementFromMode(sourceMode RequestMode, modelName string) Requirement {
	if sourceMode == "" {
		return Requirement{}
	}
	if sourceMode == RequestModeOpenAIChat && common.IsImageGenerationModel(modelName) {
		return Requirement{
			SourceMode: sourceMode,
			TargetMode: RequestModeOpenAIImageGenerations,
			Conversion: ConversionOpenAIChatToImageGenerations,
			Intent:     MediaIntentImageGenerate,
		}
	}
	return Requirement{
		SourceMode: sourceMode,
		TargetMode: sourceMode,
	}
}
