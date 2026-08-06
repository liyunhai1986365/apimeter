package controller

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type CryptoPaymentRequest struct {
	Amount  int64  `json:"amount"`
	Network string `json:"network"`
}

type CryptoPaymentConfigRequest struct {
	Enabled            bool `json:"enabled"`
	OrderExpireMinutes int  `json:"order_expire_minutes"`
	UniqueAmountDigits int  `json:"unique_amount_digits"`

	EVMEnabled       bool   `json:"evm_enabled"`
	EVMNetworkName   string `json:"evm_network_name"`
	EVMRPCURL        string `json:"evm_rpc_url"`
	EVMChainID       int64  `json:"evm_chain_id"`
	EVMWalletAddress string `json:"evm_wallet_address"`
	EVMTokenContract string `json:"evm_token_contract"`
	EVMTokenSymbol   string `json:"evm_token_symbol"`
	EVMTokenDecimals int    `json:"evm_token_decimals"`
	EVMTokenPerUSD   string `json:"evm_token_per_usd"`
	EVMConfirmations int64  `json:"evm_confirmations"`

	TronEnabled             bool   `json:"tron_enabled"`
	TronNetworkName         string `json:"tron_network_name"`
	TronAPIURL              string `json:"tron_api_url"`
	TronAPIKey              string `json:"tron_api_key"`
	TronWalletAddress       string `json:"tron_wallet_address"`
	TronTokenContract       string `json:"tron_token_contract"`
	TronTokenSymbol         string `json:"tron_token_symbol"`
	TronTokenDecimals       int    `json:"tron_token_decimals"`
	TronTokenPerUSD         string `json:"tron_token_per_usd"`
	TronConfirmationSeconds int64  `json:"tron_confirmation_seconds"`
}

func cryptoPaymentConfigResponse() gin.H {
	setting := operation_setting.GetPaymentSetting()
	return gin.H{
		"enabled":                   setting.CryptoPaymentEnabled,
		"order_expire_minutes":      setting.CryptoOrderExpireMinutes,
		"unique_amount_digits":      operation_setting.GetCryptoAmountDecimals(),
		"evm_enabled":               setting.CryptoEVMEnabled,
		"evm_network_name":          setting.CryptoEVMNetworkName,
		"evm_rpc_configured":        strings.TrimSpace(setting.CryptoEVMRPCURL) != "",
		"evm_chain_id":              setting.CryptoEVMChainID,
		"evm_wallet_address":        setting.CryptoEVMWalletAddress,
		"evm_token_contract":        setting.CryptoEVMTokenContract,
		"evm_token_symbol":          setting.CryptoEVMTokenSymbol,
		"evm_token_decimals":        setting.CryptoEVMTokenDecimals,
		"evm_token_per_usd":         setting.CryptoEVMTokenPerUSD,
		"evm_confirmations":         setting.CryptoEVMConfirmations,
		"tron_enabled":              setting.CryptoTronEnabled,
		"tron_network_name":         setting.CryptoTronNetworkName,
		"tron_api_url":              setting.CryptoTronAPIURL,
		"tron_api_key_configured":   strings.TrimSpace(setting.CryptoTronAPIKey) != "",
		"tron_wallet_address":       setting.CryptoTronWalletAddress,
		"tron_token_contract":       setting.CryptoTronTokenContract,
		"tron_token_symbol":         setting.CryptoTronTokenSymbol,
		"tron_token_decimals":       setting.CryptoTronTokenDecimals,
		"tron_token_per_usd":        setting.CryptoTronTokenPerUSD,
		"tron_confirmation_seconds": setting.CryptoTronConfirmationSeconds,
	}
}

func GetCryptoPaymentConfig(c *gin.Context) {
	common.ApiSuccess(c, cryptoPaymentConfigResponse())
}

