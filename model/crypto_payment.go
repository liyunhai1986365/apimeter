package model

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	CryptoNetworkEVM  = "evm"
	CryptoNetworkTron = "tron"

	CryptoPaymentStatusPending = "pending"
	CryptoPaymentStatusSuccess = "success"
	CryptoPaymentStatusExpired = "expired"
	CryptoPaymentStatusManual  = "manual"

	PaymentMethodCryptoEVM  = "crypto_evm"
	PaymentMethodCryptoTron = "crypto_tron"
	PaymentProviderCrypto   = "crypto"
)

type CryptoPayment struct {
	Id                    int     `json:"id"`
	UserId                int     `json:"user_id" gorm:"index"`
	TradeNo               string  `json:"trade_no" gorm:"uniqueIndex;type:varchar(255)"`
	NetworkType           string  `json:"network_type" gorm:"index;type:varchar(20)"`
	NetworkName           string  `json:"network_name" gorm:"type:varchar(100)"`
	ChainId               string  `json:"chain_id" gorm:"type:varchar(80)"`
	WalletAddress         string  `json:"wallet_address" gorm:"type:varchar(255)"`
	TokenContract         string  `json:"token_contract" gorm:"type:varchar(255)"`
	TokenSymbol           string  `json:"token_symbol" gorm:"type:varchar(50)"`
	TokenDecimals         int     `json:"token_decimals"`
	RequestedAmount       string  `json:"requested_amount" gorm:"type:varchar(100)"`
	DisplayAmount         string  `json:"display_amount" gorm:"type:varchar(100)"`
	PaymentUri            string  `json:"payment_uri" gorm:"type:text"`
	QrContent             string  `json:"qr_content" gorm:"type:text"`
	Status                string  `json:"status" gorm:"index;type:varchar(20)"`
	StartBlock            int64   `json:"start_block"`
	ScanFromBlock         int64   `json:"-"`
	CreateTime            int64   `json:"create_time"`
	CreateTimeMillis      int64   `json:"-"`
	ExpiresAt             int64   `json:"expires_at" gorm:"index"`
	CompleteTime          int64   `json:"complete_time"`
	TransactionHash       string  `json:"transaction_hash,omitempty" gorm:"type:varchar(255)"`
	TransactionEventIndex string  `json:"transaction_event_index,omitempty" gorm:"type:varchar(80)"`
	BlockNumber           int64   `json:"block_number,omitempty"`
	ReservationKey        *string `json:"-" gorm:"uniqueIndex;type:varchar(64)"`
	TransactionKey        *string `json:"-" gorm:"uniqueIndex;type:varchar(64)"`
}

type CreateCryptoPaymentParams struct {
	TopUp            *TopUp
	NetworkType      string
	NetworkName      string
	ChainId          string
	WalletAddress    string
	TokenContract    string
	TokenSymbol      string
	TokenDecimals    int
	BaseAtomicAmount string
	UniqueDigits     int
	PaymentUri       func(atomicAmount string) string
	QrContent        func(atomicAmount string, displayAmount string) string
	StartBlock       int64
	CreateTimeMillis int64
	ExpiresAt        int64
}

func cryptoPaymentReservationKey(networkType, chainId, walletAddress, tokenContract, requestedAmount string) string {
	return cryptoPaymentKey(strings.Join([]string{
		networkType,
		chainId,
		walletAddress,
		tokenContract,
		requestedAmount,
	}, ":"))
}

func cryptoPaymentKey(value string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.ToLower(value))))
}

func isDuplicateConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "unique violation")
}

func uniqueAmountCandidate(baseAtomic string, tokenDecimals, amountDecimals, offset int) (string, error) {
	base, ok := new(big.Int).SetString(baseAtomic, 10)
	if !ok || base.Sign() <= 0 {
		return "", errors.New("invalid base atomic amount")
	}
	if amountDecimals < 1 || amountDecimals > 3 || amountDecimals > tokenDecimals {
		return "", errors.New("payment amount decimals must be between 1 and 3 and not exceed token decimals")
	}
	if offset < 0 {
		return "", errors.New("payment amount offset must not be negative")
	}

	step := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(tokenDecimals-amountDecimals)), nil)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(base, step, remainder)
	candidate := new(big.Int).Mul(quotient, step)
	// Always move to the next representable amount. For an exact integer base
	// this starts at .001 instead of .000; a non-aligned base is rounded up so
	// the payment is never lower than the calculated token amount.
	candidate.Add(candidate, step)
	if offset > 0 {
		candidate.Add(candidate, new(big.Int).Mul(big.NewInt(int64(offset)), step))
	}
	return candidate.String(), nil
}

