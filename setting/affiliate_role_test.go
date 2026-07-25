package setting

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveAffiliateRewardPolicyUsesRoleOverridesAndDefaults(t *testing.T) {
	originalRatio := common.AffiliateTopUpRewardRatio
	originalLimit := common.AffiliateTopUpRewardLimit
	originalInviterQuota := common.QuotaForInviter
	originalInviteeQuota := common.QuotaForInvitee
	t.Cleanup(func() {
		common.AffiliateTopUpRewardRatio = originalRatio
		common.AffiliateTopUpRewardLimit = originalLimit
		common.QuotaForInviter = originalInviterQuota
		common.QuotaForInvitee = originalInviteeQuota
		require.NoError(t, UpdateAffiliateRoleConfigsByJSONString(DefaultAffiliateRoleConfigsJSON))
	})

	common.AffiliateTopUpRewardRatio = 0.1
	common.AffiliateTopUpRewardLimit = 3
	common.QuotaForInviter = 1000
	common.QuotaForInvitee = 2000
	require.NoError(t, UpdateAffiliateRoleConfigsByJSONString(`[
		{"id":"partner","name":"Partner","topup_reward_ratio":25,"inviter_reward_quota":5000}
	]`))

	policy := ResolveAffiliateRewardPolicy("partner")
	assert.Equal(t, "partner", policy.RoleId)
	assert.Equal(t, "Partner", policy.RoleName)
	assert.False(t, policy.UsesDefaultRole)
	assert.Equal(t, 0.25, policy.TopUpRewardRatio)
	assert.Equal(t, 3, policy.TopUpRewardLimit)
	assert.Equal(t, 5000, policy.InviterRewardQuota)
	assert.Equal(t, 2000, policy.InviteeRewardQuota)

	defaultPolicy := ResolveAffiliateRewardPolicy("missing")
	assert.True(t, defaultPolicy.UsesDefaultRole)
	assert.Equal(t, 0.1, defaultPolicy.TopUpRewardRatio)
	assert.Equal(t, 3, defaultPolicy.TopUpRewardLimit)
}

func TestParseAffiliateRoleConfigsRejectsDuplicateIDsAndInvalidValues(t *testing.T) {
	_, err := ParseAffiliateRoleConfigs(`[
		{"id":"partner","name":"Partner"},
		{"id":"partner","name":"Senior Partner"}
	]`)
	require.ErrorContains(t, err, "duplicate affiliate role id")

	_, err = ParseAffiliateRoleConfigs(`[
		{"id":"partner","name":"Partner","topup_reward_ratio":101}
	]`)
	require.ErrorContains(t, err, "must be between 0 and 100")
}
