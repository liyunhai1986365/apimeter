package oairesponses

import "github.com/QuantumNous/new-api/relaykit/dto"

const ResponsesInputTypeCustomToolCallOutput = "custom_tool_call_output"

// Keep the original bytes, including JSON Schema property order and tool history.
func PrepareOpenAIResponsesRequest(request dto.OpenAIResponsesRequest) (dto.OpenAIResponsesRequest, error) {
	return request, nil
}
