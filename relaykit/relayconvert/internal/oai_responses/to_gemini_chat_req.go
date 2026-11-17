package oairesponses

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	relaymedia "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/media"
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func convertOpenAIResponsesRequestToGeminiChat(c context.Context, info convmeta.Meta, request any) (any, error) {
	responsesRequest, err := OpenAIResponsesRequestFromAny(request)
	if err != nil {
		return nil, err
	}

	prepared, err := PrepareOpenAIResponsesRequest(*responsesRequest)
	if err != nil {
		return nil, err
	}
	return OpenAIResponsesRequestToGeminiChat(c, &prepared, info)
}

func OpenAIResponsesRequestToGeminiChat(c context.Context, req *dto.OpenAIResponsesRequest, info convmeta.Meta) (*dto.GeminiChatRequest, error) {
	opts := convmeta.OptionsOf(info)
	if req == nil {
		return nil, fmt.Errorf("request is nil")
	}
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}
	if err := ValidateRequestChatUnsupportedFields(req); err != nil {
		return nil, err
	}

	geminiRequest := &dto.GeminiChatRequest{
		GenerationConfig: dto.GeminiChatGenerationConfig{
			Temperature: req.Temperature,
		},
	}
	if req.TopP != nil {
		geminiRequest.GenerationConfig.TopP = kitutil.GetPointer(*req.TopP)
	}
	if req.MaxOutputTokens != nil {
		geminiRequest.GenerationConfig.MaxOutputTokens = kitutil.GetPointer(*req.MaxOutputTokens)
	}

	upstreamModelName := req.Model
	if modelName := convmeta.UpstreamModelName(info); modelName != "" {
		upstreamModelName = modelName
	}
	if opts.Gemini.SupportsImagineModel(upstreamModelName) {
		geminiRequest.GenerationConfig.ResponseModalities = []string{"TEXT", "IMAGE"}
	}
	if err := applyResponsesTextToGemini(c, req.Text, geminiRequest, opts.Gemini.SupportsJSONSchemaModel(upstreamModelName)); err != nil {
		return nil, err
	}
	if err := sharedgemini.ApplyThinkingConfig(c, geminiRequest, info, dto.GeneralOpenAIRequest{
		Model:               req.Model,
		MaxCompletionTokens: req.MaxOutputTokens,
		ReasoningEffort:     ReasoningEffort(req),
	}); err != nil {
		return nil, err
	}

	var safetySettings []dto.GeminiChatSafetySettings
	for _, category := range sharedgemini.SafetySettingCategories {
		threshold := opts.Gemini.SafetySettingFor(category)
		if threshold == "" {
			continue
		}
		safetySettings = append(safetySettings, dto.GeminiChatSafetySettings{
			Category:  category,
			Threshold: threshold,
		})
	}
	if len(safetySettings) > 0 {
		geminiRequest.SafetySettings = safetySettings
	}

	toolsDisabled := isResponsesToolChoiceNone(req.ToolChoice)
	tools, toolState, err := responsesToolsToGemini(c, req.Tools, opts.Gemini.SupportsJSONSchemaModel(upstreamModelName), toolsDisabled)
	if err != nil {
		return nil, err
	}
	convmeta.SetResponsesToolState(info, toolState)
	geminiRequest.ToolConfig, err = responsesToolChoiceToGemini(c, req.ToolChoice, toolState)
	if err != nil {
		return nil, err
	}
	// FunctionCallingConfig.NONE only disables function calls, not hosted
	// search. Keep declarations for tool history, but remove hosted tools.
	if toolsDisabled {
		functions := make([]dto.GeminiChatTool, 0, len(tools))
		for _, tool := range tools {
			if tool.FunctionDeclarations != nil {
				functions = append(functions, dto.GeminiChatTool{FunctionDeclarations: tool.FunctionDeclarations})
			}
		}
		tools = functions
	}
	geminiRequest.SetTools(tools)
	var parallel *bool
	if RawJSONPresent(req.ParallelToolCalls) {
		if err := kitutil.Unmarshal(req.ParallelToolCalls, &parallel); err != nil {
			return nil, fmt.Errorf("invalid parallel_tool_calls: %w", err)
		}
	}
	if !toolsDisabled && len(toolState.FunctionNames) > 0 && parallel != nil && !*parallel {
		convdiag.Add(c, types.ConversionDiagnostic{Code: "unsupported_parallel_tool_control", Path: "parallel_tool_calls", Message: "Gemini cannot enforce parallel_tool_calls=false", Severity: types.ConversionDiagnosticError})
	}

	systemTexts := make([]string, 0)
	if RawJSONPresent(req.Instructions) {
		instructions, err := JSONString(req.Instructions)
		if err != nil {
			return nil, fmt.Errorf("invalid instructions: %w", err)
		}
		if strings.TrimSpace(instructions) != "" {
			systemTexts = append(systemTexts, instructions)
		}
	}

	inputItems, err := InputItems(req.Input)
	if err != nil {
		return nil, err
	}
	callNames := make(map[string]string)
	for _, item := range inputItems {
		itemType := strings.TrimSpace(kitutil.Interface2String(item["type"]))
		switch itemType {
		case ResponsesInputTypeFunctionCall, "custom_tool_call":
			part, callID, err := responsesFunctionCallItemToGeminiPart(item)
			if err != nil {
				return nil, err
			}
			sharedgemini.AttachFunctionCallThoughtSignature(opts, &part)
			if callID != "" {
				callNames[callID] = part.FunctionCall.FunctionName
			}
			appendGeminiContentPart(geminiRequest, "model", part)
		case ResponsesInputTypeFunctionCallOutput, ResponsesInputTypeCustomToolCallOutput:
			mediaInParts := !strings.HasPrefix(upstreamModelName, "gemini-1.") && !strings.HasPrefix(upstreamModelName, "gemini-2.")
			part, media, err := responsesFunctionOutputItemToGeminiPart(c, item, callNames, mediaInParts)
			if err != nil {
				return nil, err
			}
			appendGeminiContentPart(geminiRequest, "user", part)
			for _, part := range media {
				appendGeminiContentPart(geminiRequest, "user", part)
			}
		default:
			role := responsesGeminiRole(item)
			parts, err := responsesInputContentToGeminiParts(c, item["content"])
			if err != nil {
				return nil, err
			}
			if role == "system" {
				for _, part := range parts {
					if part.Text != "" {
						systemTexts = append(systemTexts, part.Text)
					}
				}
				continue
			}
			if len(parts) > 0 {
				geminiRequest.Contents = append(geminiRequest.Contents, dto.GeminiChatContent{
					Role:  role,
					Parts: parts,
				})
			}
		}
	}

	if len(systemTexts) > 0 {
		geminiRequest.SystemInstructions = &dto.GeminiChatContent{
			Parts: []dto.GeminiPart{{Text: strings.Join(systemTexts, "\n")}},
		}
	}

	return geminiRequest, nil
}

