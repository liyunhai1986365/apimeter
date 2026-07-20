package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNAMEVerifyTargetUsesTokenAndBaseDomain(t *testing.T) {
	target, err := CNAMEVerifyTarget(" token-123 ", " agent-cname.example.com. ")

	require.NoError(t, err)
	require.Equal(t, "token-123.agent-cname.example.com", target)
}

func TestCNAMEVerifyTargetRequiresConfiguredBaseDomain(t *testing.T) {
	_, err := CNAMEVerifyTarget("verify-token", "")

	require.ErrorIs(t, err, ErrAgentCNAMEBaseDomainNotConfigured)
}
