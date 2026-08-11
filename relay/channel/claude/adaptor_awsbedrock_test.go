package claude

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicDoRequestAppliesAwsBedrockConversionWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	type receivedRequest struct {
		body []byte
		beta string
	}
	received := make(chan receivedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		received <- receivedRequest{body: body, beta: r.Header.Get("anthropic-beta")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"type":"message","content":[]}`))
	}))
	defer server.Close()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("anthropic-beta", "test-beta")

	info := &relaycommon.RelayInfo{
		OriginModelName: "claude-opus-4-8",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:    server.URL,
			ApiKey:            "test-key",
			UpstreamModelName: "claude-opus-4-8",
			HeadersOverride: map[string]interface{}{
				"*": "",
			},
			ChannelSetting: dto.ChannelSettings{
				AwsBedrockRequestConversionEnabled: true,
			},
		},
	}
	originalBody := []byte(`{"model":"claude-opus-4-8","stream":true,"max_tokens":24,"messages":[{"role":"system","content":"system prompt"},{"role":"user","content":"hello"}],"custom_field":"preserved"}`)

	response, err := (&Adaptor{}).DoRequest(ctx, info, bytes.NewReader(originalBody))
	require.NoError(t, err)
	require.NoError(t, response.(*http.Response).Body.Close())

	request := <-received
	var payload map[string]any
	require.NoError(t, common.Unmarshal(request.body, &payload))
	require.Equal(t, "system prompt", payload["system"])
	require.Equal(t, "preserved", payload["custom_field"])
	require.Equal(t, "claude-opus-4-8", payload["model"])
	require.Equal(t, true, payload["stream"])
	require.NotContains(t, payload, "anthropic_version")
	require.NotContains(t, payload, "anthropic_beta")
	require.Empty(t, request.beta)

	messages := payload["messages"].([]any)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].(map[string]any)["role"])
}

func TestFilterAwsBedrockAnthropicBetaHeaderByModelCapability(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		input    string
		expected string
	}{
		{
			name:     "Opus 4.8 removes Anthropic-only and incompatible model betas",
			model:    "claude-opus-4-8",
			input:    "claude-code-20250219,context-1m-2025-08-07,interleaved-thinking-2025-05-14,computer-use-2025-11-24,computer-use-2025-11-24,advisor-tool-2026-03-01,effort-2025-11-24",
			expected: "computer-use-2025-11-24",
		},
		{
			name:     "Sonnet 4.6 keeps its documented long context compaction and computer betas",
			model:    "anthropic.claude-sonnet-4-6",
			input:    "context-1m-2025-08-07,compact-2026-01-12,computer-use-2025-11-24,effort-2025-11-24,advisor-tool-2026-03-01",
			expected: "context-1m-2025-08-07,compact-2026-01-12,computer-use-2025-11-24",
		},
		{
			name:     "Opus 4.5 keeps its documented effort and tool betas",
			model:    "claude-opus-4-5-20251101",
			input:    "effort-2025-11-24,tool-search-tool-2025-10-19,tool-examples-2025-10-29,computer-use-2025-01-24,thinking-token-count-2026-05-13",
			expected: "effort-2025-11-24,tool-search-tool-2025-10-19,tool-examples-2025-10-29,computer-use-2025-01-24",
		},
		{
			name:     "Haiku 4.5 keeps context management but removes Opus effort",
			model:    "claude-haiku-4-5-20251001",
			input:    "context-management-2025-06-27,computer-use-2025-01-24,effort-2025-11-24,prompt-caching-scope-2026-01-05",
			expected: "context-management-2025-06-27,computer-use-2025-01-24",
		},
		{
			name:     "Sonnet 3.7 keeps output and computer betas",
			model:    "claude-3-7-sonnet-20250219",
			input:    "output-128k-2025-02-19,computer-use-2025-01-24,interleaved-thinking-2025-05-14,claude-code-20250219",
			expected: "output-128k-2025-02-19,computer-use-2025-01-24",
		},
		{
			name:     "Unknown future model keeps only AWS documented betas",
			model:    "claude-future-model",
			input:    "fine-grained-tool-streaming-2025-05-14,advisor-tool-2026-03-01",
			expected: "fine-grained-tool-streaming-2025-05-14",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			header.Set("anthropic-beta", test.input)

			filterAwsBedrockAnthropicBetaHeader(&header, test.model)

			require.Equal(t, test.expected, header.Get("anthropic-beta"))
		})
	}
}

func TestFilterAwsBedrockAnthropicBetaHeaderPrefersUpstreamModel(t *testing.T) {
	header := http.Header{}
	header.Set("anthropic-beta", "effort-2025-11-24,computer-use-2025-11-24")

	filterAwsBedrockAnthropicBetaHeader(&header, "claude-opus-4-8", "claude-opus-4-5-20251101")

	require.Equal(t, "computer-use-2025-11-24", header.Get("anthropic-beta"))
}
