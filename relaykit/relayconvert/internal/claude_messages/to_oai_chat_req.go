package claudemessages

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

const (
	webSearchMaxUsesLow    = 1
	webSearchMaxUsesMedium = 5
	webSearchMaxUsesHigh   = 10
)

type openRouterRequestReasoning struct {
	Enabled   bool   `json:"enabled"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
}

func ClaudeMessagesRequestToOpenAIChat(claudeRequest dto.ClaudeRequest, info convmeta.Meta) (*dto.GeneralOpenAIRequest, error) {
	openAIRequest := dto.GeneralOpenAIRequest{
		Model:       claudeRequest.Model,
		Temperature: claudeRequest.Temperature,
	}
	if claudeRequest.MaxTokens != nil {
		openAIRequest.MaxTokens = kitutil.GetPointer(*claudeRequest.MaxTokens)
	}
	if claudeRequest.TopP != nil {
		openAIRequest.TopP = kitutil.GetPointer(*claudeRequest.TopP)
	}
	if claudeRequest.TopK != nil {
		openAIRequest.TopK = kitutil.GetPointer(*claudeRequest.TopK)
	}
	if claudeRequest.Stream != nil {
		openAIRequest.Stream = kitutil.GetPointer(*claudeRequest.Stream)
	}

	isOpenRouter := convmeta.OptionsOf(info).OpenRouterDialect
	if isOpenRouter {
		if effort := claudeRequest.GetEfforts(); effort != "" {
			effortBytes, _ := kitutil.Marshal(effort)
			openAIRequest.Verbosity = effortBytes
		}
		if claudeRequest.Thinking != nil {
			var reasoningConfig openRouterRequestReasoning
			if claudeRequest.Thinking.Type == "enabled" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled:   true,
					MaxTokens: claudeRequest.Thinking.GetBudgetTokens(),
				}
			} else if claudeRequest.Thinking.Type == "adaptive" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled: true,
				}
			}
			reasoningJSON, err := kitutil.Marshal(reasoningConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal reasoning: %w", err)
			}
			openAIRequest.Reasoning = reasoningJSON
		}
	} else if info != nil {
		thinkingSuffix := "-thinking"
		if strings.HasSuffix(info.GetOriginModelName(), thinkingSuffix) &&
			!strings.HasSuffix(openAIRequest.Model, thinkingSuffix) {
			openAIRequest.Model = openAIRequest.Model + thinkingSuffix
		}
	}

	if len(claudeRequest.StopSequences) == 1 {
		openAIRequest.Stop = claudeRequest.StopSequences[0]
	} else if len(claudeRequest.StopSequences) > 1 {
		openAIRequest.Stop = claudeRequest.StopSequences
	}

	tools, _ := kitutil.Any2Type[[]dto.Tool](claudeRequest.Tools)
	openAITools := make([]dto.ToolCallRequest, 0)
	for _, claudeTool := range tools {
		openAITool := dto.ToolCallRequest{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        claudeTool.Name,
				Description: claudeTool.Description,
				Parameters:  claudeTool.InputSchema,
			},
		}
		openAITools = append(openAITools, openAITool)
	}
	openAIRequest.Tools = openAITools

	openAIMessages := make([]dto.Message, 0)
	if claudeRequest.System != nil {
		if claudeRequest.IsStringSystem() && claudeRequest.GetStringSystem() != "" {
			openAIMessage := dto.Message{
				Role: "system",
			}
			openAIMessage.SetStringContent(claudeRequest.GetStringSystem())
			openAIMessages = append(openAIMessages, openAIMessage)
		} else {
			systems := claudeRequest.ParseSystem()
			if len(systems) > 0 {
				openAIMessage := dto.Message{
					Role: "system",
				}
				isOpenRouterClaude := isOpenRouter && strings.HasPrefix(convmeta.UpstreamModelName(info), "anthropic/claude")
				if isOpenRouterClaude {
					systemMediaMessages := make([]dto.MediaContent, 0, len(systems))
					for _, system := range systems {
						message := dto.MediaContent{
							Type:         "text",
							Text:         system.GetText(),
							CacheControl: system.CacheControl,
						}
						systemMediaMessages = append(systemMediaMessages, message)
					}
					openAIMessage.SetMediaContent(systemMediaMessages)
				} else {
					systemStr := ""
					for _, system := range systems {
						if system.Text != nil {
							systemStr += *system.Text
						}
					}
					openAIMessage.SetStringContent(systemStr)
				}
				openAIMessages = append(openAIMessages, openAIMessage)
			}
		}
	}

	pendingToolCalls := make(map[string]bool)
	var deferredUserContent []dto.MediaContent
	for _, claudeMessage := range claudeRequest.Messages {
		if claudeMessage.Role == "assistant" {
			if len(deferredUserContent) > 0 {
				return nil, fmt.Errorf("tool results are missing before the next assistant message")
			}
			clear(pendingToolCalls)
		}
		hasToolImages := false
		openAIMessage := dto.Message{
			Role: claudeMessage.Role,
		}
		if claudeMessage.IsStringContent() {
			openAIMessage.SetStringContent(claudeMessage.GetStringContent())
		} else {
			content, err := claudeMessage.ParseContent()
			if err != nil {
				return nil, err
			}
			var toolCalls []dto.ToolCallRequest
			mediaMessages := make([]dto.MediaContent, 0, len(content))

			for _, mediaMsg := range content {
				switch mediaMsg.Type {
				case "text", "input_text":
					message := dto.MediaContent{
						Type:         "text",
						Text:         mediaMsg.GetText(),
						CacheControl: mediaMsg.CacheControl,
					}
					mediaMessages = append(mediaMessages, message)
				case "image":
					mediaMessage, err := claudeImageToChat(mediaMsg)
					if err != nil {
						return nil, err
					}
					mediaMessages = append(mediaMessages, mediaMessage)
				case "tool_use":
					toolCall := dto.ToolCallRequest{
						ID:   mediaMsg.Id,
						Type: "function",
						Function: dto.FunctionRequest{
							Name:      mediaMsg.Name,
							Arguments: requestToJSONString(mediaMsg.Input),
						},
					}
					toolCalls = append(toolCalls, toolCall)
					pendingToolCalls[mediaMsg.Id] = true
				case "tool_result":
					toolName := mediaMsg.Name
					if toolName == "" {
						toolName = claudeRequest.SearchToolNameByToolCallId(mediaMsg.ToolUseId)
					}
					oaiToolMessage := dto.Message{
						Role:       "tool",
						Name:       &toolName,
						ToolCallId: mediaMsg.ToolUseId,
					}
					toolText, toolImages, err := claudeToolResultToChat(mediaMsg)
					if err != nil {
						return nil, fmt.Errorf("convert tool result %s: %w", mediaMsg.ToolUseId, err)
					}
					oaiToolMessage.SetStringContent(toolText)
					openAIMessages = append(openAIMessages, oaiToolMessage)
					delete(pendingToolCalls, mediaMsg.ToolUseId)
					// Chat tool replies cannot carry image content. Append their
					// images to the user message emitted after all tool replies,
					// with the call ID so parallel results remain distinguishable.
					mediaMessages = append(mediaMessages, toolImages...)
					hasToolImages = hasToolImages || len(toolImages) > 0
				}
			}

			if len(toolCalls) > 0 {
				openAIMessage.SetToolCalls(toolCalls)
			}
			if len(mediaMessages) > 0 {
				openAIMessage.SetMediaContent(mediaMessages)
			}
		}
		if openAIMessage.Role == "user" && (hasToolImages || len(deferredUserContent) > 0) {
			// Claude may split parallel results across consecutive user turns.
			// Keep the generated image message after the entire tool reply set.
			deferredUserContent = append(deferredUserContent, openAIMessage.ParseContent()...)
			if len(pendingToolCalls) > 0 {
				continue
			}
			openAIMessage.SetMediaContent(deferredUserContent)
			deferredUserContent = nil
		}
		if len(openAIMessage.ParseContent()) > 0 || len(openAIMessage.ToolCalls) > 0 {
			openAIMessages = append(openAIMessages, openAIMessage)
		}
	}
	if len(deferredUserContent) > 0 {
		return nil, fmt.Errorf("tool results are missing for image-bearing tool output")
	}

	openAIRequest.Messages = openAIMessages
	return &openAIRequest, nil
}

func claudeImageToChat(image dto.ClaudeMediaMessage) (dto.MediaContent, error) {
	if image.Source == nil {
		return dto.MediaContent{}, fmt.Errorf("image source is required")
	}
	source := image.Source
	var imageURL string
	switch source.Type {
	case "url":
		if strings.TrimSpace(source.Url) == "" {
			return dto.MediaContent{}, fmt.Errorf("image URL is required")
		}
		imageURL = source.Url
	case "base64":
		data, ok := source.Data.(string)
		if !ok || data == "" || strings.TrimSpace(source.MediaType) == "" {
			return dto.MediaContent{}, fmt.Errorf("base64 image data and media_type are required")
		}
		imageURL = fmt.Sprintf("data:%s;base64,%s", source.MediaType, data)
	default:
		return dto.MediaContent{}, fmt.Errorf("unsupported image source type %q", source.Type)
	}
	return dto.MediaContent{
		Type:         "image_url",
		ImageUrl:     &dto.MessageImageUrl{Url: imageURL},
		CacheControl: image.CacheControl,
	}, nil
}

func claudeToolResultToChat(result dto.ClaudeMediaMessage) (string, []dto.MediaContent, error) {
	if result.IsStringContent() {
		return result.GetStringContent(), nil, nil
	}
	blocks, err := kitutil.Any2Type[[]dto.ClaudeMediaMessage](result.Content)
	if err != nil {
		return "", nil, err
	}
	remaining := make([]dto.ClaudeMediaMessage, 0, len(blocks))
	var images []dto.MediaContent
	for _, block := range blocks {
		if block.Type != "image" {
			remaining = append(remaining, block)
			continue
		}
		image, err := claudeImageToChat(block)
		if err != nil {
			return "", nil, err
		}
		images = append(images, image)
	}
	if len(images) == 0 {
		encoded, err := kitutil.Marshal(blocks)
		return string(encoded), nil, err
	}
	images = append([]dto.MediaContent{{Type: "text", Text: fmt.Sprintf("Images from tool call %s:", result.ToolUseId)}}, images...)
	if len(remaining) == 0 {
		return "Image output is attached in the following user message.", images, nil
	}
	encoded, err := kitutil.Marshal(remaining)
	return string(encoded), images, err
}

func requestToJSONString(v interface{}) string {
	b, err := kitutil.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
