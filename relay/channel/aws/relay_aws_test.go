package aws

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestConvertAwsBedrockRequestBody_LeavesCompliantBodyUnchanged(t *testing.T) {
	t.Parallel()

	body := []byte(`{"anthropic_version":"bedrock-2023-05-31","anthropic_beta":["test-beta"],"max_tokens":24,"system":"be helpful","messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aW1hZ2U="}}]}],"custom_field":{"keep":true}}`)

	converted, changed, err := convertAwsBedrockRequestBody(nil, body, http.Header{})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, converted)
}

func TestConvertAwsBedrockRequestBody_LeavesAbsentOptionalBetaUnchanged(t *testing.T) {
	t.Parallel()

	body := []byte(`{"anthropic_version":"bedrock-2023-05-31","max_tokens":24,"messages":[{"role":"user","content":"hello"}]}`)

	converted, changed, err := convertAwsBedrockRequestBody(nil, body, http.Header{})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, converted)
}

func TestConvertAwsBedrockRequestBody_AppliesOnlyRequiredCompatibilityFixes(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("anthropic-beta", "beta-one, beta-two")
	body := []byte(`{"model":"claude-opus-4-8","stream":true,"max_tokens":24,"messages":[{"role":"system","content":"be exact"},{"role":"user","content":"hello"}],"custom_field":{"keep":true}}`)

	converted, changed, err := convertAwsBedrockRequestBody(nil, body, header)
	require.NoError(t, err)
	require.True(t, changed)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(converted, &payload))
	require.Equal(t, awsBedrockAnthropicVersion, payload["anthropic_version"])
	require.Equal(t, "be exact", payload["system"])
	require.Equal(t, []any{"beta-one", "beta-two"}, payload["anthropic_beta"])
	require.NotContains(t, payload, "model")
	require.NotContains(t, payload, "stream")
	require.Equal(t, map[string]any{"keep": true}, payload["custom_field"])

	messages, ok := payload["messages"].([]any)
	require.True(t, ok)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].(map[string]any)["role"])
}

func TestConvertAwsBedrockRequestBody_MergesExistingSystemPrompt(t *testing.T) {
	t.Parallel()

	body := []byte(`{"anthropic_version":"bedrock-2023-05-31","system":"existing","messages":[{"role":"system","content":[{"type":"text","text":"leading","cache_control":{"type":"ephemeral"}}]},{"role":"user","content":"hello"}]}`)

	converted, changed, err := convertAwsBedrockRequestBody(nil, body, http.Header{})
	require.NoError(t, err)
	require.True(t, changed)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(converted, &payload))
	systemBlocks, ok := payload["system"].([]any)
	require.True(t, ok)
	require.Len(t, systemBlocks, 2)
	require.Equal(t, "existing", systemBlocks[0].(map[string]any)["text"])
	require.Equal(t, "leading", systemBlocks[1].(map[string]any)["text"])
}

func TestConvertAwsBedrockRequestBody_ConvertsOnlyURLMediaSources(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	body := []byte(`{"anthropic_version":"bedrock-2023-05-31","messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"https://example.com/image.png"}},{"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"already-base64"}}]}]}`)
	converted, changed, err := convertAwsBedrockRequestBodyWithURLLoader(ctx, body, http.Header{}, func(_ *gin.Context, sourceURL string) (string, string, error) {
		require.Equal(t, "https://example.com/image.png", sourceURL)
		return "encoded-image", "image/png", nil
	})
	require.NoError(t, err)
	require.True(t, changed)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(converted, &payload))
	messages := payload["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)
	convertedSource := content[0].(map[string]any)["source"].(map[string]any)
	require.Equal(t, "base64", convertedSource["type"])
	require.Equal(t, "image/png", convertedSource["media_type"])
	require.Equal(t, "encoded-image", convertedSource["data"])
	require.NotContains(t, convertedSource, "url")

	compliantSource := content[1].(map[string]any)["source"].(map[string]any)
	require.Equal(t, "already-base64", compliantSource["data"])
}

func TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-3-5-sonnet-20240620",
		IsStream:                  false,
		UseRuntimeHeadersOverride: true,
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "computer-use-2025-01-24",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
		},
	}

	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	adaptor := &Adaptor{}

	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))

	anthropicBeta, exists := payload["anthropic_beta"]
	require.True(t, exists)

	values, ok := anthropicBeta.([]any)
	require.True(t, ok)
	require.Equal(t, []any{"computer-use-2025-01-24"}, values)
}

func TestDoAwsClientRequest_UsesConditionalBedrockConversionWhenEnabled(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("anthropic-beta", "test-beta")

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-3-5-sonnet-20240620",
		IsStream:        false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1",
			UpstreamModelName: "claude-3-5-sonnet-20240620",
			ChannelSetting: dto.ChannelSettings{
				AwsBedrockRequestConversionEnabled: true,
			},
		},
	}

	requestBody := bytes.NewBufferString(`{"model":"claude-3-5-sonnet-20240620","stream":false,"messages":[{"role":"system","content":"system prompt"},{"role":"user","content":"hello"}],"max_tokens":128,"custom_field":"preserved"}`)
	adaptor := &Adaptor{}

	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))
	require.Equal(t, awsBedrockAnthropicVersion, payload["anthropic_version"])
	require.Equal(t, "system prompt", payload["system"])
	require.Equal(t, []any{"test-beta"}, payload["anthropic_beta"])
	require.Equal(t, "preserved", payload["custom_field"])
	require.NotContains(t, payload, "model")
	require.NotContains(t, payload, "stream")
}
