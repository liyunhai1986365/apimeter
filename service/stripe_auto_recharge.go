package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/customer"
	"github.com/stripe/stripe-go/v81/paymentintent"
	"github.com/stripe/stripe-go/v81/paymentmethod"
	"github.com/stripe/stripe-go/v81/setupintent"
	"github.com/thanhpk/randstr"
	"gorm.io/gorm"
)

const (
	stripeAutoRechargeTickInterval   = time.Minute
	stripeAutoRechargeBatchSize      = 100
	stripeAutoRechargePurpose        = "wallet_auto_recharge"
	stripeAutoRechargeConsentVersion = "2026-08-01"
)

var (
	stripeAutoRechargeOnce    sync.Once
	stripeAutoRechargeRunning atomic.Bool

	createStripeCustomer        = customer.New
	createStripeCheckoutSession = session.New
	retrieveStripeSetupSession  = session.Get
	retrieveStripeSetupIntent   = setupintent.Get
	retrieveStripePaymentMethod = paymentmethod.Get
	detachStripePaymentMethod   = paymentmethod.Detach
	createStripePaymentIntent   = paymentintent.New
)

var ErrStripeAutoRechargeVerification = errors.New("Stripe auto recharge payment verification failed")

type StripeAutoRechargeStatus struct {
	Available      bool    `json:"available"`
	Bound          bool    `json:"bound"`
	Enabled        bool    `json:"enabled"`
	Threshold      float64 `json:"threshold"`
	TopUpAmount    int64   `json:"topup_amount"`
	MinTopUpAmount int64   `json:"min_topup_amount"`
	CardBrand      string  `json:"card_brand,omitempty"`
	CardLast4      string  `json:"card_last4,omitempty"`
	CardExpMonth   int64   `json:"card_exp_month,omitempty"`
	CardExpYear    int64   `json:"card_exp_year,omitempty"`
	State          string  `json:"state,omitempty"`
	LastError      string  `json:"last_error,omitempty"`
	LastAttemptAt  int64   `json:"last_attempt_at,omitempty"`
	LastSuccessAt  int64   `json:"last_success_at,omitempty"`
}

func StripeAutoRechargeAvailable() bool {
	return operation_setting.IsPaymentComplianceConfirmed() &&
		(strings.HasPrefix(setting.StripeApiSecret, "sk_") || strings.HasPrefix(setting.StripeApiSecret, "rk_")) &&
		strings.TrimSpace(setting.StripeWebhookSecret) != "" &&
		strings.TrimSpace(setting.StripePriceId) != ""
}

func GetStripeAutoRechargeStatus(userId int) (StripeAutoRechargeStatus, error) {
	status := StripeAutoRechargeStatus{
		Available:      StripeAutoRechargeAvailable(),
		Threshold:      10,
		TopUpAmount:    int64(max(setting.StripeMinTopUp, 20)),
		MinTopUpAmount: int64(setting.StripeMinTopUp),
		State:          model.StripeAutoRechargeStateIdle,
	}
	config, err := model.GetStripeAutoRechargeByUserId(userId)
	if err != nil || config == nil {
		return status, err
	}
	status.Bound = config.StripePaymentMethodId != ""
	status.Enabled = config.Enabled
	if config.ThresholdQuota > 0 {
		status.Threshold = float64(config.ThresholdQuota) / common.QuotaPerUnit
	}
	if config.TopUpAmount > 0 {
		status.TopUpAmount = config.TopUpAmount
	}
	status.CardBrand = config.CardBrand
	status.CardLast4 = config.CardLast4
	status.CardExpMonth = config.CardExpMonth
	status.CardExpYear = config.CardExpYear
	status.State = config.State
	status.LastError = config.LastError
	status.LastAttemptAt = config.LastAttemptAt
	status.LastSuccessAt = config.LastSuccessAt
	return status, nil
}

