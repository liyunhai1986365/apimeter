package types

import relaytypes "github.com/QuantumNous/new-api/relaykit/types"

// Relay protocol types now live in the standalone relaykit module. Keep aliases
// here for Modelsell host extensions that still also depend on host-only types
// such as PriceData and AgentContext.
type (
	ChannelError       = relaytypes.ChannelError
	EndpointType       = relaytypes.EndpointType
	LocalFileData      = relaytypes.LocalFileData
	FileSource         = relaytypes.FileSource
	URLSource          = relaytypes.URLSource
	Base64Source       = relaytypes.Base64Source
	CachedFileData     = relaytypes.CachedFileData
	RelayFormat        = relaytypes.RelayFormat
	FileType           = relaytypes.FileType
	TokenType          = relaytypes.TokenType
	TokenCountMeta     = relaytypes.TokenCountMeta
	FileMeta           = relaytypes.FileMeta
	RequestMeta        = relaytypes.RequestMeta
	OpenAIError        = relaytypes.OpenAIError
	ClaudeError        = relaytypes.ClaudeError
	ErrorType          = relaytypes.ErrorType
	ErrorCode          = relaytypes.ErrorCode
	NewAPIError        = relaytypes.NewAPIError
	NewAPIErrorOptions = relaytypes.NewAPIErrorOptions
)

const (
	EndpointTypeOpenAI                = relaytypes.EndpointTypeOpenAI
	EndpointTypeOpenAIResponse        = relaytypes.EndpointTypeOpenAIResponse
	EndpointTypeOpenAIResponseCompact = relaytypes.EndpointTypeOpenAIResponseCompact
	EndpointTypeOpenAIAlphaSearch     = relaytypes.EndpointTypeOpenAIAlphaSearch
	EndpointTypeAnthropic             = relaytypes.EndpointTypeAnthropic
	EndpointTypeGemini                = relaytypes.EndpointTypeGemini
	EndpointTypeJinaRerank            = relaytypes.EndpointTypeJinaRerank
	EndpointTypeImageGeneration       = relaytypes.EndpointTypeImageGeneration
	EndpointTypeEmbeddings            = relaytypes.EndpointTypeEmbeddings
	EndpointTypeOpenAIVideo           = relaytypes.EndpointTypeOpenAIVideo

	RelayFormatOpenAI                    = relaytypes.RelayFormatOpenAI
	RelayFormatClaude                    = relaytypes.RelayFormatClaude
	RelayFormatGemini                    = relaytypes.RelayFormatGemini
	RelayFormatOpenAIResponses           = relaytypes.RelayFormatOpenAIResponses
	RelayFormatOpenAIResponsesCompaction = relaytypes.RelayFormatOpenAIResponsesCompaction
	RelayFormatOpenAIAlphaSearch         = relaytypes.RelayFormatOpenAIAlphaSearch
	RelayFormatOpenAIAudio               = relaytypes.RelayFormatOpenAIAudio
	RelayFormatOpenAIImage               = relaytypes.RelayFormatOpenAIImage
	RelayFormatOpenAIRealtime            = relaytypes.RelayFormatOpenAIRealtime
	RelayFormatRerank                    = relaytypes.RelayFormatRerank
	RelayFormatEmbedding                 = relaytypes.RelayFormatEmbedding
	RelayFormatTask                      = relaytypes.RelayFormatTask
	RelayFormatMjProxy                   = relaytypes.RelayFormatMjProxy

	FileTypeImage = relaytypes.FileTypeImage
	FileTypeAudio = relaytypes.FileTypeAudio
	FileTypeVideo = relaytypes.FileTypeVideo
	FileTypeFile  = relaytypes.FileTypeFile

	TokenTypeTextNumber = relaytypes.TokenTypeTextNumber
	TokenTypeTokenizer  = relaytypes.TokenTypeTokenizer
	TokenTypeImage      = relaytypes.TokenTypeImage

	ErrorTypeNewAPIError     = relaytypes.ErrorTypeNewAPIError
	ErrorTypeOpenAIError     = relaytypes.ErrorTypeOpenAIError
	ErrorTypeClaudeError     = relaytypes.ErrorTypeClaudeError
	ErrorTypeMidjourneyError = relaytypes.ErrorTypeMidjourneyError
	ErrorTypeGeminiError     = relaytypes.ErrorTypeGeminiError
	ErrorTypeRerankError     = relaytypes.ErrorTypeRerankError
	ErrorTypeUpstreamError   = relaytypes.ErrorTypeUpstreamError

	ErrorCodeInvalidRequest               = relaytypes.ErrorCodeInvalidRequest
	ErrorCodeSensitiveWordsDetected       = relaytypes.ErrorCodeSensitiveWordsDetected
	ErrorCodeViolationFeeGrokCSAM         = relaytypes.ErrorCodeViolationFeeGrokCSAM
	ErrorCodeCountTokenFailed             = relaytypes.ErrorCodeCountTokenFailed
	ErrorCodeModelPriceError              = relaytypes.ErrorCodeModelPriceError
	ErrorCodeInvalidApiType               = relaytypes.ErrorCodeInvalidApiType
	ErrorCodeJsonMarshalFailed            = relaytypes.ErrorCodeJsonMarshalFailed
	ErrorCodeDoRequestFailed              = relaytypes.ErrorCodeDoRequestFailed
	ErrorCodeGetChannelFailed             = relaytypes.ErrorCodeGetChannelFailed
	ErrorCodeGenRelayInfoFailed           = relaytypes.ErrorCodeGenRelayInfoFailed
	ErrorCodeChannelNoAvailableKey        = relaytypes.ErrorCodeChannelNoAvailableKey
	ErrorCodeChannelParamOverrideInvalid  = relaytypes.ErrorCodeChannelParamOverrideInvalid
	ErrorCodeChannelHeaderOverrideInvalid = relaytypes.ErrorCodeChannelHeaderOverrideInvalid
	ErrorCodeChannelModelMappedError      = relaytypes.ErrorCodeChannelModelMappedError
	ErrorCodeChannelAwsClientError        = relaytypes.ErrorCodeChannelAwsClientError
	ErrorCodeChannelInvalidKey            = relaytypes.ErrorCodeChannelInvalidKey
	ErrorCodeChannelResponseTimeExceeded  = relaytypes.ErrorCodeChannelResponseTimeExceeded
	ErrorCodeReadRequestBodyFailed        = relaytypes.ErrorCodeReadRequestBodyFailed
	ErrorCodeConvertRequestFailed         = relaytypes.ErrorCodeConvertRequestFailed
	ErrorCodeAccessDenied                 = relaytypes.ErrorCodeAccessDenied
	ErrorCodeBadRequestBody               = relaytypes.ErrorCodeBadRequestBody
	ErrorCodeReadResponseBodyFailed       = relaytypes.ErrorCodeReadResponseBodyFailed
	ErrorCodeBadResponseStatusCode        = relaytypes.ErrorCodeBadResponseStatusCode
	ErrorCodeBadResponse                  = relaytypes.ErrorCodeBadResponse
	ErrorCodeBadResponseBody              = relaytypes.ErrorCodeBadResponseBody
	ErrorCodeEmptyResponse                = relaytypes.ErrorCodeEmptyResponse
	ErrorCodeAwsInvokeError               = relaytypes.ErrorCodeAwsInvokeError
	ErrorCodeModelNotFound                = relaytypes.ErrorCodeModelNotFound
	ErrorCodePromptBlocked                = relaytypes.ErrorCodePromptBlocked
	ErrorCodeQueryDataError               = relaytypes.ErrorCodeQueryDataError
	ErrorCodeUpdateDataError              = relaytypes.ErrorCodeUpdateDataError
	ErrorCodeInsufficientUserQuota        = relaytypes.ErrorCodeInsufficientUserQuota
	ErrorCodePreConsumeTokenQuotaFailed   = relaytypes.ErrorCodePreConsumeTokenQuotaFailed
)