func applyResponsesTextToGemini(ctx context.Context, raw []byte, req *dto.GeminiChatRequest, supportsJSONSchema bool) error {
	if !RawJSONPresent(raw) {
		return nil
	}
	var text struct {
		Format json.RawMessage `json:"format"`
	}
	if err := kitutil.Unmarshal(raw, &text); err != nil {
		return fmt.Errorf("invalid text config: %w", err)
	}
	if !RawJSONPresent(text.Format) {
		return nil
	}
	var format struct {
		Type string `json:"type"`
	}
	if err := kitutil.Unmarshal(text.Format, &format); err != nil {
		return err
	}
	if format.Type == "json_schema" || format.Type == "json_object" {
		req.GenerationConfig.ResponseMimeType = "application/json"
		if format.Type == "json_schema" {
			sharedgemini.ApplyResponseSchema(ctx, &req.GenerationConfig, text.Format, "text.format.schema", supportsJSONSchema)
		}
	}
	return nil
}

func responsesInputContentToGeminiParts(c context.Context, content any) ([]dto.GeminiPart, error) {
	contentParts, err := ContentParts(content)
	if err != nil {
		return nil, err
	}

	parts := make([]dto.GeminiPart, 0, len(contentParts))
	for _, contentPart := range contentParts {
		nextParts, err := responsesContentPartToGeminiParts(c, contentPart)
		if err != nil {
			return nil, err
		}
		parts = append(parts, nextParts...)
	}
	return parts, nil
}

