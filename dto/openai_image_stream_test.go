package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestImageRequestPreservesSeedreamFieldsAndExplicitFalse(t *testing.T) {
	var request ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{
		"model":"doubao-seedream-5-0-260128",
		"prompt":"draw",
		"stream":false,
		"watermark":false,
		"sequential_image_generation":"auto",
		"sequential_image_generation_options":{"max_images":4},
		"tools":[{"type":"web_search"}],
		"optimize_prompt_options":{"mode":"standard"}
	}`), &request))

	body, err := common.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"model":"doubao-seedream-5-0-260128",
		"prompt":"draw",
		"stream":false,
		"watermark":false,
		"sequential_image_generation":"auto",
		"sequential_image_generation_options":{"max_images":4},
		"tools":[{"type":"web_search"}],
		"optimize_prompt_options":{"mode":"standard"}
	}`, string(body))
}

func TestImageRequestDoesNotEnableStreamingForOtherImageModels(t *testing.T) {
	var request ImageRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-image-2","prompt":"draw","stream":true}`), &request))
	require.False(t, request.IsStream(nil))
}
