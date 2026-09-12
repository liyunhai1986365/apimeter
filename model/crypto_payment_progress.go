package model

import "github.com/QuantumNous/new-api/common"

// CryptoPaymentProgress is an observation, never an instruction to credit funds.
// Store it in the shared database so polling through another node sees the same
// scanner result. It contains no RPC URLs, credentials, or provider errors.
type CryptoPaymentProgress struct {
	Stage                 string `json:"stage"`
	CheckedAt             int64  `json:"checked_at"`
	TransactionHash       string `json:"transaction_hash,omitempty"`
	TransactionTime       int64  `json:"transaction_time,omitempty"`
	BlockNumber           int64  `json:"block_number,omitempty"`
	HeadBlock             int64  `json:"head_block,omitempty"`
	Confirmations         int64  `json:"confirmations"`
	RequiredConfirmations int64  `json:"required_confirmations"`
	ConfirmedSeconds      int64  `json:"confirmed_seconds"`
	RequiredSeconds       int64  `json:"required_seconds"`
	Retrying              bool   `json:"retrying"`
	Stale                 bool   `json:"stale"`
}

func (payment *CryptoPayment) ReadProgress() CryptoPaymentProgress {
	var progress CryptoPaymentProgress
	if payment.ProgressSnapshot != "" {
		// Old orders and invalid snapshots simply start without an observation.
		if common.UnmarshalJsonStr(payment.ProgressSnapshot, &progress) != nil {
			return CryptoPaymentProgress{}
		}
	}
	return progress
}

func UpdateCryptoPaymentProgress(payment *CryptoPayment, progress CryptoPaymentProgress) error {
	data, err := common.Marshal(progress)
	if err != nil {
		return err
	}
	result := DB.Model(&CryptoPayment{}).
		Where("id = ? AND status = ?", payment.Id, CryptoPaymentStatusPending).
		Update("progress_snapshot", string(data))
	if result.Error == nil && result.RowsAffected > 0 {
		payment.ProgressSnapshot = string(data)
	}
	return result.Error
}
