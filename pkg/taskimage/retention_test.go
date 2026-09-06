package taskimage

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestTransformImagePayloadsPreservesOtherData(t *testing.T) {
	body := []byte(`{"id":"task-a","usage":{"tokens":9007199254740993},"metadata":{"keep":true},"data":{"images":[{"b64_json":"IMAGE_ONE","file_name":"one.png"},{"url":"https://example.com/two.png"},{"url":"data:image/png;base64,IMAGE_THREE"}]},"audio":{"base64":"AUDIO_KEEP"},"video":{"url":"data:video/mp4;base64,VIDEO_KEEP"}}`)
	out, summary, err := Transform(body, true, true)
	require.NoError(t, err)
	require.Equal(t, 2, summary.Base64)
	require.Positive(t, summary.URLs)
	require.NotContains(t, string(out), "IMAGE_ONE")
	require.NotContains(t, string(out), "IMAGE_THREE")
	for _, value := range []string{"9007199254740993", "one.png", "https://example.com/two.png", "AUDIO_KEEP", "VIDEO_KEEP", `"image_expired":true`} {
		require.Contains(t, string(out), value)
	}
	again, _, err := Transform(out, true, true)
	require.NoError(t, err)
	require.Equal(t, out, again)
	unchanged, _, err := Transform(body, true, false)
	require.NoError(t, err)
	require.Equal(t, body, unchanged)
}

func TestTransformImageVariants(t *testing.T) {
	for _, body := range []string{
		`{"data":[{"b64_json":"SECRET"}]}`,
		`{"output":{"images":[{"base64":"SECRET"},{"image_base64":"SECRET"},{"imageBase64":"SECRET"}]}}`,
		`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"SECRET"}},{"text":"keep text"}]}}]}`,
		`{"contents":[{"parts":[{"inline_data":{"mime_type":"image/jpeg","data":"SECRET"}}]}]}`,
		`{"messages":[{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,SECRET"}}]}]}`,
		`{"images":["data:image/png;base64,SECRET","https://example.com/keep.png"]}`,
	} {
		t.Run(body, func(t *testing.T) {
			out, summary, err := Transform([]byte(body), true, true)
			require.NoError(t, err)
			require.Positive(t, summary.Base64)
			require.NotContains(t, string(out), "SECRET")
			var valid any
			require.NoError(t, common.Unmarshal(out, &valid))
		})
	}
}

func TestTransformDoesNotTouchNonImageInlineOrOpaqueData(t *testing.T) {
	body := []byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"audio/wav","data":"AUDIO"}},{"inlineData":{"mimeType":"video/mp4","data":"VIDEO"}}]}}],"thinking":{"signature":"OPAQUE","data":"keep"},"data":{"text":"keep","number":123}}`)
	out, summary, err := Transform(body, false, true)
	require.NoError(t, err)
	require.Equal(t, body, out)
	require.Zero(t, summary.Base64)
	_, _, err = Transform([]byte(`{"data":`), true, true)
	require.Error(t, err)
}

func TestTransformPreservesOpaqueBase64Metadata(t *testing.T) {
	body := []byte(`{"data":{"images":[{"b64_json":"REMOVE"}]},"metadata":{"base64":"KEEP_METADATA"},"usage":{"b64_json":"KEEP_USAGE"},"prompt":"data:image/png;base64,KEEP_PROMPT"}`)
	out, _, err := Transform(body, true, true)
	require.NoError(t, err)
	require.NotContains(t, string(out), "REMOVE")
	for _, keep := range []string{"KEEP_METADATA", "KEEP_USAGE", "KEEP_PROMPT"} {
		require.Contains(t, string(out), keep)
	}
}
