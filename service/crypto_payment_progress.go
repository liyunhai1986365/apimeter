package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type CryptoPaymentOrderResponse struct {
	*model.CryptoPayment
	Progress model.CryptoPaymentProgress `json:"progress"`
}

// CryptoPaymentOrderWithProgress only reads the scanner snapshot. User polling
// must not trigger upstream calls or change payment/settlement state.
func CryptoPaymentOrderWithProgress(payment *model.CryptoPayment) CryptoPaymentOrderResponse {
	progress := payment.ReadProgress()
	if progress.Stage == "" {
		progress.Stage = "checking"
		if config, err := GetCryptoNetworkConfig(payment.NetworkType); err == nil {
			progress.RequiredConfirmations = config.Confirmations
			progress.RequiredSeconds = config.ConfirmationSeconds
		}
	}
	switch payment.Status {
	case model.CryptoPaymentStatusSuccess, model.CryptoPaymentStatusManual:
		progress.Stage = "completed"
		progress.Retrying = false
		progress.Stale = false
	case model.CryptoPaymentStatusExpired:
		progress.Stage = "expired"
		progress.Retrying = false
		progress.Stale = false
	default:
		lastChecked := progress.CheckedAt
		if lastChecked == 0 {
			lastChecked = payment.CreateTime
		}
		progress.Stale = common.GetTimestamp()-lastChecked > 60
	}
	return CryptoPaymentOrderResponse{CryptoPayment: payment, Progress: progress}
}

func saveCryptoPaymentProgress(payment *model.CryptoPayment, progress model.CryptoPaymentProgress) {
	if err := model.UpdateCryptoPaymentProgress(payment, progress); err != nil {
		common.SysError("failed to update crypto payment progress: " + err.Error())
	}
}

func markCryptoPaymentProgressRetrying(payments []*model.CryptoPayment, network string) {
	for _, payment := range payments {
		if payment.NetworkType != network {
			continue
		}
		progress := payment.ReadProgress()
		if progress.Retrying {
			continue
		}
		progress.Retrying = true
		saveCryptoPaymentProgress(payment, progress)
	}
}
