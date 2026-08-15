package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSetupContextForSelectedChannelClearsOptionalChannelState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	organization := "org-from-first-channel"
	tests := []struct {
		name       string
		first      *model.Channel
		contextKey string
		expected   string
	}{
		{
			name:       "organization",
			first:      &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "first-key", OpenAIOrganization: &organization},
			contextKey: string(constant.ContextKeyChannelOrganization),
			expected:   organization,
		},
		{
			name:       "api version",
			first:      &model.Channel{Type: constant.ChannelTypeAzure, Key: "first-key", Other: "2026-01-01"},
			contextKey: "api_version",
			expected:   "2026-01-01",
		},
		{
			name:       "region",
			first:      &model.Channel{Type: constant.ChannelTypeVertexAi, Key: "first-key", Other: "us-central1"},
			contextKey: "region",
			expected:   "us-central1",
		},
		{
			name:       "plugin",
			first:      &model.Channel{Type: constant.ChannelTypeAli, Key: "first-key", Other: "plugin-a"},
			contextKey: "plugin",
			expected:   "plugin-a",
		},
		{
			name:       "bot id",
			first:      &model.Channel{Type: constant.ChannelTypeCoze, Key: "first-key", Other: "bot-a"},
			contextKey: "bot_id",
			expected:   "bot-a",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

			require.Nil(t, SetupContextForSelectedChannel(c, test.first, "test-model"))
			require.Equal(t, test.expected, c.GetString(test.contextKey))

			second := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "second-key"}
			require.Nil(t, SetupContextForSelectedChannel(c, second, "test-model"))
			require.Empty(t, c.GetString(test.contextKey))
			require.Equal(t, "second-key", common.GetContextKeyString(c, constant.ContextKeyChannelKey))
		})
	}
}
