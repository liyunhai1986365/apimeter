package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestChannelValidateSettingsAcceptsFailoverRuleWithTargets(t *testing.T) {
	settings := dto.ChannelSettings{
		RetryPolicyRules: []dto.RetryPolicyRule{
			{
				Name:        "broken failover",
				Action:      operation_setting.RetryPolicyActionFailover,
				StatusCodes: "500",
				Targets: dto.RetryPolicyTargets{
					Groups: []string{"backup"},
				},
			},
		},
	}
	raw, err := common.Marshal(settings)
	require.NoError(t, err)
	rawString := string(raw)

	channel := Channel{Setting: &rawString}
	require.NoError(t, channel.ValidateSettings())
}
