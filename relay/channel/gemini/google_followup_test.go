package gemini

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGoogleEmbeddingRejectsPartialBatch(t *testing.T) {
	ctx, _, info := googleTestContext(t)
	a := &Adaptor{}
	for _, input := range []any{[]any{"text", map[string]any{"type": "image"}}, []any{"text", 0}, []any{"text", nil}} {
		_, err := a.ConvertEmbeddingRequest(ctx, info, dto.EmbeddingRequest{Input: input})
		var apiErr *types.NewAPIError
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, 400, apiErr.StatusCode)
	}
	_, err := a.ConvertEmbeddingRequest(ctx, info, dto.EmbeddingRequest{Input: []any{"one", "two"}})
	require.NoError(t, err)
}
func TestGoogleResponsesHostIncludesSearchEvents(t *testing.T) {
	payload := `{"candidates":[{"content":{"parts":[{"text":"answer"}]},"finishReason":"STOP","groundingMetadata":{"webSearchQueries":["cat","cat"]}}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5}}`
	ctx, w, info := googleTestContext(t)
	_, apiErr := GeminiResponsesHandler(ctx, info, &http.Response{Body: io.NopCloser(strings.NewReader(payload))})
	require.Nil(t, apiErr)
	var out dto.OpenAIResponsesResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Output, 2)
	require.Equal(t, "web_search_call", out.Output[1].Type)
	ctx, w, info = googleTestContext(t)
	_, apiErr = GeminiResponsesStreamHandler(ctx, info, &http.Response{Body: io.NopCloser(strings.NewReader("data: " + payload + "\n\ndata: [DONE]\n\n"))})
	require.Nil(t, apiErr)
	body := w.Body.String()
	require.Contains(t, body, "event: response.web_search_call.completed")
	require.Less(t, strings.Index(body, "event: response.web_search_call.completed"), strings.Index(body, "event: response.completed"))
}

func TestGoogleImagenRejectsEmptyOutputs(t *testing.T) {
	for _, payload := range []string{
		`{"predictions":[]}`,
		`{"predictions":[{"raiFilteredReason":"filtered"}]}`,
		`{"predictions":[{}, {"bytesBase64Encoded":"  "}]}`,
	} {
		ctx, w, info := googleTestContext(t)
		usage, apiErr := GeminiImageHandler(ctx, info, &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(payload)),
		})
		require.NotNil(t, apiErr)
		require.Nil(t, usage)
		require.Nil(t, info.BillingImageCount)
		require.Empty(t, w.Body.String())
	}
}

func TestGoogleImagenTokenPricingDoesNotMultiplyByImageCount(t *testing.T) {
	ctx, w, info := googleTestContext(t)
	usage, apiErr := GeminiImageHandler(ctx, info, &http.Response{
		StatusCode: http.StatusOK,
		Body: io.NopCloser(strings.NewReader(`{"predictions":[
			{"bytesBase64Encoded":"aW1hZ2Ux"}, {},
			{"bytesBase64Encoded":"aW1hZ2Uy"}
		]}`)),
	})
	require.Nil(t, apiErr)
	require.Equal(t, 2, *info.BillingImageCount)
	require.Equal(t, 516, usage.TotalTokens)
	require.NotContains(t, info.PriceData.OtherRatios, "n")
	var response dto.ImageResponse
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data, 2)
}