func SaveCryptoPaymentConfig(c *gin.Context) {
	request := CryptoPaymentConfigRequest{}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "无效的加密货币支付配置")
		return
	}
	if request.Enabled && !operation_setting.IsPaymentComplianceConfirmed() {
		common.ApiErrorMsg(c, "请先完成支付合规确认")
		return
	}
	if request.OrderExpireMinutes < 5 || request.OrderExpireMinutes > 1440 {
		common.ApiErrorMsg(c, "支付订单有效期必须在 5 到 1440 分钟之间")
		return
	}
	if request.UniqueAmountDigits < 1 || request.UniqueAmountDigits > 3 {
		common.ApiErrorMsg(c, "支付金额小数位数必须在 1 到 3 位之间")
		return
	}
	if request.Enabled && !request.EVMEnabled && !request.TronEnabled {
		common.ApiErrorMsg(c, "至少启用一个 EVM 或 TRON 支付网络")
		return
	}

	current := operation_setting.GetPaymentSetting()
	evmRPCURL := strings.TrimSpace(request.EVMRPCURL)
	if evmRPCURL == "" {
		evmRPCURL = current.CryptoEVMRPCURL
	}
	if request.EVMEnabled {
		var err error
		evmRPCURL, err = service.NormalizeEVMRPCURLs(evmRPCURL)
		if err != nil {
			common.ApiErrorMsg(c, "EVM 配置错误："+err.Error())
			return
		}
	}
	tronAPIKey := strings.TrimSpace(request.TronAPIKey)
	if tronAPIKey == "" {
		tronAPIKey = current.CryptoTronAPIKey
	}

	evmConfig := service.CryptoNetworkConfig{
		Enabled:       request.EVMEnabled,
		NetworkType:   model.CryptoNetworkEVM,
		NetworkName:   strings.TrimSpace(request.EVMNetworkName),
		ChainId:       strconv.FormatInt(request.EVMChainID, 10),
		RPCURL:        evmRPCURL,
		WalletAddress: strings.TrimSpace(request.EVMWalletAddress),
		TokenContract: strings.TrimSpace(request.EVMTokenContract),
		TokenSymbol:   strings.TrimSpace(request.EVMTokenSymbol),
		TokenDecimals: request.EVMTokenDecimals,
		TokenPerUSD:   strings.TrimSpace(request.EVMTokenPerUSD),
		Confirmations: request.EVMConfirmations,
	}
	if request.EVMEnabled {
		if err := service.ValidateCryptoNetworkConfig(evmConfig, request.UniqueAmountDigits); err != nil {
			common.ApiErrorMsg(c, "EVM 配置错误："+err.Error())
			return
		}
	}

	tronConfig := service.CryptoNetworkConfig{
		Enabled:             request.TronEnabled,
		NetworkType:         model.CryptoNetworkTron,
		NetworkName:         strings.TrimSpace(request.TronNetworkName),
		ChainId:             model.CryptoNetworkTron,
		RPCURL:              strings.TrimRight(strings.TrimSpace(request.TronAPIURL), "/"),
		APIKey:              tronAPIKey,
		WalletAddress:       strings.TrimSpace(request.TronWalletAddress),
		TokenContract:       strings.TrimSpace(request.TronTokenContract),
		TokenSymbol:         strings.TrimSpace(request.TronTokenSymbol),
		TokenDecimals:       request.TronTokenDecimals,
		TokenPerUSD:         strings.TrimSpace(request.TronTokenPerUSD),
		ConfirmationSeconds: request.TronConfirmationSeconds,
	}
	if request.TronEnabled {
		if err := service.ValidateCryptoNetworkConfig(tronConfig, request.UniqueAmountDigits); err != nil {
			common.ApiErrorMsg(c, "TRON 配置错误："+err.Error())
			return
		}
	}

	values := map[string]string{
		"payment_setting.crypto_payment_enabled":           strconv.FormatBool(request.Enabled),
		"payment_setting.crypto_order_expire_minutes":      strconv.Itoa(request.OrderExpireMinutes),
		"payment_setting.crypto_unique_amount_digits":      strconv.Itoa(request.UniqueAmountDigits),
		"payment_setting.crypto_evm_enabled":               strconv.FormatBool(request.EVMEnabled),
		"payment_setting.crypto_evm_network_name":          evmConfig.NetworkName,
		"payment_setting.crypto_evm_rpc_url":               evmRPCURL,
		"payment_setting.crypto_evm_chain_id":              evmConfig.ChainId,
		"payment_setting.crypto_evm_wallet_address":        evmConfig.WalletAddress,
		"payment_setting.crypto_evm_token_contract":        evmConfig.TokenContract,
		"payment_setting.crypto_evm_token_symbol":          evmConfig.TokenSymbol,
		"payment_setting.crypto_evm_token_decimals":        strconv.Itoa(evmConfig.TokenDecimals),
		"payment_setting.crypto_evm_token_per_usd":         evmConfig.TokenPerUSD,
		"payment_setting.crypto_evm_confirmations":         strconv.FormatInt(evmConfig.Confirmations, 10),
		"payment_setting.crypto_tron_enabled":              strconv.FormatBool(request.TronEnabled),
		"payment_setting.crypto_tron_network_name":         tronConfig.NetworkName,
		"payment_setting.crypto_tron_api_url":              tronConfig.RPCURL,
		"payment_setting.crypto_tron_api_key":              tronAPIKey,
		"payment_setting.crypto_tron_wallet_address":       tronConfig.WalletAddress,
		"payment_setting.crypto_tron_token_contract":       tronConfig.TokenContract,
		"payment_setting.crypto_tron_token_symbol":         tronConfig.TokenSymbol,
		"payment_setting.crypto_tron_token_decimals":       strconv.Itoa(tronConfig.TokenDecimals),
		"payment_setting.crypto_tron_token_per_usd":        tronConfig.TokenPerUSD,
		"payment_setting.crypto_tron_confirmation_seconds": strconv.FormatInt(tronConfig.ConfirmationSeconds, 10),
	}
	if err := model.UpdateOptionsBulk(values); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, cryptoPaymentConfigResponse())
}

