package conversion

import (
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
)

type RequestMode string
type MediaIntent string
type ConversionID string

const (
	RequestModeOpenAIChat             RequestMode = "openai.chat"
	RequestModeOpenAIImageGenerations RequestMode = "openai.image.generations"
	RequestModeOpenAIImageEdits       RequestMode = "openai.image.edits"
	RequestModeOpenAIVideoGenerations RequestMode = "openai.video.generations"
	RequestModeOpenAIResponses        RequestMode = "openai.responses"
	RequestModeOpenAIResponsesCompact RequestMode = "openai.responses.compact"
	RequestModeClaudeMessages         RequestMode = "claude.messages"
	RequestModeGeminiGenerateContent  RequestMode = "gemini.generate_content"
)

const (
	MediaIntentImageGenerate MediaIntent = "image.generate"
	MediaIntentImageEdit     MediaIntent = "image.edit"
	MediaIntentVideoGenerate MediaIntent = "video.generate"
)

const (
	ConversionOpenAIChatToImageGenerations            ConversionID = "openai.chat_to_openai.image.generations"
	ConversionGeminiGenerateContentToImageGenerations ConversionID = "gemini.generate_content_to_openai.image.generations"
)

func ModeFromRelay(format types.RelayFormat, relayMode int) RequestMode {
	switch format {
	case types.RelayFormatOpenAI:
		switch relayMode {
		case constant.RelayModeChatCompletions:
			return RequestModeOpenAIChat
		}
	case types.RelayFormatOpenAIImage:
		switch relayMode {
		case constant.RelayModeImagesGenerations:
			return RequestModeOpenAIImageGenerations
		case constant.RelayModeImagesEdits:
			return RequestModeOpenAIImageEdits
		}
	case types.RelayFormatOpenAIResponses:
		return RequestModeOpenAIResponses
	case types.RelayFormatOpenAIResponsesCompaction:
		return RequestModeOpenAIResponsesCompact
	case types.RelayFormatClaude:
		return RequestModeClaudeMessages
	case types.RelayFormatGemini:
		return RequestModeGeminiGenerateContent
	}
	return ""
}
