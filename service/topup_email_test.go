package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSendTopUpSuccessEmailIncludesAutomaticRechargeDetails(t *testing.T) {
	truncate(t)
	user := &model.User{
		Id:       9201,
		Username: "stripe_email_user",
		Email:    "stripe-email@example.com",
		AffCode:  "stripe_email_aff",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          200,
		Money:           200,
		TradeNo:         "auto_ref_email_test",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      common.GetTimestamp() - 1,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, topUp.Insert())

	originalSender := topUpSuccessEmailSenderFunc
	t.Cleanup(func() { topUpSuccessEmailSenderFunc = originalSender })
	var subject, receiver, content string
	sendCount := 0
	topUpSuccessEmailSenderFunc = func(gotSubject string, gotReceiver string, gotContent string) error {
		sendCount++
		subject = gotSubject
		receiver = gotReceiver
		content = gotContent
		return nil
	}

	params := TopUpSuccessEmailParams{
		TradeNo:    topUp.TradeNo,
		PaidAmount: "200.00",
		Currency:   "usd",
		Automatic:  true,
	}
	require.NoError(t, sendTopUpSuccessEmail(params))
	require.NoError(t, sendTopUpSuccessEmail(params))
	require.Equal(t, 1, sendCount)
	require.Equal(t, "钱包充值成功通知", subject)
	require.Equal(t, user.Email, receiver)
	require.Contains(t, content, "自动充值")
	require.Contains(t, content, "200.00 USD")
	require.Contains(t, content, topUp.TradeNo)
}

func TestSendTopUpSuccessEmailIncludesExactUSDTAmount(t *testing.T) {
	truncate(t)
	user := &model.User{
		Id:       9202,
		Username: "crypto_email_user",
		Email:    "crypto-email@example.com",
		AffCode:  "crypto_email_aff",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10.017,
		TradeNo:         "crypto_email_test",
		PaymentMethod:   model.PaymentMethodCryptoTron,
		PaymentProvider: model.PaymentProviderCrypto,
		CreateTime:      common.GetTimestamp() - 1,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, topUp.Insert())

	originalSender := topUpSuccessEmailSenderFunc
	t.Cleanup(func() { topUpSuccessEmailSenderFunc = originalSender })
	var content string
	topUpSuccessEmailSenderFunc = func(_, _ string, gotContent string) error {
		content = gotContent
		return nil
	}

	require.NoError(t, sendTopUpSuccessEmail(TopUpSuccessEmailParams{
		TradeNo:    topUp.TradeNo,
		PaidAmount: "10.017",
		Currency:   "USDT",
	}))
	require.True(t, strings.Contains(content, "USDT 链上充值（TRON）"))
	require.True(t, strings.Contains(content, "10.017 USDT"))
}

func TestSendTopUpSuccessEmailIgnoresPendingOrder(t *testing.T) {
	truncate(t)
	topUp := &model.TopUp{
		UserId:          9203,
		Amount:          20,
		Money:           20,
		TradeNo:         "ref_pending_email_test",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	require.NoError(t, sendTopUpSuccessEmail(TopUpSuccessEmailParams{TradeNo: topUp.TradeNo}))
}

func TestSendTopUpSuccessEmailReleasesClaimAfterSendFailure(t *testing.T) {
	truncate(t)
	user := &model.User{
		Id:       9204,
		Username: "retry_email_user",
		Email:    "retry-email@example.com",
		AffCode:  "retry_email_aff",
		Status:   common.UserStatusEnabled,
	}
	require.NoError(t, model.DB.Create(user).Error)
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          20,
		Money:           20,
		TradeNo:         "ref_retry_email_test",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      common.GetTimestamp() - 1,
		CompleteTime:    common.GetTimestamp(),
		Status:          common.TopUpStatusSuccess,
	}
	require.NoError(t, topUp.Insert())

	originalSender := topUpSuccessEmailSenderFunc
	t.Cleanup(func() { topUpSuccessEmailSenderFunc = originalSender })
	sendCount := 0
	topUpSuccessEmailSenderFunc = func(_, _, _ string) error {
		sendCount++
		if sendCount == 1 {
			return errors.New("temporary SMTP failure")
		}
		return nil
	}

	params := TopUpSuccessEmailParams{TradeNo: topUp.TradeNo}
	require.Error(t, sendTopUpSuccessEmail(params))
	require.NoError(t, sendTopUpSuccessEmail(params))
	require.Equal(t, 2, sendCount)
}
