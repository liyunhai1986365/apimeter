package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAdaptNativeClaudeThinking(t *testing.T) {
	settings := model_setting.GetClaudeSettings()
	oldEnabled := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = true
	t.Cleanup(func() { settings.ThinkingAdapterEnabled = oldEnabled })
	for _, tc := range []struct {
		name, model                     string
		thinking                        *dto.Thinking
		config                          json.RawMessage
		wantType, wantEffort, wantModel string
		wantBudget                      int
	}{
		{name: "omitted", model: "claude-opus-5-5"},
		{name: "legacy enabled", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, wantType: "adaptive", wantEffort: "high"},
		{name: "preserve config", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(8192)}, config: json.RawMessage(`{"effort":"low","format":{"type":"json_schema","schema":{"type":"object","properties":{"n":{"const":9007199254740993}}}}}`), wantType: "adaptive", wantEffort: "low"},
		{name: "adaptive budget removed", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "adaptive", BudgetTokens: common.GetPointer(0), Display: "omitted"}, config: json.RawMessage(`{"effort":"medium"}`), wantType: "adaptive", wantEffort: "medium"},
		{name: "disabled uses omission", model: "claude-opus-5-5", thinking: &dto.Thinking{Type: "disabled"}},
		{name: "thinking suffix", model: "claude-opus-5-5-thinking", wantType: "adaptive", wantEffort: "high", wantModel: "claude-opus-5-5"},
		{name: "thinking suffix with explicit budget", model: "claude-opus-5-5-thinking", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(8192)}, wantType: "adaptive", wantEffort: "high", wantModel: "claude-opus-5-5"},
		{name: "effort suffix", model: "claude-opus-5-5-high", config: json.RawMessage(`{"format":{"type":"json_schema","schema":{"type":"object"}}}`), wantType: "adaptive", wantEffort: "high", wantModel: "claude-opus-5-5"},
		{name: "legacy model unchanged", model: "claude-sonnet-4-5", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, wantType: "enabled", wantBudget: 4096},
		{name: "opus 4.6 manual unchanged", model: "claude-opus-4-6", thinking: &dto.Thinking{Type: "enabled", BudgetTokens: common.GetPointer(4096)}, wantType: "enabled", wantBudget: 4096},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &dto.ClaudeRequest{Model: tc.model, MaxTokens: common.GetPointer[uint](16384), Thinking: tc.thinking, OutputConfig: tc.config, Temperature: common.GetPointer(0.0)}
			require.NoError(t, adaptClaudeThinking(req, tc.model))
			wantModel := tc.wantModel
			if wantModel == "" {
				wantModel = tc.model
			}
			require.Equal(t, wantModel, req.Model)
			body, err := common.Marshal(req)
			require.NoError(t, err)
			require.Equal(t, tc.wantType, gjson.GetBytes(body, "thinking.type").String())
			require.Equal(t, tc.wantEffort, gjson.GetBytes(body, "output_config.effort").String())
			if tc.wantBudget > 0 {
				require.Equal(t, int64(tc.wantBudget), gjson.GetBytes(body, "thinking.budget_tokens").Int())
			} else {
				require.False(t, gjson.GetBytes(body, "thinking.budget_tokens").Exists())
			}
			if tc.wantType == "" {
				require.False(t, gjson.GetBytes(body, "thinking").Exists())
			}
			if gjson.GetBytes(tc.config, "format").Exists() {
				require.JSONEq(t, gjson.GetBytes(tc.config, "format").Raw, gjson.GetBytes(body, "output_config.format").Raw)
			}
			if tc.name == "preserve config" {
				require.Equal(t, "9007199254740993", gjson.GetBytes(body, "output_config.format.schema.properties.n.const").Raw)
			}
			if tc.thinking != nil && tc.thinking.Display != "" {
				require.Equal(t, tc.thinking.Display, req.Thinking.Display)
			}
			// No evidence that Opus 5.5 shares 4.7's sampling restriction.
			require.Equal(t, 0.0, *req.Temperature)
		})
	}
}

func TestAdaptNativeClaudeThinkingRejectsInvalidOutputConfig(t *testing.T) {
	req := &dto.ClaudeRequest{Model: "claude-opus-5-5", Thinking: &dto.Thinking{Type: "enabled"}, OutputConfig: json.RawMessage(`[]`)}
	require.ErrorContains(t, adaptClaudeThinking(req, req.Model), "output_config")
}

