package dto

import relaydto "github.com/QuantumNous/new-api/relaykit/dto"

// Host-only task DTOs still share these primitive response types with relaykit.
type (
	Request                             = relaydto.Request
	ChannelSettings                     = relaydto.ChannelSettings
	ChannelOtherSettings                = relaydto.ChannelOtherSettings
	ChannelProtocolSettings             = relaydto.ChannelProtocolSettings
	AdvancedCustomConfig                = relaydto.AdvancedCustomConfig
	AdvancedCustomRoute                 = relaydto.AdvancedCustomRoute
	AdvancedCustomRouteAuth             = relaydto.AdvancedCustomRouteAuth
	RetryPolicyRule                     = relaydto.RetryPolicyRule
	RetryPolicyConditions               = relaydto.RetryPolicyConditions
	RetryPolicyTargets                  = relaydto.RetryPolicyTargets
	RetryPolicyStrategy                 = relaydto.RetryPolicyStrategy
	VertexKeyType                       = relaydto.VertexKeyType
	AwsKeyType                          = relaydto.AwsKeyType
	UserSetting                         = relaydto.UserSetting
	Notify                              = relaydto.Notify
	Usage                               = relaydto.Usage
	BoolValue                           = relaydto.BoolValue
	StringValue                         = relaydto.StringValue
	ImageRequest                        = relaydto.ImageRequest
	GeneralOpenAIRequest                = relaydto.GeneralOpenAIRequest
	OpenAIResponsesRequest              = relaydto.OpenAIResponsesRequest
	OpenAIResponsesResponse             = relaydto.OpenAIResponsesResponse
	OpenAIResponsesCompactionRequest    = relaydto.OpenAIResponsesCompactionRequest
	OpenAITextResponse                  = relaydto.OpenAITextResponse
	OpenAITextResponseChoice            = relaydto.OpenAITextResponseChoice
	ChatCompletionsStreamResponse       = relaydto.ChatCompletionsStreamResponse
	ChatCompletionsStreamResponseChoice = relaydto.ChatCompletionsStreamResponseChoice
	InputTokenDetails                   = relaydto.InputTokenDetails
	OutputTokenDetails                  = relaydto.OutputTokenDetails
	ToolCallResponse                    = relaydto.ToolCallResponse
	FunctionResponse                    = relaydto.FunctionResponse
	Message                             = relaydto.Message
	GeminiChatRequest                   = relaydto.GeminiChatRequest
	GeminiInlineData                    = relaydto.GeminiInlineData
	OpenAIVideo                         = relaydto.OpenAIVideo
	OpenAIVideoError                    = relaydto.OpenAIVideoError
	MessageImageUrl                     = relaydto.MessageImageUrl
	GeminiChatResponse                  = relaydto.GeminiChatResponse
	GeminiPart                          = relaydto.GeminiPart
	GeminiChatCandidate                 = relaydto.GeminiChatCandidate
	GeminiUsageMetadata                 = relaydto.GeminiUsageMetadata
	GeminiChatContent                   = relaydto.GeminiChatContent
	ImageResponse                       = relaydto.ImageResponse
	ImageData                           = relaydto.ImageData
	SimpleResponse                      = relaydto.SimpleResponse
	RealtimeUsage                       = relaydto.RealtimeUsage
	UserImageStorageSetting             = relaydto.UserImageStorageSetting
	ClaudeUsage                         = relaydto.ClaudeUsage
	ClaudeCacheCreationUsage            = relaydto.ClaudeCacheCreationUsage
	ClaudeMediaMessage                  = relaydto.ClaudeMediaMessage
	ClaudeResponse                      = relaydto.ClaudeResponse
	MediaContent                        = relaydto.MediaContent
	FunctionRequest                     = relaydto.FunctionRequest
	MessageFile                         = relaydto.MessageFile
	ToolCallRequest                     = relaydto.ToolCallRequest
)

const (
	ContentTypeText     = relaydto.ContentTypeText
	ContentTypeImageURL = relaydto.ContentTypeImageURL
	ContentTypeFile     = relaydto.ContentTypeFile
	VertexKeyTypeJSON   = relaydto.VertexKeyTypeJSON
	VertexKeyTypeAPIKey = relaydto.VertexKeyTypeAPIKey
	AwsKeyTypeAKSK      = relaydto.AwsKeyTypeAKSK
	AwsKeyTypeApiKey    = relaydto.AwsKeyTypeApiKey

	VideoStatusQueued     = relaydto.VideoStatusQueued
	VideoStatusInProgress = relaydto.VideoStatusInProgress
	VideoStatusCompleted  = relaydto.VideoStatusCompleted
	VideoStatusFailed     = relaydto.VideoStatusFailed
	VideoStatusUnknown    = relaydto.VideoStatusUnknown

	AdvancedCustomModelListPath      = relaydto.AdvancedCustomModelListPath
	AdvancedCustomBalancePath        = relaydto.AdvancedCustomBalancePath
	AdvancedCustomAuthTypeNone       = relaydto.AdvancedCustomAuthTypeNone
	AdvancedCustomAuthTypeHeader     = relaydto.AdvancedCustomAuthTypeHeader
	AdvancedCustomAuthTypeQuery      = relaydto.AdvancedCustomAuthTypeQuery
	MaxImageN                        = relaydto.MaxImageN
	UserImageStorageTypeAliyunOSS    = relaydto.UserImageStorageTypeAliyunOSS
	UserImageStorageTypeCloudflareR2 = relaydto.UserImageStorageTypeCloudflareR2

	NotifyTypeQuotaExceed     = relaydto.NotifyTypeQuotaExceed
	NotifyTypeBalanceForecast = relaydto.NotifyTypeBalanceForecast
	NotifyTypeChannelUpdate   = relaydto.NotifyTypeChannelUpdate
	NotifyTypeChannelTest     = relaydto.NotifyTypeChannelTest
)

var (
	NewOpenAIVideo                = relaydto.NewOpenAIVideo
	EstimateGPTImage2OutputTokens = relaydto.EstimateGPTImage2OutputTokens
	EstimateGPTImage2ImageTokens  = relaydto.EstimateGPTImage2ImageTokens
	NewNotify                     = relaydto.NewNotify
	NotifyTypeEmail               = relaydto.NotifyTypeEmail
	NotifyTypeBark                = relaydto.NotifyTypeBark
	NotifyTypeGotify              = relaydto.NotifyTypeGotify
	NotifyTypeWebhook             = relaydto.NotifyTypeWebhook
)