var (
	FinishReasonStop          = relaytypes.FinishReasonStop
	FinishReasonToolCalls     = relaytypes.FinishReasonToolCalls
	FinishReasonLength        = relaytypes.FinishReasonLength
	FinishReasonFunctionCall  = relaytypes.FinishReasonFunctionCall
	FinishReasonContentFilter = relaytypes.FinishReasonContentFilter

	NewChannelError               = relaytypes.NewChannelError
	NewURLFileSource              = relaytypes.NewURLFileSource
	NewBase64FileSource           = relaytypes.NewBase64FileSource
	NewFileSourceFromData         = relaytypes.NewFileSourceFromData
	NewMemoryCachedData           = relaytypes.NewMemoryCachedData
	NewDiskCachedData             = relaytypes.NewDiskCachedData
	NewFileMeta                   = relaytypes.NewFileMeta
	NewImageFileMeta              = relaytypes.NewImageFileMeta
	NewError                      = relaytypes.NewError
	NewOpenAIError                = relaytypes.NewOpenAIError
	InitOpenAIError               = relaytypes.InitOpenAIError
	NewErrorWithStatusCode        = relaytypes.NewErrorWithStatusCode
	WithOpenAIError               = relaytypes.WithOpenAIError
	WithClaudeError               = relaytypes.WithClaudeError
	IsChannelError                = relaytypes.IsChannelError
	IsSkipRetryError              = relaytypes.IsSkipRetryError
	ErrOptionWithSkipRetry        = relaytypes.ErrOptionWithSkipRetry
	ErrOptionWithNoRecordErrorLog = relaytypes.ErrOptionWithNoRecordErrorLog
	ErrOptionWithStatusCode       = relaytypes.ErrOptionWithStatusCode
	ErrOptionWithHideErrMsg       = relaytypes.ErrOptionWithHideErrMsg
	IsRecordErrorLog              = relaytypes.IsRecordErrorLog
)
