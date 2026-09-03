package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestAgentEmailSiteURLNormalizesDomain(t *testing.T) {
	require.Equal(t, "https://agent.example.com", AgentEmailSiteURL(" Agent.Example.com:443 "))
	require.Empty(t, AgentEmailSiteURL(" "))
}

func TestEmailSiteURLForRelayUsesAgentContextDomain(t *testing.T) {
	require.Empty(t, emailSiteURLForRelay(nil))
	require.Empty(t, emailSiteURLForRelay(&relaycommon.RelayInfo{}))
	require.Equal(t, "https://agent.example.com", emailSiteURLForRelay(&relaycommon.RelayInfo{
		AgentContext: &types.AgentContext{Domain: "agent.example.com"},
	}))
}

func TestRewriteEmailContentForSiteUsesConfiguredMainSiteAsSource(t *testing.T) {
	previousAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() { system_setting.ServerAddress = previousAddress })

	content := `<a href="https://modelsell.com/wallet">Wallet</a> <a href="https://external.example/wallet">External</a>`
	rewritten := RewriteEmailContentForSite(content, "https://agent.example.com")

	require.Contains(t, rewritten, `href="https://agent.example.com/wallet"`)
	require.Contains(t, rewritten, `href="https://external.example/wallet"`)
}
