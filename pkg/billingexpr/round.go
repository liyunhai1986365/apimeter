package billingexpr

import "github.com/QuantumNous/new-api/common"

// QuotaRound converts a float64 quota value to int using half-away-from-zero
// rounding. Every tiered billing path (pre-consume, settlement, breakdown
// validation, log fields) MUST use this function to avoid +-1 discrepancies.
func QuotaRound(f float64) int {
	return common.QuotaRound(f)
}

// QuotaRoundChecked is QuotaRound but also reports whether the result had to
// be saturated. Pre-consume callers use this to reject an unrepresentable
// estimate before any quota is deducted.
func QuotaRoundChecked(f float64) (int, *common.QuotaClamp) {
	return common.QuotaRoundChecked(f)
}