func formatAtomicAmount(atomicAmount string, decimals, displayDecimals int) (string, error) {
	value, ok := new(big.Int).SetString(atomicAmount, 10)
	if !ok || value.Sign() < 0 {
		return "", errors.New("invalid atomic amount")
	}
	if decimals < 0 || decimals > 36 {
		return "", errors.New("invalid token decimals")
	}
	if displayDecimals < 0 || displayDecimals > decimals {
		return "", errors.New("invalid display decimals")
	}
	if decimals == 0 {
		return value.String(), nil
	}

	digits := value.String()
	if len(digits) <= decimals {
		digits = strings.Repeat("0", decimals-len(digits)+1) + digits
	}
	integerPart := digits[:len(digits)-decimals]
	if displayDecimals == 0 {
		return integerPart, nil
	}
	fractionalPart := digits[len(digits)-decimals : len(digits)-decimals+displayDecimals]
	return integerPart + "." + fractionalPart, nil
}

func CreateCryptoPaymentOrder(params CreateCryptoPaymentParams) (*CryptoPayment, error) {
	if params.TopUp == nil {
		return nil, errors.New("topup is required")
	}
	if params.UniqueDigits < 1 || params.UniqueDigits > 3 || params.UniqueDigits > params.TokenDecimals {
		return nil, errors.New("payment amount decimals must be between 1 and 3 and not exceed token decimals")
	}

	maxAttempts := 1
	for digit := 0; digit < params.UniqueDigits; digit++ {
		maxAttempts *= 10
	}
	maxAttempts--
	for attempt := 0; attempt < maxAttempts; attempt++ {
		requestedAmount, err := uniqueAmountCandidate(
			params.BaseAtomicAmount,
			params.TokenDecimals,
			params.UniqueDigits,
			attempt,
		)
		if err != nil {
			return nil, err
		}
		displayAmount, err := formatAtomicAmount(requestedAmount, params.TokenDecimals, params.UniqueDigits)
		if err != nil {
			return nil, err
		}
		reservationKey := cryptoPaymentReservationKey(
			params.NetworkType,
			params.ChainId,
			params.WalletAddress,
			params.TokenContract,
			requestedAmount,
		)
		payment := &CryptoPayment{
			UserId:           params.TopUp.UserId,
			TradeNo:          params.TopUp.TradeNo,
			NetworkType:      params.NetworkType,
			NetworkName:      params.NetworkName,
			ChainId:          params.ChainId,
			WalletAddress:    params.WalletAddress,
			TokenContract:    params.TokenContract,
			TokenSymbol:      params.TokenSymbol,
			TokenDecimals:    params.TokenDecimals,
			RequestedAmount:  requestedAmount,
			DisplayAmount:    displayAmount,
			Status:           CryptoPaymentStatusPending,
			StartBlock:       params.StartBlock,
			ScanFromBlock:    params.StartBlock,
			CreateTime:       params.TopUp.CreateTime,
			CreateTimeMillis: params.CreateTimeMillis,
			ExpiresAt:        params.ExpiresAt,
			ReservationKey:   &reservationKey,
		}
		if params.PaymentUri != nil {
			payment.PaymentUri = params.PaymentUri(requestedAmount)
		}
		if params.QrContent != nil {
			payment.QrContent = params.QrContent(requestedAmount, displayAmount)
		}

		displayDecimal, _ := decimal.NewFromString(displayAmount)
		params.TopUp.Money, _ = displayDecimal.Float64()
		params.TopUp.Id = 0
		err = DB.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(params.TopUp).Error; err != nil {
				return err
			}
			return tx.Create(payment).Error
		})
		if err == nil {
			return payment, nil
		}
		if !isDuplicateConstraintError(err) {
			return nil, err
		}
	}

	return nil, errors.New("unable to reserve a unique payment amount")
}

func GetCryptoPaymentForUser(tradeNo string, userId int) (*CryptoPayment, error) {
	payment := &CryptoPayment{}
	if err := DB.Where("trade_no = ? AND user_id = ?", tradeNo, userId).First(payment).Error; err != nil {
		return nil, err
	}
	return payment, nil
}

func ListPendingCryptoPayments(limit int) ([]*CryptoPayment, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	var payments []*CryptoPayment
	err := DB.Where("status = ?", CryptoPaymentStatusPending).
		Order("id asc").
		Limit(limit).
		Find(&payments).Error
	return payments, err
}

func UpdateCryptoPaymentScanBlock(paymentId int, nextBlock int64) error {
	if paymentId <= 0 || nextBlock <= 0 {
		return nil
	}
	return DB.Model(&CryptoPayment{}).
		Where("id = ? AND status = ? AND scan_from_block < ?", paymentId, CryptoPaymentStatusPending, nextBlock).
		Update("scan_from_block", nextBlock).Error
}