func getCryptoPaymentUSD(amount int64, group string) decimal.Decimal {
	value := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		value = value.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	groupRatio := common.GetTopupGroupRatio(group)
	if groupRatio == 0 {
		groupRatio = 1
	}
	discount := 1.0
	if configured, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && configured > 0 {
		discount = configured
	}
	localPaymentAmount := value.
		Mul(decimal.NewFromFloat(operation_setting.Price)).
		Mul(decimal.NewFromFloat(groupRatio)).
		Mul(decimal.NewFromFloat(discount))
	// Price is the site's local-currency recharge price. Stablecoin rates are
	// configured per USD, so convert the checkout amount back to USD first.
	usdExchangeRate := operation_setting.USDExchangeRate
	if usdExchangeRate <= 0 {
		usdExchangeRate = 1
	}
	return localPaymentAmount.Div(decimal.NewFromFloat(usdExchangeRate))
}

func normalizeCryptoTopUpAmount(amount int64) int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount
	}
	return decimal.NewFromInt(amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
}

func prepareCryptoPayment(c *gin.Context, request CryptoPaymentRequest) (service.CryptoNetworkConfig, decimal.Decimal, bool) {
	request.Network = strings.ToLower(strings.TrimSpace(request.Network))
	if !service.IsCryptoPaymentAvailable(request.Network) {
		common.ApiErrorMsg(c, "当前加密货币支付网络未启用或配置不完整")
		return service.CryptoNetworkConfig{}, decimal.Zero, false
	}
	if request.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("充值数量不能小于 %d", getMinTopup()))
		return service.CryptoNetworkConfig{}, decimal.Zero, false
	}
	if normalizeCryptoTopUpAmount(request.Amount) <= 0 {
		common.ApiErrorMsg(c, "充值数量必须大于 0")
		return service.CryptoNetworkConfig{}, decimal.Zero, false
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		common.ApiErrorMsg(c, "获取用户分组失败")
		return service.CryptoNetworkConfig{}, decimal.Zero, false
	}
	config, err := service.GetCryptoNetworkConfig(request.Network)
	if err != nil {
		common.ApiError(c, err)
		return service.CryptoNetworkConfig{}, decimal.Zero, false
	}
	return config, getCryptoPaymentUSD(request.Amount, group), true
}

