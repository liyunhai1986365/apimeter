package geminichat

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func convertGeminiToolsToChat(ctx context.Context, source *dto.GeminiChatRequest, target *dto.GeneralOpenAIRequest) error {
	groups, err := dto.ParseGeminiTools[map[string]json.RawMessage](source.Tools)
	if err != nil {
		return err
	}
	var mode string
	var allowedNames []string
	if cfg := source.ToolConfig; cfg != nil && cfg.FunctionCallingConfig != nil {
		mode = strings.ToUpper(string(cfg.FunctionCallingConfig.Mode))
		if mode == "ANY" {
			allowedNames = cfg.FunctionCallingConfig.AllowedFunctionNames
		}
	}
	for groupIndex, group := range groups {
		for key := range group {
			if key != "functionDeclarations" {
				convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_hosted_tool", Path: "tools", Message: fmt.Sprintf("Chat cannot represent Gemini tool %q", key), Severity: types.ConversionDiagnosticError})
			}
		}
		var funcs []struct {
			Name           string           `json:"name"`
			Description    string           `json:"description"`
			Parameters     json.RawMessage  `json:"parameters"`
			JSONSchema     json.RawMessage  `json:"parametersJsonSchema"`
			Response       *json.RawMessage `json:"response"`
			ResponseSchema *json.RawMessage `json:"responseJsonSchema"`
		}
		if raw := group["functionDeclarations"]; len(raw) > 0 {
			if err := kitutil.Unmarshal(raw, &funcs); err != nil {
				return err
			}
		}
		for index, f := range funcs {
			selectable := mode != "NONE" && (len(allowedNames) != 1 || allowedNames[0] == f.Name)
			if selectable && (f.Response != nil || f.ResponseSchema != nil) {
				convdiag.Add(ctx, types.ConversionDiagnostic{
					Code: "unsupported_function_response_schema", Path: fmt.Sprintf("tools[%d].functionDeclarations[%d]", groupIndex, index),
					Message: fmt.Sprintf("Chat cannot declare a return schema for function %q", f.Name), Severity: types.ConversionDiagnosticError,
				})
			}
			function := dto.FunctionRequest{Name: f.Name, Description: f.Description}
			raw := f.JSONSchema
			if len(f.Parameters) > 0 && string(f.Parameters) != "null" {
				if len(raw) > 0 && string(raw) != "null" {
					return fmt.Errorf("function %q declares both schema fields", f.Name)
				}
				var schema any
				if err := kitutil.Unmarshal(f.Parameters, &schema); err != nil {
					return err
				}
				converted, loss := sharedgemini.OpenAPISchemaToJSONSchema(schema)
				raw, _ = kitutil.Marshal(converted)
				if len(loss) > 0 {
					convdiag.Add(ctx, types.ConversionDiagnostic{Code: "gemini_schema_keyword_dropped", Path: "tools", Message: strings.Join(loss, ", "), Severity: types.ConversionDiagnosticWarning})
				}
			}
			if len(raw) > 0 {
				if err := kitutil.Unmarshal(raw, &function.Parameters); err != nil {
					return err
				}
				function.RawParameters = raw
			}
			target.Tools = append(target.Tools, dto.ToolCallRequest{Type: "function", Function: function})
		}
	}
	switch mode {
	case "", "MODE_UNSPECIFIED":
		// Omit tool_choice to use Chat's default for the supplied tools.
	case "AUTO":
		target.ToolChoice = "auto"
	case "NONE":
		target.ToolChoice = "none"
	case "ANY":
		switch len(allowedNames) {
		case 0:
			target.ToolChoice = "required"
		case 1:
			target.ToolChoice = map[string]any{"type": "function", "function": map[string]any{"name": allowedNames[0]}}
		default:
			target.ToolChoice = "required"
			if !sameFunctionNames(allowedNames, target.Tools) {
				convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_tool_subset", Path: "toolConfig", Message: "Chat cannot enforce this function name subset", Severity: types.ConversionDiagnosticError})
			}
		}
	default:
		convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_tool_choice", Path: "toolConfig", Message: "Chat cannot represent this Gemini tool mode", Severity: types.ConversionDiagnosticError})
	}
	return nil
}

func sameFunctionNames(allowed []string, tools []dto.ToolCallRequest) bool {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	declaredSet := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		declaredSet[tool.Function.Name] = struct{}{}
	}
	return maps.Equal(allowedSet, declaredSet)
}
