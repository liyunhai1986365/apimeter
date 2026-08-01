package service

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
)

func TestStripeAutoRechargePaymentIntentExplicitlyUsesCard(t *testing.T) {
	params := newStripeAutoRechargePaymentIntentParams(
		&model.StripeAutoRecharge{
			StripeCustomerId:      "cus_test",
			StripePaymentMethodId: "pm_test",
		},
		&model.User{Id: 9100, Email: "stripe-auto@example.com"},
		"auto_ref_params",
		2000,
	)

	require.Len(t, params.PaymentMethodTypes, 1)
	require.Equal(t, string(stripe.PaymentMethodTypeCard), stripe.StringValue(params.PaymentMethodTypes[0]))
	require.True(t, stripe.BoolValue(params.ErrorOnRequiresAction))
	require.True(t, stripe.BoolValue(params.OffSession))
}

func TestStripeAutoRechargeAmountCentsUsesConfiguredUSD(t *testing.T) {
	require.Equal(t, int64(20000), stripeAutoRechargeAmountCents(200))
}

func TestHandleStripeAutoRechargePaymentSucceededRejectsAmountMismatch(t *testing.T) {
	truncate(t)
	user := &model.User{
		Id:       9101,
		Username: "stripe_auto_verify",
		AffCode:  "stripe_auto_verify_aff",
		Quota:    0,
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          20,
		Money:           20,
		TradeNo:         "auto_ref_verify",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	require.NoError(t, model.DB.Create(&model.StripeAutoRecharge{
		UserId:                 user.Id,
		StripeCustomerId:       "cus_verify",
		StripePaymentMethodId:  "pm_verify",
		CardBrand:              "visa",
		CardLast4:              "4242",
		Enabled:                true,
		ThresholdQuota:         int64(common.QuotaPerUnit),
		TopUpAmount:            20,
		State:                  model.StripeAutoRechargeStatePending,
		PendingTradeNo:         topUp.TradeNo,
		PendingPaymentIntentId: "pi_verify",
		PendingAmountCents:     2000,
	}).Error)

	err := HandleStripeAutoRechargePaymentSucceeded(
		context.Background(), topUp.TradeNo, "pi_verify", "cus_verify", 1999, "usd", "127.0.0.1",
	)
	require.ErrorContains(t, err, "verification failed")
	require.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(topUp.TradeNo).Status)
	storedUser, err := model.GetUserById(user.Id, false)
	require.NoError(t, err)
	require.Zero(t, storedUser.Quota)
}
