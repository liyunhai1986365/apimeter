package common

import (
	"math"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

const overflowingQuotaProduct = 2000 * 1.8446744073686647e19

func TestQuotaFromFloatSaturatesOversizedCharges(t *testing.T) {
	require.Equal(t, 42, QuotaFromFloat(42.9))
	require.Equal(t, -42, QuotaFromFloat(-42.9))
	require.Equal(t, MaxQuota, QuotaFromFloat(overflowingQuotaProduct))
	require.Equal(t, MinQuota, QuotaFromFloat(-overflowingQuotaProduct))
	require.Equal(t, MaxQuota, QuotaFromFloat(math.Inf(1)))
	require.Equal(t, MinQuota, QuotaFromFloat(math.Inf(-1)))
	require.Equal(t, 0, QuotaFromFloat(math.NaN()))
}

func TestQuotaRoundSaturatesOversizedCharges(t *testing.T) {
	require.Equal(t, 43, QuotaRound(42.5))
	require.Equal(t, -43, QuotaRound(-42.5))
	require.Equal(t, MaxQuota, QuotaRound(overflowingQuotaProduct))
	require.Equal(t, MinQuota, QuotaRound(-overflowingQuotaProduct))
	require.Equal(t, 0, QuotaRound(math.NaN()))
}

func TestQuotaFromDecimalSaturatesOversizedCharges(t *testing.T) {
	require.Equal(t, 43, QuotaFromDecimal(decimal.NewFromFloat(42.5)))
	require.Equal(t, MaxQuota, QuotaFromDecimal(decimal.NewFromInt(2000).Mul(decimal.NewFromFloat(1.8446744073686647e19))))
	require.Equal(t, MinQuota, QuotaFromDecimal(decimal.NewFromInt(-2000).Mul(decimal.NewFromFloat(1.8446744073686647e19))))
}

func TestQuotaFromFloatCheckedReturnsClampMetadata(t *testing.T) {
	quota, clamp := QuotaFromFloatChecked(42.9)
	require.Equal(t, 42, quota)
	require.Nil(t, clamp)

	quota, clamp = QuotaFromFloatChecked(overflowingQuotaProduct)
	require.Equal(t, MaxQuota, quota)
	require.NotNil(t, clamp)
	require.Equal(t, "QuotaFromFloat", clamp.Op)
	require.Equal(t, QuotaClampOverflow, clamp.Kind)
	require.Equal(t, MaxQuota, clamp.Clamped)
}
