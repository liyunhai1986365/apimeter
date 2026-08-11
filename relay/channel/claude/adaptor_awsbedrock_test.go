package claude

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/awsbedrock"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAnthropicDoRequestAppliesAwsBedrockConversionWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()

	receivedBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		receivedBody <- body
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
			ChannelSetting: dto.ChannelSettings{
				AwsBedrockRequestConversionEnabled: true,
			},
		},
	}
	originalBody := []byte(`{"model":"claude-opus-4-8","stream":false,"max_tokens":24,"messages":[{"role":"system","content":"system prompt"},{"role":"user","content":"hello"}],"custom_field":"preserved"}`)

	response, err := (&Adaptor{}).DoRequest(ctx, info, bytes.NewReader(originalBody))
	require.NoError(t, err)
	require.NoError(t, response.(*http.Response).Body.Close())

	var payload map[string]any
	require.NoError(t, common.Unmarshal(<-receivedBody, &payload))
	require.Equal(t, awsbedrock.AnthropicVersion, payload["anthropic_version"])
	require.Equal(t, "system prompt", payload["system"])
	require.Equal(t, []any{"test-beta"}, payload["anthropic_beta"])
	require.Equal(t, "preserved", payload["custom_field"])
	require.Equal(t, "claude-opus-4-8", payload["model"])
	require.NotContains(t, payload, "stream")

	messages := payload["messages"].([]any)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].(map[string]any)["role"])
}