func ensureStripeCustomer(user *model.User) (string, error) {
	if user == nil || user.Id <= 0 {
		return "", errors.New("invalid user")
	}
	if user.StripeCustomer != "" {
		return user.StripeCustomer, nil
	}
	stripe.Key = setting.StripeApiSecret
	params := &stripe.CustomerParams{
		Email:       stripe.String(user.Email),
		Description: stripe.String(fmt.Sprintf("new-api user #%d", user.Id)),
	}
	params.AddMetadata("new_api_user_id", strconv.Itoa(user.Id))
	params.SetIdempotencyKey(fmt.Sprintf("new-api-customer-%d", user.Id))
	created, err := createStripeCustomer(params)
	if err != nil {
		return "", err
	}
	return model.SetUserStripeCustomerIfEmpty(user.Id, created.ID)
}

func CreateStripeAutoRechargeSetupSession(userId int, successURL string, cancelURL string) (string, error) {
	if !StripeAutoRechargeAvailable() {
		return "", errors.New("Stripe 自动充值未启用")
	}
	current, err := model.GetStripeAutoRechargeByUserId(userId)
	if err != nil {
		return "", err
	}
	if current != nil && (current.PendingTradeNo != "" || current.State == model.StripeAutoRechargeStateProcessing || current.State == model.StripeAutoRechargeStatePending) {
		return "", model.ErrStripeAutoRechargeBusy
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return "", err
	}
	customerId, err := ensureStripeCustomer(user)
	if err != nil {
		return "", err
	}
	stripe.Key = setting.StripeApiSecret
	referenceId := fmt.Sprintf("auto_setup_%d_%s", userId, randstr.String(10))
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		Customer:          stripe.String(customerId),
		Mode:              stripe.String(string(stripe.CheckoutSessionModeSetup)),
		PaymentMethodTypes: []*string{
			stripe.String("card"),
		},
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}
	params.AddMetadata("purpose", stripeAutoRechargePurpose)
	params.AddMetadata("user_id", strconv.Itoa(userId))
	params.SetIdempotencyKey(referenceId)
	checkoutSession, err := createStripeCheckoutSession(params)
	if err != nil {
		return "", err
	}
	return checkoutSession.URL, nil
}