type claudePassthroughTransport func(*http.Request) (*http.Response, error)

func (f claudePassthroughTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestClaudePassthroughPreservesBodyAndRoutesModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	client := service.GetHttpClient()
	oldTransport := client.Transport
	global, claude := model_setting.GetGlobalSettings(), model_setting.GetClaudeSettings()
	oldGlobal, oldEnabled := *global, claude.ThinkingAdapterEnabled
	t.Cleanup(func() {
		client.Transport = oldTransport
		*global, claude.ThinkingAdapterEnabled = oldGlobal, oldEnabled
	})
	for _, provider := range []struct {
		name string
		kind int
	}{{"aws", constant.ChannelTypeAws}, {"vertex", constant.ChannelTypeVertexAi}} {
		for _, globalPass := range []bool{false, true} {
			for _, tc := range []struct {
				name, model, mapped, want string
				disabled, preserve        bool
			}{
				{name: "effort", model: "claude-opus-4-6-high", want: "claude-opus-4-6"},
				{name: "thinking", model: "claude-sonnet-4-5-20250929-thinking", want: "claude-sonnet-4-5-20250929"},
				{name: "mapped", model: "public-model", mapped: "claude-opus-4-7-high", want: "claude-opus-4-7"},
				{name: "preserved", model: "public-thinking", mapped: "claude-opus-4-7-thinking", want: "claude-opus-4-7-thinking", preserve: true},
				{name: "disabled", model: "claude-opus-4-7-thinking", want: "claude-opus-4-7-thinking", disabled: true},
			} {
				t.Run(fmt.Sprintf("%s/global=%v/%s", provider.name, globalPass, tc.name), func(t *testing.T) {
					global.PassThroughRequestEnabled = globalPass
					global.ThinkingModelBlacklist = nil
					if tc.preserve {
						global.ThinkingModelBlacklist = []string{tc.model}
					}
					claude.ThinkingAdapterEnabled = !tc.disabled
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					// Passthrough must not validate output_config or synthesize thinking.
					payload := fmt.Sprintf(`{"model":%q,"anthropic_version":"bedrock-2023-05-31","max_tokens":100,"temperature":0,"output_config":[],"custom_field":true,"messages":[{"role":"user","content":"hello"}]}`, tc.model)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(payload))
					c.Request.Header.Set("Content-Type", "application/json")
					defer common.CleanupBodyStorage(c)
					common.SetContextKey(c, constant.ContextKeyChannelType, provider.kind)
					common.SetContextKey(c, constant.ContextKeyChannelKey, "test-access|test-secret|us-east-1")
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: !globalPass})
					common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{VertexKeyType: dto.VertexKeyTypeAPIKey})
					c.Set("region", "us-east5")
					if tc.mapped != "" {
						mapping, err := common.Marshal(map[string]string{tc.model: tc.mapped})
						require.NoError(t, err)
						c.Set("model_mapping", string(mapping))
					}
					var request dto.ClaudeRequest
					require.NoError(t, common.Unmarshal([]byte(payload), &request))
					info := &relaycommon.RelayInfo{OriginModelName: tc.model, Request: &request}
					var sentURL string
					var sentBody []byte
					client.Transport = claudePassthroughTransport(func(r *http.Request) (*http.Response, error) {
						sentURL = r.URL.String()
						var err error
						sentBody, err = io.ReadAll(r.Body)
						if err != nil {
							return nil, err
						}
						// Stop before response billing; no request leaves this process.
						return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"message":"test stop","type":"error","error":{"type":"invalid_request_error","message":"test stop"}}`)), Request: r}, nil
					})
					require.NotNil(t, ClaudeHelper(c, info))
					require.NotEmpty(t, sentURL)
					require.Equal(t, tc.want, info.UpstreamModelName)
					require.Empty(t, info.ReasoningEffort)
					if !tc.disabled && !tc.preserve {
						require.NotContains(t, sentURL, "-high")
						require.NotContains(t, sentURL, "-thinking")
					}
					if provider.kind == constant.ChannelTypeAws {
						// Bedrock's existing passthrough adapter removes the model field.
						var expected map[string]any
						require.NoError(t, common.Unmarshal([]byte(payload), &expected))
						delete(expected, "model")
						body, err := common.Marshal(expected)
						require.NoError(t, err)
						require.JSONEq(t, string(body), string(sentBody))
					} else {
						require.Equal(t, payload, string(sentBody))
					}
				})
			}
		}
	}
}
