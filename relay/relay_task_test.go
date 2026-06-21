package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestApplyTaskOtherRatiosAppliesSeedanceTieredExprSecondsPreConsumeQuota(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
		},
		PriceData: types.PriceData{
			Quota: 250000,
			OtherRatios: map[string]float64{
				"seconds": 5,
			},
		},
	}

	applyTaskOtherRatios(info, "doubao-seedance-2-0-260128")

	require.Equal(t, 1250000, info.PriceData.Quota)
}

func TestApplyTaskOtherRatiosKeepsPerCallTaskMultiplier(t *testing.T) {
	info := &relaycommon.RelayInfo{
		PriceData: types.PriceData{
			Quota: 1000,
			OtherRatios: map[string]float64{
				"seconds": 5,
				"size":    2,
			},
		},
	}

	applyTaskOtherRatios(info, "sora-2")

	require.Equal(t, 10000, info.PriceData.Quota)
}