func CompleteStripeAutoRechargeSetupSession(ctx context.Context, sessionId string, expectedUserId int) error {
	if !strings.HasPrefix(sessionId, "cs_") || len(sessionId) > 255 {
		return errors.New("invalid Stripe setup session")
	}
	stripe.Key = setting.StripeApiSecret
	params := &stripe.CheckoutSessionParams{}
	params.AddExpand("setup_intent")
	checkoutSession, err := retrieveStripeSetupSession(sessionId, params)
	if err != nil {
		return err
	}
	if checkoutSession.Mode != stripe.CheckoutSessionModeSetup || checkoutSession.Status != stripe.CheckoutSessionStatusComplete {
		return errors.New("Stripe card setup is not complete")
	}
	if checkoutSession.Metadata["purpose"] != stripeAutoRechargePurpose {
		return errors.New("Stripe setup purpose mismatch")
	}
	userId, err := strconv.Atoi(checkoutSession.Metadata["user_id"])
	if err != nil || userId <= 0 || (expectedUserId > 0 && userId != expectedUserId) {
		return errors.New("Stripe setup user mismatch")
	}
	if checkoutSession.Customer == nil || checkoutSession.Customer.ID == "" || checkoutSession.SetupIntent == nil || checkoutSession.SetupIntent.ID == "" {
		return errors.New("Stripe setup session is incomplete")
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		return err
	}
	if user.StripeCustomer != checkoutSession.Customer.ID {
		return errors.New("Stripe customer mismatch")
	}
	setupIntent, err := retrieveStripeSetupIntent(checkoutSession.SetupIntent.ID, nil)
	if err != nil {
		return err
	}
	if setupIntent.Status != stripe.SetupIntentStatusSucceeded || setupIntent.Customer == nil || setupIntent.Customer.ID != user.StripeCustomer || setupIntent.PaymentMethod == nil {
		return errors.New("Stripe card setup was not successful")
	}
	paymentMethod, err := retrieveStripePaymentMethod(setupIntent.PaymentMethod.ID, nil)
	if err != nil {
		return err
	}
	if paymentMethod.Type != stripe.PaymentMethodTypeCard || paymentMethod.Card == nil || paymentMethod.Customer == nil || paymentMethod.Customer.ID != user.StripeCustomer {
		return errors.New("Stripe payment method is not an attached card")
	}
	current, err := model.GetStripeAutoRechargeByUserId(userId)
	if err != nil {
		return err
	}
	thresholdQuota := int64(math.Round(10 * common.QuotaPerUnit))
	topUpAmount := int64(max(setting.StripeMinTopUp, 20))
	if current != nil {
		if current.ThresholdQuota > 0 {
			thresholdQuota = current.ThresholdQuota
		}
		if current.TopUpAmount > 0 {
			topUpAmount = current.TopUpAmount
		}
	}
	config := &model.StripeAutoRecharge{
		UserId:                userId,
		StripeCustomerId:      user.StripeCustomer,
		StripePaymentMethodId: paymentMethod.ID,
		CardBrand:             string(paymentMethod.Card.Brand),
		CardLast4:             paymentMethod.Card.Last4,
		CardExpMonth:          paymentMethod.Card.ExpMonth,
		CardExpYear:           paymentMethod.Card.ExpYear,
		ThresholdQuota:        thresholdQuota,
		TopUpAmount:           topUpAmount,
		State:                 model.StripeAutoRechargeStateIdle,
	}
	if err := model.UpsertStripeAutoRechargePaymentMethod(config); err != nil {
		return err
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe auto recharge card bound user_id=%d payment_method=%s", userId, paymentMethod.ID))
	return nil
}

func UpdateStripeAutoRecharge(userId int, enabled bool, threshold float64, topUpAmount int64, consent bool) error {
	if !StripeAutoRechargeAvailable() {
		return errors.New("Stripe 自动充值未启用")
	}
	if threshold < 1 || threshold > 10000 || math.IsNaN(threshold) || math.IsInf(threshold, 0) {
		return errors.New("自动充值阈值必须在 1 到 10000 美元之间")
	}
	if topUpAmount < int64(setting.StripeMinTopUp) || topUpAmount > 10000 {
		return fmt.Errorf("自动充值金额必须在 %d 到 10000 之间", setting.StripeMinTopUp)
	}
	if enabled && !consent {
		return errors.New("开启自动充值前必须确认自动扣款授权")
	}
	thresholdQuota := int64(math.Round(threshold * common.QuotaPerUnit))
	return model.UpdateStripeAutoRechargeSettings(userId, enabled, thresholdQuota, topUpAmount, stripeAutoRechargeConsentVersion)
}

func DeleteStripeAutoRechargePaymentMethod(userId int) error {
	config, err := model.GetStripeAutoRechargeByUserId(userId)
	if err != nil || config == nil {
		return err
	}
	if config.PendingTradeNo != "" || config.State == model.StripeAutoRechargeStateProcessing || config.State == model.StripeAutoRechargeStatePending {
		return model.ErrStripeAutoRechargeBusy
	}
	stripe.Key = setting.StripeApiSecret
	if config.StripePaymentMethodId != "" {
		if _, err := detachStripePaymentMethod(config.StripePaymentMethodId, nil); err != nil {
			return err
		}
	}
	return model.DeleteStripeAutoRecharge(userId)
}

func StripeTopUpPayMoney(amount float64, group string) float64 {
	originalAmount := amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount /= common.QuotaPerUnit
	}
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	discount := 1.0
	if configured, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(originalAmount)]; ok && configured > 0 {
		discount = configured
	}
	return amount * setting.StripeUnitPrice * topupGroupRatio * discount
}

func StripeTopUpCredit(amount float64, group string) float64 {
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	return amount * topupGroupRatio
}

func StartStripeAutoRechargeTask() {
	stripeAutoRechargeOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			logger.LogInfo(context.Background(), fmt.Sprintf("Stripe auto recharge task started: tick=%s", stripeAutoRechargeTickInterval))
			ticker := time.NewTicker(stripeAutoRechargeTickInterval)
			defer ticker.Stop()
			runStripeAutoRechargeOnce()
			for range ticker.C {
				runStripeAutoRechargeOnce()
			}
		})
	})
}

