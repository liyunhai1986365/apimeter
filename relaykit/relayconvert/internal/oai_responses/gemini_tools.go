package oairesponses

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	sharedgemini "github.com/QuantumNous/new-api/relaykit/relayconvert/internal/shared/gemini"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

func responsesToolsToGemini(ctx context.Context, raw json.RawMessage, supportsJSONSchema, disabled bool) ([]dto.GeminiChatTool, *convmeta.ResponsesToolState, error) {
	state := &convmeta.ResponsesToolState{CustomToolNames: make(map[string]struct{}), Namespaces: make(map[string]string), FunctionNames: make(map[string]struct{})}
	if !RawJSONPresent(raw) {
		return nil, state, nil
	}
	var tools []json.RawMessage
	if err := kitutil.Unmarshal(raw, &tools); err != nil {
		return nil, nil, fmt.Errorf("invalid tools: %w", err)
	}
	// Disabled declarations are retained for interpreting tool history. Their
	// execution constraints cannot be violated under NONE, so do not diagnose
	// schema/grammar/namespace adaptation. Keep JSON validation and ambiguous
	// name diagnostics, which still affect the interpretation of history.
	declarationCtx := ctx
	if disabled {
		declarationCtx = nil
	}
	var err error
	tools, namespaces, err := flattenResponsesTools(declarationCtx, tools)
	if err != nil {
		return nil, nil, err
	}
	// A declared function wins over a custom tool with the same name.
	functionNames := make(map[string]struct{})
	for _, rawTool := range tools {
		var h struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if err := kitutil.Unmarshal(rawTool, &h); err != nil {
			return nil, nil, err
		}
		if h.Type == "function" {
			functionNames[h.Name] = struct{}{}
		}
	}
	seen := make(map[string]struct{})
	searchAdded := false
	var declarations []map[string]any
	var hosted []dto.GeminiChatTool
	for toolIndex, rawTool := range tools {
		var header struct {
			Type        string          `json:"type"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Format      json.RawMessage `json:"format"`
		}
		if err := kitutil.Unmarshal(rawTool, &header); err != nil {
			return nil, nil, err
		}
		if header.Type == "function" || header.Type == "custom" {
			if strings.TrimSpace(header.Name) == "" {
				return nil, nil, fmt.Errorf("tool is missing name")
			}
			_, duplicate := seen[header.Name]
			_, functionConflict := functionNames[header.Name]
			if duplicate || header.Type == "custom" && functionConflict {
				convdiag.Add(ctx, types.ConversionDiagnostic{Code: "tool_name_conflict", Path: "tools", Message: fmt.Sprintf("duplicate tool %q was omitted", header.Name), Severity: types.ConversionDiagnosticError})
				continue
			}
			seen[header.Name] = struct{}{}
			state.FunctionNames[header.Name] = struct{}{}
			state.Namespaces[header.Name] = namespaces[toolIndex]
		}
		switch header.Type {
		case "function":
			var function dto.FunctionRequest
			if err := kitutil.Unmarshal(rawTool, &function); err != nil {
				return nil, nil, err
			}
			declarations = append(declarations, sharedgemini.FunctionDeclaration(declarationCtx, function, supportsJSONSchema))
		case "custom":
			if strings.TrimSpace(header.Name) == "" {
				return nil, nil, fmt.Errorf("custom tool is missing name")
			}
			description := header.Description
			if RawJSONPresent(header.Format) {
				description += "\nTool input format: " + string(header.Format)
			}
			declarations = append(declarations, sharedgemini.FunctionDeclaration(declarationCtx, dto.FunctionRequest{
				Name: header.Name, Description: description,
				Parameters: map[string]any{"type": "object", "properties": map[string]any{convmeta.CustomToolInputArgument: map[string]any{"type": "string"}}, "required": []any{convmeta.CustomToolInputArgument}},
			}, supportsJSONSchema))
			state.CustomToolNames[header.Name] = struct{}{}
			convdiag.Add(declarationCtx, types.ConversionDiagnostic{Code: "custom_tool_as_function", Path: "tools", Message: "Custom tool input is encoded as a string argument; grammar constraints cannot be enforced", Severity: types.ConversionDiagnosticWarning})
		case "web_search", "web_search_preview", "google_search":
			if disabled {
				continue
			}
			if !searchAdded {
				hosted = append(hosted, dto.GeminiChatTool{GoogleSearch: map[string]string{}})
				searchAdded = true
			}
			diagnoseGeminiSearchLoss(ctx, rawTool)

		default:
			convdiag.Add(declarationCtx, types.ConversionDiagnostic{Code: "unsupported_hosted_tool", Path: "tools", Message: fmt.Sprintf("Responses tool %q has no supported Gemini mapping; omitted", header.Type), Severity: types.ConversionDiagnosticError})
		}
	}
	if len(declarations) > 0 {
		hosted = append(hosted, dto.GeminiChatTool{FunctionDeclarations: declarations})
	}
	return hosted, state, nil
}

// Keep each member's raw parameters so namespace flattening does not sort schemas.
func flattenResponsesTools(ctx context.Context, tools []json.RawMessage) ([]json.RawMessage, []string, error) {
	var out []json.RawMessage
	var namespaces []string
	for _, raw := range tools {
		var header struct {
			Type        string            `json:"type"`
			Name        string            `json:"name"`
			Description string            `json:"description"`
			Tools       []json.RawMessage `json:"tools"`
		}
		if err := kitutil.Unmarshal(raw, &header); err != nil {
			return nil, nil, err
		}
		if header.Type != "namespace" {
			out = append(out, raw)
			namespaces = append(namespaces, "")
			continue
		}
		ns := strings.TrimSpace(header.Name)
		if ns == "" {
			return nil, nil, fmt.Errorf("namespace tool requires name")
		}
		if ns != convmeta.DefaultResponsesToolNamespace {
			convdiag.Add(ctx, types.ConversionDiagnostic{Code: "tool_namespace_flattened", Path: "tools", Message: "Namespaced tools are encoded as functions with qualified names", Severity: types.ConversionDiagnosticWarning})
		}
		for _, member := range header.Tools {
			var fields map[string]json.RawMessage
			if err := kitutil.Unmarshal(member, &fields); err != nil {
				return nil, nil, err
			}
			var name, description, kind string
			_ = kitutil.Unmarshal(fields["name"], &name)
			_ = kitutil.Unmarshal(fields["description"], &description)
			_ = kitutil.Unmarshal(fields["type"], &kind)
			if kind != "function" && kind != "custom" {
				return nil, nil, fmt.Errorf("namespace %q accepts only function and custom tools", ns)
			}
			if strings.TrimSpace(name) == "" {
				return nil, nil, fmt.Errorf("namespace member requires name")
			}
			fields["name"], _ = kitutil.Marshal(convmeta.NamespacedToolName(ns, name))
			if header.Description != "" {
				fields["description"], _ = kitutil.Marshal(strings.TrimSpace(header.Description + "\n\n" + description))
			}
			encoded, err := kitutil.Marshal(fields)
			if err != nil {
				return nil, nil, err
			}
			out = append(out, encoded)
			namespaces = append(namespaces, ns)
		}
	}
	return out, namespaces, nil
}

func responsesToolChoiceToGemini(ctx context.Context, raw json.RawMessage, state *convmeta.ResponsesToolState) (*dto.ToolConfig, error) {
	if !RawJSONPresent(raw) {
		return nil, nil
	}
	var choice any
	if err := kitutil.Unmarshal(raw, &choice); err != nil {
		return nil, err
	}
	if object, ok := choice.(map[string]any); ok {
		kind := kitutil.Interface2String(object["type"])
		if kind == "function" || kind == "custom" {
			name := convmeta.NamespacedToolName(kitutil.Interface2String(object["namespace"]), kitutil.Interface2String(object["name"]))
			_, exists := state.FunctionNames[name]
			if !exists || kind == "custom" && !state.IsCustomTool(name) || kind == "function" && state.IsCustomTool(name) {
				return nil, fmt.Errorf("tool_choice references unavailable %s tool %q", kind, name)
			}
			choice = map[string]any{"type": "function", "function": map[string]any{"name": name}}
		}
	}
	if len(state.FunctionNames) == 0 {
		// Automatic selection is Gemini's default, including hosted search
		// and requests without tools. No functionCallingConfig is needed.
		// For none, the caller also removes hosted tools before sending.
		if choice == "auto" || choice == "none" {
			return nil, nil
		}
		convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_tool_choice", Path: "tool_choice", Message: "Gemini cannot enforce this choice without function declarations", Severity: types.ConversionDiagnosticError})
		return nil, nil
	}
	config := sharedgemini.OpenAIToolChoiceToConfig(choice)
	if config == nil {
		convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_tool_choice", Path: "tool_choice", Message: "Gemini cannot represent this tool choice", Severity: types.ConversionDiagnosticError})
	}
	return config, nil
}

func isResponsesToolChoiceNone(raw json.RawMessage) bool {
	var choice string
	return kitutil.Unmarshal(raw, &choice) == nil && choice == "none"
}

func diagnoseGeminiSearchLoss(ctx context.Context, raw json.RawMessage) {
	var fields map[string]json.RawMessage
	if kitutil.Unmarshal(raw, &fields) != nil {
		return
	}
	for key, value := range fields {
		if key == "type" || !RawJSONPresent(value) {
			continue
		}
		severity := types.ConversionDiagnosticWarning
		code := "unsupported_search_tuning"
		switch key {
		case "filters":
			var filters struct {
				Allowed []string `json:"allowed_domains"`
				Blocked []string `json:"blocked_domains"`
			}
			if kitutil.Unmarshal(value, &filters) == nil && len(filters.Allowed) == 0 && len(filters.Blocked) == 0 {
				continue
			}
			severity = types.ConversionDiagnosticError
			code = "unsupported_search_constraints"
		case "external_web_access", "max_uses", "allowed_callers":
			severity = types.ConversionDiagnosticError
			code = "unsupported_search_constraints"
		}
		convdiag.Add(ctx, types.ConversionDiagnostic{Code: code, Path: "tools." + key, Message: "Gemini Google Search cannot preserve this source search option", Severity: severity})
	}
}