// ExpireCryptoPayments expires only orders whose network scanner has safely
// covered the payment window. EVM orders remain pending while their cursor is
// behind the last successfully confirmed head, so a temporary RPC outage does
// not permanently discard a valid on-chain transfer.
func ExpireCryptoPayments(now, evmScannedThrough int64, tronScanComplete bool) error {
	var payments []*CryptoPayment
	if err := DB.Where("status = ? AND expires_at <= ?", CryptoPaymentStatusPending, now).
		Limit(500).
		Find(&payments).Error; err != nil {
		return err
	}
	if len(payments) == 0 {
		return nil
	}

	ids := make([]int, 0, len(payments))
	tradeNos := make([]string, 0, len(payments))
	for _, payment := range payments {
		switch payment.NetworkType {
		case CryptoNetworkEVM:
			if evmScannedThrough <= 0 || payment.ScanFromBlock <= evmScannedThrough {
				continue
			}
		case CryptoNetworkTron:
			if !tronScanComplete {
				continue
			}
		}
		ids = append(ids, payment.Id)
		tradeNos = append(tradeNos, payment.TradeNo)
	}
	if len(ids) == 0 {
		return nil
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&CryptoPayment{}).
			Where("id IN ? AND status = ?", ids, CryptoPaymentStatusPending).
			Updates(map[string]interface{}{
				"status":          CryptoPaymentStatusExpired,
				"reservation_key": nil,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&TopUp{}).
			Where("trade_no IN ? AND payment_provider = ? AND status = ?", tradeNos, PaymentProviderCrypto, common.TopUpStatusPending).
			Update("status", common.TopUpStatusFailed).Error
	})
}

func MarkCryptoPaymentManuallyCompleted(tx *gorm.DB, tradeNo string, completeTime int64) error {
	if tx == nil || strings.TrimSpace(tradeNo) == "" {
		return nil
	}
	return tx.Model(&CryptoPayment{}).
		Where("trade_no = ? AND status IN ?", tradeNo, []string{CryptoPaymentStatusPending, CryptoPaymentStatusExpired}).
		Updates(map[string]interface{}{
			"status":          CryptoPaymentStatusManual,
			"complete_time":   completeTime,
			"reservation_key": nil,
		}).Error
}

func CompleteCryptoPayment(tradeNo, transactionHash, eventIndex string, blockNumber int64) error {
	if tradeNo == "" || transactionHash == "" {
		return errors.New("missing crypto payment reference")
	}

	var completedTopUp *TopUp
	var quotaToAdd int
	eventKey := cryptoPaymentKey(strings.Join([]string{transactionHash, eventIndex}, ":"))

	err := DB.Transaction(func(tx *gorm.DB) error {
		payment := &CryptoPayment{}
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(payment).Error; err != nil {
			return err
		}
		if payment.Status == CryptoPaymentStatusSuccess {
			return nil
		}
		if payment.Status != CryptoPaymentStatusPending {
			return ErrTopUpStatusInvalid
		}

		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
			return err
		}
		if topUp.PaymentProvider != PaymentProviderCrypto {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		quotaDecimal := decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit))
		quotaToAdd = int(quotaDecimal.IntPart())
		if quotaToAdd <= 0 {
			return errors.New("invalid crypto topup quota")
		}

		now := common.GetTimestamp()
		payment.Status = CryptoPaymentStatusSuccess
		payment.CompleteTime = now
		payment.TransactionHash = transactionHash
		payment.TransactionEventIndex = eventIndex
		payment.BlockNumber = blockNumber
		payment.TransactionKey = &eventKey
		payment.ReservationKey = nil
		if err := tx.Save(payment).Error; err != nil {
			return err
		}

		topUp.Status = common.TopUpStatusSuccess
		topUp.CompleteTime = now
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).
			Where("id = ?", topUp.UserId).
			Update("quota", gorm.Expr("quota + ?", quotaToAdd)).Error; err != nil {
			return err
		}
		completedTopUp = topUp
		return nil
	})
	if err != nil {
		return err
	}
	if completedTopUp == nil {
		return nil
	}

	RecordTopupLog(
		completedTopUp.UserId,
		fmt.Sprintf("链上充值成功，充值额度: %v，链上支付金额: %.8f", logger.FormatQuota(quotaToAdd), completedTopUp.Money),
		"system",
		completedTopUp.PaymentMethod,
		PaymentProviderCrypto,
	)
	createAffiliateTopUpRewardAfterSuccess(completedTopUp, quotaToAdd)
	return nil
}