func runStripeAutoRechargeOnce() {
	if !StripeAutoRechargeAvailable() || !stripeAutoRechargeRunning.CompareAndSwap(false, true) {
		return
	}
	defer stripeAutoRechargeRunning.Store(false)
	configs, err := model.ListDueStripeAutoRecharges(stripeAutoRechargeBatchSize)
	if err != nil {
		logger.LogWarn(context.Background(), fmt.Sprintf("Stripe auto recharge scan failed: %v", err))
		return
	}
	for _, config := range configs {
		if config == nil {
			continue
		}
		claimed, err := model.ClaimStripeAutoRecharge(config.Id)
		if err != nil || !claimed {
			continue
		}
		fresh, err := model.GetStripeAutoRechargeByUserId(config.UserId)
		if err != nil || fresh == nil {
			_ = model.ReleaseStripeAutoRechargeClaim(config.Id)
			continue
		}
		processStripeAutoRecharge(context.Background(), fresh)
	}
}

func processStripeAutoRecharge(ctx context.Context, config *model.StripeAutoRecharge) {
	user, err := model.GetUserById(config.UserId, false)
	if err != nil {
		_ = model.FailStripeAutoRecharge(config.Id, "", "无法读取钱包账户", false)
		return
	}
	currentQuota, err := model.GetUserQuota(config.UserId, true)
	if err != nil || int64(currentQuota) >= config.ThresholdQuota {
		_ = model.ReleaseStripeAutoRechargeClaim(config.Id)
		return
	}
	amountCents := stripeAutoRechargeAmountCents(config.TopUpAmount)
	if amountCents < 50 {
		_ = model.FailStripeAutoRecharge(config.Id, "", "自动充值扣款金额低于 Stripe 最低金额", true)
		return
	}
	tradeNo := fmt.Sprintf("auto_ref_%d_%d_%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	topUp := &model.TopUp{
		UserId:          user.Id,
		Amount:          config.TopUpAmount,
		Money:           StripeTopUpCredit(float64(config.TopUpAmount), user.Group),
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	if err := topUp.Insert(); err != nil {
		_ = model.FailStripeAutoRecharge(config.Id, "", "创建自动充值订单失败", false)
		return
	}
	if err := model.SetStripeAutoRechargePending(config.Id, tradeNo, "", amountCents); err != nil {
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		_ = model.FailStripeAutoRecharge(config.Id, "", "保存自动充值订单失败", false)
		return
	}

	stripe.Key = setting.StripeApiSecret
	params := newStripeAutoRechargePaymentIntentParams(config, user, tradeNo, amountCents)
	paymentIntent, err := createStripePaymentIntent(params)
	if err != nil {
		var stripeErr *stripe.Error
		finalFailure := false
		message := "Stripe 自动扣款暂时失败"
		paymentIntentId := ""
		if errors.As(err, &stripeErr) {
			finalFailure = stripeErr.Type == stripe.ErrorTypeCard || stripeErr.Type == stripe.ErrorTypeInvalidRequest || stripeErr.Code == stripe.ErrorCodeAuthenticationRequired
			if stripeErr.Code == stripe.ErrorCodeAuthenticationRequired {
				message = "银行卡需要重新验证，请重新绑定后开启自动充值"
			} else if stripeErr.Type == stripe.ErrorTypeCard {
				message = "银行卡扣款失败，请检查卡片或重新绑定"
			} else if stripeErr.Type == stripe.ErrorTypeInvalidRequest {
				message = "Stripe 自动充值配置错误，请联系管理员"
			}
			if stripeErr.PaymentIntent != nil && stripeErr.PaymentIntent.ID != "" {
				paymentIntentId = stripeErr.PaymentIntent.ID
			}
		}
		if finalFailure {
			_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
			_ = model.FailStripeAutoRecharge(config.Id, tradeNo, message, true)
		} else {
			// A network/API error can be ambiguous: Stripe might have accepted the
			// idempotent request. Keep the order pending and stop further charges;
			// a later signed webhook may still safely fulfill it.
			_ = model.SuspendStripeAutoRechargePending(config.Id, tradeNo, paymentIntentId, message)
		}
		logger.LogWarn(ctx, fmt.Sprintf("Stripe auto recharge failed user_id=%d trade_no=%s error=%q", user.Id, tradeNo, err.Error()))
		return
	}
	if paymentIntent == nil || paymentIntent.ID == "" {
		_ = model.SuspendStripeAutoRechargePending(config.Id, tradeNo, "", "Stripe 未返回明确扣款结果，自动充值已暂停")
		return
	}
	if paymentIntent.Status != stripe.PaymentIntentStatusSucceeded && paymentIntent.Status != stripe.PaymentIntentStatusProcessing {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
		_ = model.FailStripeAutoRecharge(config.Id, tradeNo, "银行卡自动扣款未完成，请检查卡片后重新开启", true)
		return
	}
	if err := model.SetStripeAutoRechargePending(config.Id, tradeNo, paymentIntent.ID, amountCents); err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe auto recharge pending state failed user_id=%d trade_no=%s payment_intent=%s error=%q", user.Id, tradeNo, paymentIntent.ID, err.Error()))
		return
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe auto recharge submitted user_id=%d trade_no=%s payment_intent=%s status=%s", user.Id, tradeNo, paymentIntent.ID, paymentIntent.Status))
}

// stripeAutoRechargeAmountCents converts the user-configured USD amount
// directly to cents. Unlike manual top-up quantities, this value must not be
// multiplied by StripeUnitPrice, group ratios, or preset discounts.
func stripeAutoRechargeAmountCents(topUpAmount int64) int64 {
	return topUpAmount * 100
}

func newStripeAutoRechargePaymentIntentParams(config *model.StripeAutoRecharge, user *model.User, tradeNo string, amountCents int64) *stripe.PaymentIntentParams {
	params := &stripe.PaymentIntentParams{
		Amount:        stripe.Int64(amountCents),
		Currency:      stripe.String(string(stripe.CurrencyUSD)),
		Customer:      stripe.String(config.StripeCustomerId),
		PaymentMethod: stripe.String(config.StripePaymentMethodId),
		PaymentMethodTypes: []*string{
			stripe.String(string(stripe.PaymentMethodTypeCard)),
		},
		Confirm:               stripe.Bool(true),
		OffSession:            stripe.Bool(true),
		ErrorOnRequiresAction: stripe.Bool(true),
		Description:           stripe.String(fmt.Sprintf("Wallet auto recharge for user #%d", user.Id)),
		ReceiptEmail:          stripe.String(user.Email),
	}
	params.AddMetadata("purpose", stripeAutoRechargePurpose)
	params.AddMetadata("trade_no", tradeNo)
	params.AddMetadata("user_id", strconv.Itoa(user.Id))
	params.SetIdempotencyKey(tradeNo)
	return params
}

func HandleStripeAutoRechargePaymentSucceeded(ctx context.Context, tradeNo string, paymentIntentId string, customerId string, amountCents int64, currency string, callerIp string) error {
	if !strings.HasPrefix(tradeNo, "auto_ref_") {
		return errors.New("not an auto recharge order")
	}
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderStripe {
		return model.ErrTopUpNotFound
	}
	if topUp.Status == common.TopUpStatusSuccess {
		return model.CompleteStripeAutoRecharge(tradeNo)
	}
	config, err := model.GetStripeAutoRechargeByTradeNo(tradeNo)
	if err != nil {
		return err
	}
	if config == nil || config.StripeCustomerId != customerId || config.PendingAmountCents != amountCents || !strings.EqualFold(currency, string(stripe.CurrencyUSD)) {
		return ErrStripeAutoRechargeVerification
	}
	if config.PendingPaymentIntentId != "" && config.PendingPaymentIntentId != paymentIntentId {
		return fmt.Errorf("%w: payment intent mismatch", ErrStripeAutoRechargeVerification)
	}
	if err := model.Recharge(tradeNo, customerId, callerIp); err != nil {
		return err
	}
	if err := model.CompleteStripeAutoRecharge(tradeNo); err != nil {
		return err
	}
	logger.LogInfo(ctx, fmt.Sprintf("Stripe auto recharge completed trade_no=%s payment_intent=%s", tradeNo, paymentIntentId))
	return nil
}

func HandleStripeAutoRechargePaymentFailed(tradeNo string, message string) error {
	if !strings.HasPrefix(tradeNo, "auto_ref_") {
		return errors.New("not an auto recharge order")
	}
	err := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
	if err != nil && !errors.Is(err, model.ErrTopUpStatusInvalid) {
		return err
	}
	err = model.FailStripeAutoRechargeByTradeNo(tradeNo, message, true)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