func responsesContentPartToGeminiParts(c context.Context, part map[string]any) ([]dto.GeminiPart, error) {
	partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
	switch partType {
	case "input_text", "output_text", "text":
		text := kitutil.Interface2String(part["text"])
		if text == "" {
			return nil, nil
		}
		return []dto.GeminiPart{{Text: text}}, nil
	case "input_image", "input_file", "input_audio", "input_video":
		source := ContentPartToFileSource(part)
		if source == nil {
			return nil, nil
		}
		base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Responses input for Gemini")
		if err != nil {
			return nil, fmt.Errorf("get file data from '%s' failed: %w", source.GetIdentifier(), err)
		}
		if !sharedgemini.IsSupportedMimeType(mimeType) {
			return nil, fmt.Errorf("mime type is not supported by Gemini: '%s', url: '%s', supported types are: %v", mimeType, source.GetIdentifier(), sharedgemini.SupportedMimeTypesList())
		}
		return []dto.GeminiPart{
			{
				InlineData: &dto.GeminiInlineData{
					MimeType: mimeType,
					Data:     base64Data,
				},
			},
		}, nil
	default:
		return nil, nil
	}
}

func responsesFunctionCallItemToGeminiPart(item map[string]any) (dto.GeminiPart, string, error) {
	name := convmeta.NamespacedToolName(strings.TrimSpace(kitutil.Interface2String(item["namespace"])), strings.TrimSpace(kitutil.Interface2String(item["name"])))
	if name == "" {
		return dto.GeminiPart{}, "", fmt.Errorf("function_call item is missing name")
	}
	callID := CallID(item)
	arguments := ObjectValue(item["arguments"], "arguments")
	if item["type"] == "custom_tool_call" {
		arguments = map[string]any{convmeta.CustomToolInputArgument: kitutil.Interface2String(item["input"])}
	}
	return dto.GeminiPart{
		FunctionCall: &dto.FunctionCall{
			ID:           callID,
			FunctionName: name,
			Arguments:    arguments,
		},
	}, callID, nil
}

func responsesFunctionOutputItemToGeminiPart(c context.Context, item map[string]any, callNames map[string]string, mediaInParts bool) (dto.GeminiPart, []dto.GeminiPart, error) {
	callID := CallID(item)
	name := convmeta.NamespacedToolName(strings.TrimSpace(kitutil.Interface2String(item["namespace"])), strings.TrimSpace(kitutil.Interface2String(item["name"])))
	if name == "" {
		name = callNames[callID]
	}
	output, media := responsesToolOutputToGemini(c, item["output"], mediaInParts)
	response := &dto.GeminiFunctionResponse{
		Name:     name,
		Response: output,
	}
	if mediaInParts && len(media) > 0 {
		parts, err := kitutil.Marshal(media)
		if err != nil {
			return dto.GeminiPart{}, nil, fmt.Errorf("failed to marshal function response parts: %w", err)
		}
		response.Parts = parts
		media = nil
	}
	if callID != "" {
		id, err := kitutil.Marshal(callID)
		if err != nil {
			return dto.GeminiPart{}, nil, fmt.Errorf("failed to marshal function response ID: %w", err)
		}
		response.ID = id
	}
	return dto.GeminiPart{
		FunctionResponse: response,
	}, media, nil
}

