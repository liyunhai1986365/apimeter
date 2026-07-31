package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

type PaymentSetting struct {
	AmountOptions  []int           `json:"amount_options"`
	AmountDiscount map[int]float64 `json:"amount_discount"` // 充值金额对应的折扣，例如 100 元 0.9 表示 100 元充值享受 9 折优惠

	CryptoPaymentEnabled     bool `json:"crypto_payment_enabled"`
	CryptoOrderExpireMinutes int  `json:"crypto_order_expire_minutes"`
	CryptoUniqueAmountDigits int  `json:"crypto_unique_amount_digits"`

	CryptoEVMEnabled       bool   `json:"crypto_evm_enabled"`
	CryptoEVMNetworkName   string `json:"crypto_evm_network_name"`
	CryptoEVMRPCURL        string `json:"crypto_evm_rpc_url"`
	CryptoEVMChainID       int64  `json:"crypto_evm_chain_id"`
	CryptoEVMWalletAddress string `json:"crypto_evm_wallet_address"`
	CryptoEVMTokenContract string `json:"crypto_evm_token_contract"`
	CryptoEVMTokenSymbol   string `json:"crypto_evm_token_symbol"`
	CryptoEVMTokenDecimals int    `json:"crypto_evm_token_decimals"`
	CryptoEVMTokenPerUSD   string `json:"crypto_evm_token_per_usd"`
	CryptoEVMConfirmations int64  `json:"crypto_evm_confirmations"`

	CryptoTronEnabled             bool   `json:"crypto_tron_enabled"`
	CryptoTronNetworkName         string `json:"crypto_tron_network_name"`
	CryptoTronAPIURL              string `json:"crypto_tron_api_url"`
	CryptoTronAPIKey              string `json:"crypto_tron_api_key"`
	CryptoTronWalletAddress       string `json:"crypto_tron_wallet_address"`
	CryptoTronTokenContract       string `json:"crypto_tron_token_contract"`
	CryptoTronTokenSymbol         string `json:"crypto_tron_token_symbol"`
	CryptoTronTokenDecimals       int    `json:"crypto_tron_token_decimals"`
	CryptoTronTokenPerUSD         string `json:"crypto_tron_token_per_usd"`
	CryptoTronConfirmationSeconds int64  `json:"crypto_tron_confirmation_seconds"`

	ComplianceConfirmed    bool   `json:"compliance_confirmed"`
	ComplianceTermsVersion string `json:"compliance_terms_version"`
	ComplianceConfirmedAt  int64  `json:"compliance_confirmed_at"`
	ComplianceConfirmedBy  int    `json:"compliance_confirmed_by"`
	ComplianceConfirmedIP  string `json:"compliance_confirmed_ip"`
}

const CurrentComplianceTermsVersion = "v1"

// 默认配置
var paymentSetting = PaymentSetting{
	AmountOptions:                 []int{10, 20, 50, 100, 200, 500},
	AmountDiscount:                map[int]float64{},
	CryptoOrderExpireMinutes:      30,
	CryptoUniqueAmountDigits:      3,
	CryptoEVMNetworkName:          "EVM",
	CryptoEVMTokenSymbol:          "USDT",
	CryptoEVMTokenDecimals:        6,
	CryptoEVMTokenPerUSD:          "1",
	CryptoEVMConfirmations:        12,
	CryptoTronNetworkName:         "TRON",
	CryptoTronAPIURL:              "https://api.trongrid.io",
	CryptoTronTokenSymbol:         "USDT",
	CryptoTronTokenDecimals:       6,
	CryptoTronTokenPerUSD:         "1",
	CryptoTronConfirmationSeconds: 60,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("payment_setting", &paymentSetting)
}

func GetPaymentSetting() *PaymentSetting {
	return &paymentSetting
}

func GetCryptoAmountDecimals() int {
	if paymentSetting.CryptoUniqueAmountDigits < 1 || paymentSetting.CryptoUniqueAmountDigits > 3 {
		return 3
	}
	return paymentSetting.CryptoUniqueAmountDigits
}

func IsPaymentComplianceConfirmed() bool {
	return paymentSetting.ComplianceConfirmed &&
		paymentSetting.ComplianceTermsVersion == CurrentComplianceTermsVersion
}
