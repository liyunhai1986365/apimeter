package conversion

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	ContextKeyConversionID         = "conversion_id"
	ContextKeySourceRequestMode    = "source_request_mode"
	ContextKeyTargetRequestMode    = "target_request_mode"
	ContextKeyPreserveResponse     = "preserve_response_mode"
	ContextKeySourceRelayFormat    = "source_relay_format"
	ContextKeyTargetRelayFormat    = "target_relay_format"
	ContextKeySourceRelayMode      = "source_relay_mode"
	ContextKeyTargetRelayMode      = "target_relay_mode"
	ContextKeySourceRequestURLPath = "source_request_url_path"
	ContextKeyTargetRequestURLPath = "target_request_url_path"
)

type Plan struct {
	ID                   ConversionID
	SourceMode           RequestMode
	TargetMode           RequestMode
	SourceRelayFormat    types.RelayFormat
	TargetRelayFormat    types.RelayFormat
	SourceRelayMode      int
	TargetRelayMode      int
	SourceRequestURLPath string
	TargetRequestURLPath string
	PreserveResponseMode bool
}

func (p *Plan) Store(c *gin.Context) {
	if p == nil || c == nil {
		return
	}
	c.Set(ContextKeyConversionID, string(p.ID))
	c.Set(ContextKeySourceRequestMode, string(p.SourceMode))
	c.Set(ContextKeyTargetRequestMode, string(p.TargetMode))
	c.Set(ContextKeyPreserveResponse, p.PreserveResponseMode)
	c.Set(ContextKeySourceRelayFormat, string(p.SourceRelayFormat))
	c.Set(ContextKeyTargetRelayFormat, string(p.TargetRelayFormat))
	c.Set(ContextKeySourceRelayMode, p.SourceRelayMode)
	c.Set(ContextKeyTargetRelayMode, p.TargetRelayMode)
	c.Set(ContextKeySourceRequestURLPath, p.SourceRequestURLPath)
	c.Set(ContextKeyTargetRequestURLPath, p.TargetRequestURLPath)
	c.Set("relay_mode", p.TargetRelayMode)
	// Keep the legacy marker during migration so older call sites remain safe.
	if p.ID == ConversionOpenAIChatToImageGenerations {
		c.Set("chat_image_compat", true)
	}
}

func ActiveConversionID(c *gin.Context) ConversionID {
	if c == nil {
		return ""
	}
	return ConversionID(c.GetString(ContextKeyConversionID))
}

func ShouldPreserveResponseMode(c *gin.Context) bool {
	if c == nil {
		return false
	}
	return c.GetBool(ContextKeyPreserveResponse)
}

func channelSettings(c *gin.Context) (dto.ChannelSettings, bool) {
	if c == nil {
		return dto.ChannelSettings{}, false
	}
	value, ok := c.Get(string(constant.ContextKeyChannelSetting))
	if !ok {
		return dto.ChannelSettings{}, false
	}
	settings, ok := value.(dto.ChannelSettings)
	return settings, ok
}

func conversionEnabled(c *gin.Context, id ConversionID) bool {
	settings, ok := channelSettings(c)
	return !ok || ConversionEnabled(settings, id)
}

func conversionAllowed(c *gin.Context, id ConversionID) bool {
	settings, ok := channelSettings(c)
	if !ok || settings.Protocol == nil || len(settings.Protocol.EnabledConversions) == 0 {
		return true
	}
	return ConversionEnabled(settings, id)
}

func protocolConfigured(c *gin.Context) bool {
	settings, ok := channelSettings(c)
	return ok && settings.Protocol != nil
}

func nativeModeSupported(c *gin.Context, mode RequestMode) bool {
	settings, ok := channelSettings(c)
	return !ok || NativeModeSupported(settings, mode)
}

func ConversionEnabled(settings dto.ChannelSettings, id ConversionID) bool {
	if settings.Protocol == nil {
		return true
	}
	if len(settings.Protocol.EnabledConversions) == 0 {
		return false
	}
	for _, enabled := range settings.Protocol.EnabledConversions {
		if enabled == string(id) {
			return true
		}
	}
	return false
}

func NativeModeSupported(settings dto.ChannelSettings, mode RequestMode) bool {
	if settings.Protocol == nil || len(settings.Protocol.NativeModes) == 0 {
		return true
	}
	for _, nativeMode := range settings.Protocol.NativeModes {
		if nativeMode == string(mode) {
			return true
		}
	}
	return false
}

func SupportsConversion(settings dto.ChannelSettings, source RequestMode, target RequestMode, id ConversionID) bool {
	return NativeModeSupported(settings, source) &&
		ConversionEnabled(settings, id) &&
		NativeModeSupported(settings, target)
}