func RequestCryptoPaymentAmount(c *gin.Context) {
	request := CryptoPaymentRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	config, usdAmount, ok := prepareCryptoPayment(c, request)
	if !ok {
		return
	}
	_, displayAmount, err := service.CryptoTokenAmountFromUSD(usdAmount, config)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, displayAmount)
}

func RequestCryptoPayment(c *gin.Context) {
	request := CryptoPaymentRequest{}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	config, usdAmount, ok := prepareCryptoPayment(c, request)
	if !ok {
		return
	}
	baseAtomic, _, err := service.CryptoTokenAmountFromUSD(usdAmount, config)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	startBlock := int64(0)
	if config.NetworkType == model.CryptoNetworkEVM {
		startBlock, err = service.GetEVMStartBlock(context.Background(), config)
		if err != nil {
			common.ApiErrorMsg(c, "EVM 节点当前不可用，请稍后重试")
			return
		}
	}

	now := time.Now()
	tradeNo := fmt.Sprintf("CRY%d%s%d", c.GetInt("id"), common.GetRandomString(8), now.Unix())
	topUp := &model.TopUp{
		UserId:          c.GetInt("id"),
		Amount:          normalizeCryptoTopUpAmount(request.Amount),
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodCryptoEVM,
		PaymentProvider: model.PaymentProviderCrypto,
		CreateTime:      now.Unix(),
		Status:          common.TopUpStatusPending,
	}
	if config.NetworkType == model.CryptoNetworkTron {
		topUp.PaymentMethod = model.PaymentMethodCryptoTron
	}

	setting := operation_setting.GetPaymentSetting()
	payment, err := model.CreateCryptoPaymentOrder(model.CreateCryptoPaymentParams{
		TopUp:            topUp,
		NetworkType:      config.NetworkType,
		NetworkName:      config.NetworkName,
		ChainId:          config.ChainId,
		WalletAddress:    config.WalletAddress,
		TokenContract:    config.TokenContract,
		TokenSymbol:      config.TokenSymbol,
		TokenDecimals:    config.TokenDecimals,
		BaseAtomicAmount: baseAtomic,
		UniqueDigits:     operation_setting.GetCryptoAmountDecimals(),
		StartBlock:       startBlock,
		CreateTimeMillis: now.UnixMilli(),
		ExpiresAt:        now.Add(time.Duration(setting.CryptoOrderExpireMinutes) * time.Minute).Unix(),
		PaymentUri: func(atomicAmount string) string {
			if config.NetworkType != model.CryptoNetworkEVM {
				return ""
			}
			return fmt.Sprintf(
				"ethereum:%s@%s/transfer?address=%s&uint256=%s",
				config.TokenContract,
				config.ChainId,
				config.WalletAddress,
				atomicAmount,
			)
		},
		QrContent: func(atomicAmount string, _ string) string {
			if config.NetworkType == model.CryptoNetworkEVM {
				return fmt.Sprintf(
					"ethereum:%s@%s/transfer?address=%s&uint256=%s",
					config.TokenContract,
					config.ChainId,
					config.WalletAddress,
					atomicAmount,
				)
			}
			return config.WalletAddress
		},
	})
	if err != nil {
		common.ApiErrorMsg(c, "创建链上支付订单失败")
		return
	}
	common.ApiSuccess(c, payment)
}

func GetCryptoPaymentOrder(c *gin.Context) {
	tradeNo := strings.TrimSpace(c.Param("trade_no"))
	if tradeNo == "" {
		common.ApiErrorMsg(c, "订单号不能为空")
		return
	}
	payment, err := model.GetCryptoPaymentForUser(tradeNo, c.GetInt("id"))
	if err != nil {
		common.ApiErrorMsg(c, "支付订单不存在")
		return
	}
	common.ApiSuccess(c, payment)
}
