package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

func TestCryptoPaymentOrderReturnsProgressOnlyToOwner(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "crypto.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.CryptoPayment{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	payment := &model.CryptoPayment{
		UserId: 7, TradeNo: "private-progress", Status: model.CryptoPaymentStatusPending,
		CreateTime: time.Now().Unix(),
	}
	require.NoError(t, db.Create(payment).Error)
	require.NoError(t, model.UpdateCryptoPaymentProgress(payment, model.CryptoPaymentProgress{
		Stage: "confirming", CheckedAt: time.Now().Unix(), TransactionHash: "private-transaction",
		Confirmations: 3, RequiredConfirmations: 12,
	}))
	for _, userID := range []int{7, 8} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Set("id", userID)
		c.Params = gin.Params{{Key: "trade_no", Value: payment.TradeNo}}
		c.Request = httptest.NewRequest(http.MethodGet, "/api/user/crypto/order/"+payment.TradeNo, nil)
		GetCryptoPaymentOrder(c)
		var body struct {
			Success bool `json:"success"`
			Data    struct {
				TradeNo  string                      `json:"trade_no"`
				Progress model.CryptoPaymentProgress `json:"progress"`
			} `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
		if userID == 7 {
			require.True(t, body.Success)
			require.Equal(t, payment.TradeNo, body.Data.TradeNo)
			require.Equal(t, int64(3), body.Data.Progress.Confirmations)
			require.NotContains(t, recorder.Body.String(), "progress_snapshot")
		} else {
			require.False(t, body.Success)
			require.NotContains(t, recorder.Body.String(), "private-transaction")
		}
	}

	require.NoError(t, db.Model(payment).Update("status", model.CryptoPaymentStatusManual).Error)
	require.NoError(t, model.UpdateCryptoPaymentProgress(payment, model.CryptoPaymentProgress{Stage: "waiting"}))
	stored, err := model.GetCryptoPaymentForUser(payment.TradeNo, payment.UserId)
	require.NoError(t, err)
	require.Equal(t, "confirming", stored.ReadProgress().Stage, "a scanner snapshot must not overwrite a completed order")
}
