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

func TestImageRequestStreamFlag(t *testing.T) {
	for _, model := range []string{"gpt-image-1", "gpt-image-1.5", "gpt-image-2", "dall-e-3", "custom-image-alias", "doubao-seedream-5-0-260128"} {
		for _, stream := range []*bool{nil, common.GetPointer(false), common.GetPointer(true)} {
			request := ImageRequest{Model: model, Stream: stream}
			require.Equal(t, stream != nil && *stream, request.IsStream(nil), model)
		}
	}
}
