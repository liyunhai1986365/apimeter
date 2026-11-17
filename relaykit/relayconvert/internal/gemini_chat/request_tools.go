package geminichat

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
	"strings"
)

func convertGeminiToolsToChat(ctx context.Context, source *dto.GeminiChatRequest, target *dto.GeneralOpenAIRequest) error {
	var groups []map[string]json.RawMessage
	if len(source.Tools) > 0 {
		if err := kitutil.Unmarshal(source.Tools, &groups); err != nil {
			return err
		}
	}
	for _, group := range groups {
		for key := range group {
			if key != "functionDeclarations" {
				convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_hosted_tool", Path: "tools", Message: fmt.Sprintf("Chat cannot represent Gemini tool %q", key), Severity: types.ConversionDiagnosticError})
			}
		}
		var funcs []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Parameters  json.RawMessage `json:"parameters"`
			JSONSchema  json.RawMessage `json:"parametersJsonSchema"`
		}
		if raw := group["functionDeclarations"]; len(raw) > 0 {
			if err := kitutil.Unmarshal(raw, &funcs); err != nil {
				return err
			}
		}
		for _, f := range funcs {
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
	if source.ToolConfig != nil && source.ToolConfig.FunctionCallingConfig != nil {
		config := source.ToolConfig.FunctionCallingConfig
		switch strings.ToUpper(string(config.Mode)) {
		case "AUTO":
			target.ToolChoice = "auto"
		case "NONE":
			target.ToolChoice = "none"
		case "ANY":
			switch len(config.AllowedFunctionNames) {
			case 0:
				target.ToolChoice = "required"
			case 1:
				target.ToolChoice = map[string]any{"type": "function", "function": map[string]any{"name": config.AllowedFunctionNames[0]}}
			default:
				target.ToolChoice = "required"
				convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_tool_subset", Path: "toolConfig", Message: "Chat cannot enforce this function name subset", Severity: types.ConversionDiagnosticError})
			}
		default:
			convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_tool_choice", Path: "toolConfig", Message: "Chat cannot represent this Gemini tool mode", Severity: types.ConversionDiagnosticError})
		}
	}
	return nil
}
