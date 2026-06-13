package service

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestAppendChannelCostInfoUsesChannelRatioAndBaseQuota(t *testing.T) {
	other := map[string]interface{}{}
	relayInfo := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelRatio: 0.8,
		},
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1.5,
			},
		},
	}

	appendChannelCostInfo(other, relayInfo, 1500)

	require.Equal(t, 1.5, other["group_ratio"])
	require.Equal(t, 0.8, other["channel_ratio"])
	require.Equal(t, 1000, other["cost_base_quota"])
	require.Equal(t, 800, other["cost_quota"])
	require.Equal(t, 700, other["profit_quota"])
	require.Equal(t, 0.4667, other["profit_rate"])
}

func TestAppendChannelCostInfoDefaultsMissingChannelRatioToOne(t *testing.T) {
	other := map[string]interface{}{}
	relayInfo := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 2,
			},
		},
	}

	appendChannelCostInfo(other, relayInfo, 200)

	require.Equal(t, 1.0, other["channel_ratio"])
	require.Equal(t, 100, other["cost_base_quota"])
	require.Equal(t, 100, other["cost_quota"])
	require.Equal(t, 100, other["profit_quota"])
	require.Equal(t, 0.5, other["profit_rate"])
}