// responsesToolOutputToGemini maps a function_call_output payload onto a Gemini
// functionResponse. A Responses content-part array keeps its text in response
// and returns its media as inlineData parts; stringifying them instead would
// hand base64 media to the upstream text tokenizer. Multimodal function
// responses (mediaInParts) carry images and documents only; separate parts
// carry every type Gemini accepts. Media that cannot be loaded or carried is
// omitted with a diagnostic. An array holding any other part type, and every
// other payload shape, keeps the historical response map.
func responsesToolOutputToGemini(c context.Context, value any, mediaInParts bool) (map[string]any, []dto.GeminiPart) {
	rawParts, ok := value.([]any)
	if !ok || len(rawParts) == 0 {
		return GeminiResponseMap(value), nil
	}
	parts := make([]map[string]any, 0, len(rawParts))
	for _, rawPart := range rawParts {
		part, isMap := rawPart.(map[string]any)
		if !isMap {
			return GeminiResponseMap(value), nil
		}
		switch strings.TrimSpace(kitutil.Interface2String(part["type"])) {
		case "input_text", "output_text", "text", "input_image", "input_file", "input_audio", "input_video":
		default:
			return GeminiResponseMap(value), nil
		}
		parts = append(parts, part)
	}

	texts := make([]string, 0, len(parts))
	labels := make([]string, 0, len(parts))
	media := make([]dto.GeminiPart, 0, len(parts))
	for _, part := range parts {
		partType := strings.TrimSpace(kitutil.Interface2String(part["type"]))
		switch partType {
		case "input_text", "output_text", "text":
			if text := kitutil.Interface2String(part["text"]); text != "" {
				texts = append(texts, text)
			}
			continue
		}
		kind := strings.TrimPrefix(partType, "input_")
		if label := "[" + kind + "]"; !slices.Contains(labels, label) {
			labels = append(labels, label)
		}
		source := ContentPartToFileSource(part)
		if source == nil {
			continue
		}
		// The documented MIME list for multimodal function responses has no
		// audio or video, and Gemini 3 ignores audio sent there.
		reason := fmt.Sprintf("Gemini function responses cannot carry %s content", kind)
		if !mediaInParts || kind == "image" || kind == "file" {
			base64Data, mimeType, err := relaymedia.ResolveBase64Data(c, source, "formatting Responses tool output for Gemini")
			topLevelType, _, _ := strings.Cut(strings.ToLower(mimeType), "/")
			switch {
			case err != nil:
				reason = fmt.Sprintf("the %s could not be loaded", kind)
			case sharedgemini.IsSupportedMimeType(mimeType) && (!mediaInParts || topLevelType != "audio" && topLevelType != "video"):
				media = append(media, dto.GeminiPart{InlineData: &dto.GeminiInlineData{MimeType: mimeType, Data: base64Data}})
				continue
			default:
				reason = fmt.Sprintf("Gemini function responses cannot carry %s content", mimeType)
			}
		}
		convdiag.Add(c, types.ConversionDiagnostic{
			Code:     "unsupported_media_type",
			Path:     "input.output",
			Message:  reason + "; the part was omitted",
			Severity: types.ConversionDiagnosticError,
		})
	}

	switch {
	case len(texts) > 0:
		return GeminiResponseMap(strings.Join(texts, "\n")), media
	case len(media) > 0:
		return map[string]any{}, media
	default:
		// Every media part was omitted; name what was there instead of
		// sending the payload as text.
		return GeminiResponseMap(strings.Join(labels, " ")), nil
	}
}

func appendGeminiContentPart(req *dto.GeminiChatRequest, role string, part dto.GeminiPart) {
	if len(req.Contents) > 0 && req.Contents[len(req.Contents)-1].Role == role {
		parts := req.Contents[len(req.Contents)-1].Parts
		insertAt := len(parts)
		switch {
		case role == "model" && part.FunctionCall != nil:
			insertAt = 0
			for insertAt < len(parts) && parts[insertAt].FunctionCall != nil {
				insertAt++
			}
		case part.FunctionResponse != nil:
			// Tool media sent as separate parts stay after every function
			// response of the turn, so a later response goes before them.
			for i := len(parts) - 1; i >= 0; i-- {
				if parts[i].FunctionResponse != nil {
					insertAt = i + 1
					break
				}
			}
		}
		req.Contents[len(req.Contents)-1].Parts = slices.Insert(parts, insertAt, part)
		return
	}
	req.Contents = append(req.Contents, dto.GeminiChatContent{
		Role:  role,
		Parts: []dto.GeminiPart{part},
	})
}

func responsesGeminiRole(item map[string]any) string {
	switch strings.TrimSpace(kitutil.Interface2String(item["role"])) {
	case "assistant":
		return "model"
	case "system", "developer":
		return "system"
	case "model":
		return "model"
	default:
		return "user"
	}
}
