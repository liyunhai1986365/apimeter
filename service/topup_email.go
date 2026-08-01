package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

type topUpSuccessEmailSender func(subject string, receiver string, content string) error

var topUpSuccessEmailSenderFunc topUpSuccessEmailSender = common.SendEmail

// TopUpSuccessEmailParams carries the provider-confirmed paid amount. PaidAmount
// is a formatted decimal string so crypto amounts keep their exact unique
// fraction instead of being rounded through float64.
type TopUpSuccessEmailParams struct {
	TradeNo      string
	PaidAmount   string
	Currency     string
	Automatic    bool
	RechargeType string
}

func NotifyTopUpSuccessAsync(params TopUpSuccessEmailParams) {
	gopool.Go(func() {
		for attempt := 1; attempt <= 3; attempt++ {
			if err := sendTopUpSuccessEmail(params); err != nil {
				logger.LogError(context.Background(), fmt.Sprintf("top-up success email failed trade_no=%s attempt=%d error=%q", params.TradeNo, attempt, err.Error()))
				if attempt < 3 {
					time.Sleep(time.Duration(attempt*2) * time.Second)
				}
				continue
			}
			return
		}
	})
}

func sendTopUpSuccessEmail(params TopUpSuccessEmailParams) (err error) {
	topUp, claimValue, claimed, err := model.ClaimTopUpSuccessEmail(params.TradeNo)
	if err != nil {
		return err
	}
	if !claimed {
		return nil
	}
	claimCompleted := false
	defer func() {
		if !claimCompleted {
			if releaseErr := model.ReleaseTopUpSuccessEmailClaim(params.TradeNo, claimValue); releaseErr != nil {
				logger.LogError(context.Background(), fmt.Sprintf("top-up success email claim release failed trade_no=%s error=%q", params.TradeNo, releaseErr.Error()))
			}
		}
	}()

	if topUp == nil || topUp.Status != common.TopUpStatusSuccess {
		return errors.New("top-up is not completed")
	}
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil {
		return err
	}
	receiver := strings.TrimSpace(user.Email)
	if receiver == "" {
		logger.LogInfo(context.Background(), fmt.Sprintf("top-up success email skipped: user has no email user_id=%d trade_no=%s", topUp.UserId, params.TradeNo))
	} else {
		subject := "钱包充值成功通知"
		content := buildTopUpSuccessEmailContent(topUp, params)
		if err := topUpSuccessEmailSenderFunc(subject, receiver, content); err != nil {
			return err
		}
		logger.LogInfo(context.Background(), fmt.Sprintf("top-up success email sent user_id=%d trade_no=%s provider=%s automatic=%t", topUp.UserId, params.TradeNo, topUp.PaymentProvider, params.Automatic))
	}

	if err := model.CompleteTopUpSuccessEmailClaim(params.TradeNo, claimValue); err != nil {
		return err
	}
	claimCompleted = true
	return nil
}

func buildTopUpSuccessEmailContent(topUp *model.TopUp, params TopUpSuccessEmailParams) string {
	rechargeType := strings.TrimSpace(params.RechargeType)
	if rechargeType == "" {
		rechargeType = topUpRechargeType(topUp, params.Automatic)
	}
	paidAmount := strings.TrimSpace(params.PaidAmount)
	if paidAmount == "" {
		paidAmount = fmt.Sprintf("%.2f", topUp.Money)
	}
	currency := strings.ToUpper(strings.TrimSpace(params.Currency))
	if currency != "" {
		paidAmount += " " + currency
	}
	var creditedQuota string
	switch topUp.PaymentProvider {
	case model.PaymentProviderStripe:
		creditedQuota = logger.FormatQuota(int(topUp.Money * common.QuotaPerUnit))
	case model.PaymentProviderCreem:
		creditedQuota = logger.FormatQuota(int(topUp.Amount))
	default:
		creditedQuota = logger.FormatQuota(int(float64(topUp.Amount) * common.QuotaPerUnit))
	}
	completedAt := time.Unix(topUp.CompleteTime, 0).Format("2006-01-02 15:04:05")

	return fmt.Sprintf(
		"<p>您好，您的钱包充值已成功到账。</p>"+
			"<ul>"+
			"<li><strong>充值类型：</strong>%s</li>"+
			"<li><strong>支付金额：</strong>%s</li>"+
			"<li><strong>到账额度：</strong>%s</li>"+
			"<li><strong>订单号：</strong>%s</li>"+
			"<li><strong>到账时间：</strong>%s</li>"+
			"</ul>"+
			"<p>如非本人操作，请及时联系管理员。</p>",
		html.EscapeString(rechargeType),
		html.EscapeString(paidAmount),
		html.EscapeString(creditedQuota),
		html.EscapeString(topUp.TradeNo),
		html.EscapeString(completedAt),
	)
}

func topUpRechargeType(topUp *model.TopUp, automatic bool) string {
	if automatic {
		return "自动充值"
	}
	switch topUp.PaymentProvider {
	case model.PaymentProviderCrypto:
		if topUp.PaymentMethod == model.PaymentMethodCryptoTron {
			return "USDT 链上充值（TRON）"
		}
		return "USDT 链上充值（EVM）"
	case model.PaymentProviderCreem:
		return "Creem 在线充值"
	case model.PaymentProviderWaffo:
		return "Waffo 在线充值"
	case model.PaymentProviderWaffoPancake:
		return "Waffo Pancake 在线充值"
	case model.PaymentProviderEpay:
		return "在线充值"
	case model.PaymentProviderStripe:
		return "Stripe 在线充值"
	default:
		return "在线充值"
	}
}
