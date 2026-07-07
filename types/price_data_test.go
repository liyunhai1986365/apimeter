package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPriceDataAddOtherRatioRejectsInvalidRatios(t *testing.T) {
	var price PriceData

	price.AddOtherRatio("nan", math.NaN())
	price.AddOtherRatio("inf", math.Inf(1))
	price.AddOtherRatio("negative", -1)
	price.AddOtherRatio("valid", 2)

	require.Equal(t, map[string]float64{"valid": 2}, price.OtherRatios)
}
