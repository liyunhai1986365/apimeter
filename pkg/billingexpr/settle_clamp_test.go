package billingexpr_test

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

func TestComputeTieredQuotaClampsOversizedResult(t *testing.T) {
	expr := `tier("base", p * 1000000000)`
	snap := &billingexpr.BillingSnapshot{
		BillingMode:  "tiered_expr",
		ExprString:   expr,
		ExprHash:     billingexpr.ExprHashString(expr),
		GroupRatio:   1,
		QuotaPerUnit: 500_000,
	}

	result, err := billingexpr.ComputeTieredQuota(snap, billingexpr.TokenParams{P: 1_000_000_000})
	require.NoError(t, err)
	require.Equal(t, math.MaxInt32, result.ActualQuotaAfterGroup)
	require.NotNil(t, result.Clamp)
	require.Equal(t, common.QuotaClampOverflow, result.Clamp.Kind)
}
