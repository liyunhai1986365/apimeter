package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

func setupStripeConversionTest(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TopUp{}, &model.SubscriptionOrder{}))

	oldDB := model.DB
	oldStripeKey := setting.StripeApiSecret
	oldRetriever := retrieveStripeCheckoutSession
	model.DB = db
	setting.StripeApiSecret = "sk_test_conversion"
	t.Cleanup(func() {
		model.DB = oldDB
		setting.StripeApiSecret = oldStripeKey
		retrieveStripeCheckoutSession = oldRetriever
	})
}

func TestAppendStripeCheckoutSessionPreservesExistingQuery(t *testing.T) {
	returnURL := appendStripeCheckoutSession("https://example.com/wallet?show_history=true")
	require.Equal(
		t,
		"https://example.com/wallet?show_history=true&stripe_session_id={CHECKOUT_SESSION_ID}",
		returnURL,
	)
}

func TestGetStripePurchaseConversionReturnsVerifiedPurchase(t *testing.T) {
	setupStripeConversionTest(t)
	require.NoError(t, (&model.TopUp{
		UserId:          41,
		TradeNo:         "ref_verified",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
	}).Insert())

	retrieveStripeCheckoutSession = func(_ string, _ *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
		return &stripe.CheckoutSession{
			ClientReferenceID: "ref_verified",
			PaymentStatus:     stripe.CheckoutSessionPaymentStatusPaid,
			AmountTotal:       1999,
			Currency:          stripe.CurrencyUSD,
		}, nil
	}

	router := gin.New()
	router.GET("/conversion", func(c *gin.Context) {
		c.Set("id", 41)
		GetStripePurchaseConversion(c)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/conversion?session_id=cs_test_123", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Success bool                     `json:"success"`
		Data    stripePurchaseConversion `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, "paid", response.Data.Status)
	require.Equal(t, "ref_verified", response.Data.TransactionID)
	require.Equal(t, 19.99, response.Data.Value)
	require.Equal(t, "USD", response.Data.Currency)
}

func TestGetStripePurchaseConversionDoesNotCountUnpaidSession(t *testing.T) {
	setupStripeConversionTest(t)
	require.NoError(t, (&model.SubscriptionOrder{
		UserId:          52,
		TradeNo:         "sub_ref_pending",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		Status:          common.TopUpStatusPending,
	}).Insert())

	retrieveStripeCheckoutSession = func(_ string, _ *stripe.CheckoutSessionParams) (*stripe.CheckoutSession, error) {
		return &stripe.CheckoutSession{
			ClientReferenceID: "sub_ref_pending",
			PaymentStatus:     stripe.CheckoutSessionPaymentStatusUnpaid,
		}, nil
	}

	router := gin.New()
	router.GET("/conversion", func(c *gin.Context) {
		c.Set("id", 52)
		GetStripePurchaseConversion(c)
	})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/conversion?session_id=cs_test_pending", nil))

	var response struct {
		Success bool                     `json:"success"`
		Data    stripePurchaseConversion `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, "pending", response.Data.Status)
	require.Empty(t, response.Data.TransactionID)
	require.Zero(t, response.Data.Value)
}
