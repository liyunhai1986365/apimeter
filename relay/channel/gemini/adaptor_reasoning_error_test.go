package gemini

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGeminiReasoningValidationReturnsBadRequest(t *testing.T) {
	settings := model_setting.GetGeminiSettings()
	oldThinkingAdapter := settings.ThinkingAdapterEnabled
	settings.ThinkingAdapterEnabled = true
	t.Cleanup(func() { settings.ThinkingAdapterEnabled = oldThinkingAdapter })

	for _, tc := range []struct {
		name, model, body string
		format            types.RelayFormat
		wantError         string
	}{
		{
			name: "chat invalid effort", format: types.RelayFormatOpenAI,
			body:      `{"model":"gemini-2.5-flash","reasoning_effort":"typo","messages":[{"role":"user","content":"hello"}]}`,
			wantError: "unsupported reasoning effort",
		},
		{
			name: "responses invalid effort", format: types.RelayFormatOpenAIResponses,
			body:      `{"model":"gemini-2.5-flash","reasoning":{"effort":"typo"},"input":"hello"}`,
			wantError: "unsupported reasoning effort",
		},
		{
			name: "chat invalid native level", format: types.RelayFormatOpenAI,
			body:      `{"model":"gemini-2.5-flash","extra_body":{"google":{"thinking_config":{"thinking_level":"typo"}}},"messages":[{"role":"user","content":"hello"}]}`,
			wantError: "unsupported reasoning effort",
		},
		{
			name: "native invalid budget suffix", format: types.RelayFormatGemini,
			model:     "gemini-2.5-flash-thinking-typo",
			body:      `{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`,
			wantError: "invalid Gemini thinking budget",
		},
		{
			name: "chat valid effort", format: types.RelayFormatOpenAI,
			body: `{"model":"gemini-2.5-flash","reasoning_effort":"low","messages":[{"role":"user","content":"hello"}]}`,
		},
		{
			name: "responses valid effort", format: types.RelayFormatOpenAIResponses,
			body: `{"model":"gemini-2.5-flash","reasoning":{"effort":"low"},"input":"hello"}`,
		},
		{
			name: "native config remains passthrough", format: types.RelayFormatGemini,
			body: `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"provider_specific"}},"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/", nil)
			model := tc.model
			if model == "" {
				model = "gemini-2.5-flash"
			}
			info := &relaycommon.RelayInfo{
				OriginModelName: model,
				RelayFormat:     tc.format,
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelType: constant.ChannelTypeGemini, UpstreamModelName: model,
				},
			}
			adaptor := &Adaptor{}
			var converted any
			var err error
			switch tc.format {
			case types.RelayFormatOpenAI:
				var request dto.GeneralOpenAIRequest
				require.NoError(t, common.UnmarshalJsonStr(tc.body, &request))
				converted, err = adaptor.ConvertOpenAIRequest(ctx, info, &request)
			case types.RelayFormatOpenAIResponses:
				var request dto.OpenAIResponsesRequest
				require.NoError(t, common.UnmarshalJsonStr(tc.body, &request))
				converted, err = adaptor.ConvertOpenAIResponsesRequest(ctx, info, request)
			case types.RelayFormatGemini:
				var request dto.GeminiChatRequest
				require.NoError(t, common.UnmarshalJsonStr(tc.body, &request))
				converted, err = adaptor.ConvertGeminiRequest(ctx, info, &request)
			}
			if tc.wantError == "" {
				require.NoError(t, err)
				require.NotNil(t, converted)
				return
			}
			require.ErrorContains(t, err, tc.wantError)
			require.Nil(t, converted)
			var apiErr *types.NewAPIError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Equal(t, types.ErrorCodeInvalidRequest, apiErr.GetErrorCode())
			require.True(t, types.IsSkipRetryError(apiErr))

			// Entry handlers wrap adapter errors; the client status must survive.
			wrapped := types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
			require.Equal(t, http.StatusBadRequest, wrapped.StatusCode)
			require.Equal(t, types.ErrorCodeInvalidRequest, wrapped.GetErrorCode())
		})
	}
}
