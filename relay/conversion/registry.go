package conversion

import (
	"fmt"

	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type Converter interface {
	ID() ConversionID
	From() RequestMode
	To() RequestMode
	Match(c *gin.Context, request dto.Request) bool
	ConvertRequest(c *gin.Context, request dto.Request) (dto.Request, error)
	Plan(c *gin.Context, sourceFormat types.RelayFormat, sourceMode int) *Plan
}

var converters = []Converter{
	openAIChatToImageGenerationsConverter{},
}

func ApplyRequest(c *gin.Context, relayFormat types.RelayFormat, relayMode int, request dto.Request) (dto.Request, *Plan, error) {
	if request == nil {
		return request, nil, nil
	}
	sourceMode := ModeFromRelay(relayFormat, relayMode)
	if sourceMode == "" {
		return request, nil, nil
	}
	for _, converter := range converters {
		if converter.From() != sourceMode {
			continue
		}
		if !converter.Match(c, request) {
			continue
		}
		if protocolConfigured(c) && !nativeModeSupported(c, sourceMode) {
			return nil, nil, fmt.Errorf("request mode %s is not supported by current channel", sourceMode)
		}
		if !conversionEnabled(c, converter.ID()) {
			return nil, nil, fmt.Errorf("conversion %s is not supported by current channel", converter.ID())
		}
		if !nativeModeSupported(c, converter.To()) {
			return nil, nil, fmt.Errorf("request mode %s is not supported by current channel", converter.To())
		}
		convertedRequest, err := converter.ConvertRequest(c, request)
		if err != nil {
			return nil, nil, err
		}
		plan := converter.Plan(c, relayFormat, relayMode)
		return convertedRequest, plan, nil
	}
	return request, nil, nil
}

func targetImageGenerationsPlan(c *gin.Context, id ConversionID, sourceFormat types.RelayFormat, sourceMode int) *Plan {
	sourcePath := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		sourcePath = c.Request.URL.String()
	}
	return &Plan{
		ID:                   id,
		SourceMode:           ModeFromRelay(sourceFormat, sourceMode),
		TargetMode:           RequestModeOpenAIImageGenerations,
		SourceRelayFormat:    sourceFormat,
		TargetRelayFormat:    types.RelayFormatOpenAIImage,
		SourceRelayMode:      sourceMode,
		TargetRelayMode:      relayconstant.RelayModeImagesGenerations,
		SourceRequestURLPath: sourcePath,
		TargetRequestURLPath: "/v1/images/generations",
		PreserveResponseMode: true,
	}
}
