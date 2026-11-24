package oaichat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/internal/convdiag"
	kitutil "github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Seed output_config before reasoning is applied; its effort setter preserves
// other members. Keep schema raw so numbers and property order survive.
func claudeOutputConfigForChatFormat(ctx context.Context, format *dto.ResponseFormat) (json.RawMessage, error) {
	if format == nil || format.Type == "text" {
		return nil, nil
	}
	if format.Type == "json_schema" {
		var spec struct {
			Schema json.RawMessage `json:"schema"`
		}
		if err := kitutil.Unmarshal(format.JsonSchema, &spec); err == nil && kitutil.GetJsonType(spec.Schema) == "object" {
			return kitutil.Marshal(map[string]any{"format": map[string]any{"type": "json_schema", "schema": spec.Schema}})
		}
	}
	convdiag.Add(ctx, types.ConversionDiagnostic{Code: "unsupported_response_format", Path: "response_format",
		Message:  fmt.Sprintf("Claude requires an object schema in output_config.format; cannot preserve Chat response format %q", format.Type),
		Severity: types.ConversionDiagnosticError})
	return nil, nil
}
