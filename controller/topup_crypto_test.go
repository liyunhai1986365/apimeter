package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/require"
)

func TestGetCryptoPaymentUSDConvertsLocalRechargePrice(t *testing.T) {
	originalPrice := operation_setting.Price
	originalExchangeRate := operation_setting.USDExchangeRate
	originalDisplayType := operation_setting.GetGeneralSetting().QuotaDisplayType
	originalRatios := common.TopupGroupRatio2JSONString()
	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	t.Cleanup(func() {
		operation_setting.Price = originalPrice
		operation_setting.USDExchangeRate = originalExchangeRate
		operation_setting.GetGeneralSetting().QuotaDisplayType = originalDisplayType
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(originalRatios))
	})

	operation_setting.Price = 7.3
	operation_setting.USDExchangeRate = 7.3
	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{10: 0.8}
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"vip":1.25}`))

	amount := getCryptoPaymentUSD(10, "vip")
	require.Equal(t, "10", amount.String())
}
