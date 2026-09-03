package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/shopspring/decimal"
)

const (
	cryptoPaymentPollInterval   = 12 * time.Second
	cryptoPaymentRequestTimeout = 15 * time.Second
	evmScannerLogInterval       = 5 * time.Minute
	maxCryptoResponseBytes      = 8 << 20
	maxEVMBlockRange            = int64(2000)
	evmScanOverlapBlocks        = int64(32)
	maxEVMRPCEndpoints          = 5
	maxTronPagesPerScan         = 50
	erc20TransferTopic          = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
)

var (
	evmAddressPattern          = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
	evmRPCURLInErrorPattern    = regexp.MustCompile(`https?://[^\s"']+`)
	tronAddressPattern         = regexp.MustCompile(`^T[1-9A-HJ-NP-Za-km-z]{33}$`)
	startCryptoPaymentTaskOnce sync.Once
	evmScannerLogNow           = time.Now
	evmScannerHealth           = struct {
		sync.Mutex
		unhealthy      bool
		lastFailureLog time.Time
		lastFallback   time.Time
	}{}
)

type CryptoNetworkConfig struct {
	Enabled             bool
	NetworkType         string
	NetworkName         string
	ChainId             string
	RPCURL              string
	APIKey              string
	WalletAddress       string
	TokenContract       string
	TokenSymbol         string
	TokenDecimals       int
	TokenPerUSD         string
	Confirmations       int64
	ConfirmationSeconds int64
}

func GetCryptoNetworkConfig(networkType string) (CryptoNetworkConfig, error) {
	setting := operation_setting.GetPaymentSetting()
	switch strings.ToLower(strings.TrimSpace(networkType)) {
	case model.CryptoNetworkEVM:
		return CryptoNetworkConfig{
			Enabled:       setting.CryptoEVMEnabled,
			NetworkType:   model.CryptoNetworkEVM,
			NetworkName:   setting.CryptoEVMNetworkName,
			ChainId:       strconv.FormatInt(setting.CryptoEVMChainID, 10),
			RPCURL:        setting.CryptoEVMRPCURL,
			WalletAddress: setting.CryptoEVMWalletAddress,
			TokenContract: setting.CryptoEVMTokenContract,
			TokenSymbol:   setting.CryptoEVMTokenSymbol,
			TokenDecimals: setting.CryptoEVMTokenDecimals,
			TokenPerUSD:   setting.CryptoEVMTokenPerUSD,
			Confirmations: setting.CryptoEVMConfirmations,
		}, nil
	case model.CryptoNetworkTron:
		return CryptoNetworkConfig{
			Enabled:             setting.CryptoTronEnabled,
			NetworkType:         model.CryptoNetworkTron,
			NetworkName:         setting.CryptoTronNetworkName,
			ChainId:             model.CryptoNetworkTron,
			RPCURL:              setting.CryptoTronAPIURL,
			APIKey:              setting.CryptoTronAPIKey,
			WalletAddress:       setting.CryptoTronWalletAddress,
			TokenContract:       setting.CryptoTronTokenContract,
			TokenSymbol:         setting.CryptoTronTokenSymbol,
			TokenDecimals:       setting.CryptoTronTokenDecimals,
			TokenPerUSD:         setting.CryptoTronTokenPerUSD,
			ConfirmationSeconds: setting.CryptoTronConfirmationSeconds,
		}, nil
	default:
		return CryptoNetworkConfig{}, errors.New("unsupported crypto network")
	}
}

func ValidateCryptoNetworkConfig(config CryptoNetworkConfig, uniqueDigits int) error {
	if config.NetworkType != model.CryptoNetworkEVM && config.NetworkType != model.CryptoNetworkTron {
		return errors.New("unsupported crypto network")
	}
	if strings.TrimSpace(config.NetworkName) == "" {
		return errors.New("network name is required")
	}
	if config.NetworkType == model.CryptoNetworkEVM {
		if _, err := NormalizeEVMRPCURLs(config.RPCURL); err != nil {
			return err
		}
	} else {
		parsedURL, err := url.Parse(strings.TrimSpace(config.RPCURL))
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			return errors.New("a valid HTTP or HTTPS RPC/API URL is required")
		}
	}
	if config.TokenDecimals < 1 || config.TokenDecimals > 36 {
		return errors.New("token decimals must be between 1 and 36")
	}
	if uniqueDigits < 1 || uniqueDigits > 3 || uniqueDigits > config.TokenDecimals {
		return errors.New("payment amount decimals must be between 1 and 3 and not exceed token decimals")
	}
	if strings.TrimSpace(config.TokenSymbol) == "" || len(config.TokenSymbol) > 20 {
		return errors.New("token symbol is required")
	}
	tokenPerUSD, err := decimal.NewFromString(strings.TrimSpace(config.TokenPerUSD))
	if err != nil || !tokenPerUSD.IsPositive() {
		return errors.New("token per USD must be a positive decimal")
	}

	if config.NetworkType == model.CryptoNetworkEVM {
		chainId, err := strconv.ParseInt(config.ChainId, 10, 64)
		if err != nil || chainId <= 0 {
			return errors.New("EVM chain ID must be positive")
		}
		if !evmAddressPattern.MatchString(config.WalletAddress) {
			return errors.New("invalid EVM wallet address")
		}
		if !evmAddressPattern.MatchString(config.TokenContract) {
			return errors.New("invalid ERC-20 token contract")
		}
		if config.Confirmations < 1 || config.Confirmations > 10000 {
			return errors.New("EVM confirmations must be between 1 and 10000")
		}
		return nil
	}

	if !tronAddressPattern.MatchString(config.WalletAddress) {
		return errors.New("invalid TRON wallet address")
	}
	if !tronAddressPattern.MatchString(config.TokenContract) {
		return errors.New("invalid TRC-20 token contract")
	}
	if config.ConfirmationSeconds < 0 || config.ConfirmationSeconds > 3600 {
		return errors.New("TRON confirmation delay must be between 0 and 3600 seconds")
	}
	return nil
}

// NormalizeEVMRPCURLs validates and normalizes a primary RPC plus optional
// fallback RPCs. One endpoint per line is preferred; commas are accepted for
// compatibility with environment-style configuration.
func NormalizeEVMRPCURLs(raw string) (string, error) {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ','
	})
	if len(parts) == 0 {
		return "", errors.New("at least one valid HTTP or HTTPS EVM RPC URL is required")
	}
	if len(parts) > maxEVMRPCEndpoints {
		return "", fmt.Errorf("at most %d EVM RPC URLs are allowed", maxEVMRPCEndpoints)
	}

	seen := make(map[string]struct{}, len(parts))
	endpoints := make([]string, 0, len(parts))
	for _, part := range parts {
		endpoint := strings.TrimRight(strings.TrimSpace(part), "/")
		parsedURL, err := url.Parse(endpoint)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			return "", errors.New("all EVM RPC URLs must be valid HTTP or HTTPS URLs")
		}
		if _, exists := seen[endpoint]; exists {
			continue
		}
		seen[endpoint] = struct{}{}
		endpoints = append(endpoints, endpoint)
	}
	return strings.Join(endpoints, "\n"), nil
}

func evmRPCEndpoints(config CryptoNetworkConfig) ([]string, error) {
	normalized, err := NormalizeEVMRPCURLs(config.RPCURL)
	if err != nil {
		return nil, err
	}
	return strings.Split(normalized, "\n"), nil
}

func IsCryptoPaymentAvailable(networkType string) bool {
	setting := operation_setting.GetPaymentSetting()
	if !setting.CryptoPaymentEnabled || !operation_setting.IsPaymentComplianceConfirmed() {
		return false
	}
	config, err := GetCryptoNetworkConfig(networkType)
	if err != nil || !config.Enabled {
		return false
	}
	return ValidateCryptoNetworkConfig(config, operation_setting.GetCryptoAmountDecimals()) == nil
}

func CryptoTokenAmountFromUSD(usdAmount decimal.Decimal, config CryptoNetworkConfig) (string, string, error) {
	if !usdAmount.IsPositive() {
		return "", "", errors.New("USD amount must be positive")
	}
	tokenPerUSD, err := decimal.NewFromString(strings.TrimSpace(config.TokenPerUSD))
	if err != nil || !tokenPerUSD.IsPositive() {
		return "", "", errors.New("invalid token per USD")
	}
	tokenAmount := usdAmount.Mul(tokenPerUSD)
	atomicAmount := tokenAmount.Shift(int32(config.TokenDecimals)).Round(0)
	if !atomicAmount.IsPositive() {
		return "", "", errors.New("crypto payment amount is too small")
	}
	display := atomicAmount.Shift(int32(-config.TokenDecimals)).StringFixed(int32(config.TokenDecimals))
	display = strings.TrimRight(strings.TrimRight(display, "0"), ".")
	return atomicAmount.StringFixed(0), display, nil
}

type evmRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type evmRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *evmRPCError    `json:"error"`
}

func cryptoHTTPClient() *http.Client {
	if client := GetHttpClient(); client != nil {
		return client
	}
	return http.DefaultClient
}

func doJSONRequest(ctx context.Context, method, endpoint string, headers map[string]string, requestBody interface{}, responseBody interface{}) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := common.Marshal(requestBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		if strings.TrimSpace(value) != "" {
			req.Header.Set(key, value)
		}
	}

	resp, err := cryptoHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCryptoResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > maxCryptoResponseBytes {
		return errors.New("crypto API response is too large")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("crypto API returned HTTP %d", resp.StatusCode)
	}
	if responseBody == nil {
		return nil
	}
	return common.Unmarshal(data, responseBody)
}

func callEVMRPC(ctx context.Context, rpcURL, method string, params interface{}, result interface{}) error {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	response := evmRPCResponse{}
	if err := doJSONRequest(ctx, http.MethodPost, rpcURL, nil, payload, &response); err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("EVM RPC error %d: %s", response.Error.Code, response.Error.Message)
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return errors.New("EVM RPC returned an empty result")
	}
	return common.Unmarshal(response.Result, result)
}

func parseHexInt64(value string) (int64, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if value == "" {
		return 0, errors.New("empty hex quantity")
	}
	parsed, ok := new(big.Int).SetString(value, 16)
	if !ok || !parsed.IsInt64() {
		return 0, errors.New("invalid hex quantity")
	}
	return parsed.Int64(), nil
}

func getEVMStartBlockFromEndpoint(ctx context.Context, config CryptoNetworkConfig, rpcURL string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, cryptoPaymentRequestTimeout)
	defer cancel()

	var chainIdHex string
	if err := callEVMRPC(ctx, rpcURL, "eth_chainId", []interface{}{}, &chainIdHex); err != nil {
		return 0, err
	}
	actualChainId, err := parseHexInt64(chainIdHex)
	if err != nil {
		return 0, err
	}
	expectedChainId, _ := strconv.ParseInt(config.ChainId, 10, 64)
	if actualChainId != expectedChainId {
		return 0, fmt.Errorf("EVM RPC chain ID mismatch: expected %d, got %d", expectedChainId, actualChainId)
	}

	var blockHex string
	if err := callEVMRPC(ctx, rpcURL, "eth_blockNumber", []interface{}{}, &blockHex); err != nil {
		return 0, err
	}
	block, err := parseHexInt64(blockHex)
	if err != nil {
		return 0, err
	}
	return block + 1, nil
}

func GetEVMStartBlock(ctx context.Context, config CryptoNetworkConfig) (int64, error) {
	endpoints, err := evmRPCEndpoints(config)
	if err != nil {
		return 0, err
	}
	errorsByEndpoint := make([]error, 0, len(endpoints))
	for index, endpoint := range endpoints {
		block, endpointErr := getEVMStartBlockFromEndpoint(ctx, config, endpoint)
		if endpointErr == nil {
			return block, nil
		}
		errorsByEndpoint = append(errorsByEndpoint, fmt.Errorf("RPC %d: %w", index+1, endpointErr))
	}
	return 0, fmt.Errorf("all EVM RPC endpoints failed: %w", errors.Join(errorsByEndpoint...))
}

type evmLog struct {
	Address         string   `json:"address"`
	Topics          []string `json:"topics"`
	Data            string   `json:"data"`
	BlockNumber     string   `json:"blockNumber"`
	TransactionHash string   `json:"transactionHash"`
	LogIndex        string   `json:"logIndex"`
	Removed         bool     `json:"removed"`
}

type evmReceipt struct {
	Status          string `json:"status"`
	BlockNumber     string `json:"blockNumber"`
	TransactionHash string `json:"transactionHash"`
}

type evmBlock struct {
	Timestamp string `json:"timestamp"`
}

func evmRecipientTopic(address string) string {
	return "0x" + strings.Repeat("0", 24) + strings.ToLower(strings.TrimPrefix(address, "0x"))
}

func normalizeEVMAtomicValue(data string) (string, error) {
	value := strings.TrimPrefix(strings.TrimSpace(data), "0x")
	parsed, ok := new(big.Int).SetString(value, 16)
	if !ok {
		return "", errors.New("invalid EVM log value")
	}
	return parsed.String(), nil
}

type evmVerifiedTransfer struct {
	payment         *model.CryptoPayment
	transactionHash string
	eventIndex      string
	blockNumber     int64
}

type evmGroupScanPlan struct {
	payments  []*model.CryptoPayment
	nextBlock int64
	transfers []evmVerifiedTransfer
}

func safeEVMRPCError(err error) error {
	if err == nil {
		return nil
	}
	message := evmRPCURLInErrorPattern.ReplaceAllString(err.Error(), "[redacted URL]")
	const maxMessageLength = 512
	if len(message) > maxMessageLength {
		message = message[:maxMessageLength] + "..."
	}
	return errors.New(message)
}

func evmRPCStageError(stage string, err error) error {
	return fmt.Errorf("%s: %w", stage, safeEVMRPCError(err))
}

func shortEVMReference(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 18 {
		return value
	}
	return value[:10] + "..." + value[len(value)-6:]
}

func reportEVMScannerResult(err error) {
	now := evmScannerLogNow()
	evmScannerHealth.Lock()
	if err == nil {
		wasUnhealthy := evmScannerHealth.unhealthy
		evmScannerHealth.unhealthy = false
		evmScannerHealth.Unlock()
		if wasUnhealthy {
			common.SysLog("EVM crypto payment scanner recovered")
		}
		return
	}

	shouldLog := !evmScannerHealth.unhealthy || now.Sub(evmScannerHealth.lastFailureLog) >= evmScannerLogInterval
	evmScannerHealth.unhealthy = true
	if shouldLog {
		evmScannerHealth.lastFailureLog = now
	}
	evmScannerHealth.Unlock()
	if shouldLog {
		common.SysError("EVM crypto payment scan failed: " + safeEVMRPCError(err).Error())
	}
}

func reportEVMScannerFallback(endpointIndex int, endpointErrors []error) {
	if endpointIndex <= 0 || len(endpointErrors) == 0 {
		return
	}
	now := evmScannerLogNow()
	evmScannerHealth.Lock()
	shouldLog := now.Sub(evmScannerHealth.lastFallback) >= evmScannerLogInterval
	if shouldLog {
		evmScannerHealth.lastFallback = now
	}
	evmScannerHealth.Unlock()
	if !shouldLog {
		return
	}

	safeErrors := make([]error, 0, len(endpointErrors))
	for _, err := range endpointErrors {
		safeErrors = append(safeErrors, safeEVMRPCError(err))
	}
	common.SysLog(fmt.Sprintf(
		"EVM crypto payment scanner used fallback RPC %d after %d endpoint failure(s): %s",
		endpointIndex+1,
		len(endpointErrors),
		errors.Join(safeErrors...).Error(),
	))
}

func scanEVMPaymentsWithEndpoint(config CryptoNetworkConfig, rpcURL string, payments []*model.CryptoPayment) (int64, error) {
	if len(payments) == 0 || strings.TrimSpace(config.RPCURL) == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), cryptoPaymentRequestTimeout)
	defer cancel()

	var chainIdHex string
	if err := callEVMRPC(ctx, rpcURL, "eth_chainId", []interface{}{}, &chainIdHex); err != nil {
		return 0, evmRPCStageError("eth_chainId failed", err)
	}
	actualChainId, err := parseHexInt64(chainIdHex)
	if err != nil || strconv.FormatInt(actualChainId, 10) != config.ChainId {
		return 0, errors.New("EVM scanner chain ID mismatch")
	}

	var headHex string
	if err := callEVMRPC(ctx, rpcURL, "eth_blockNumber", []interface{}{}, &headHex); err != nil {
		return 0, evmRPCStageError("eth_blockNumber failed", err)
	}
	head, err := parseHexInt64(headHex)
	if err != nil {
		return 0, err
	}
	confirmedHead := head - config.Confirmations
	if confirmedHead <= 0 {
		return 0, nil
	}

	groups := make(map[string][]*model.CryptoPayment)
	for _, payment := range payments {
		if payment.NetworkType != model.CryptoNetworkEVM || payment.ChainId != config.ChainId {
			continue
		}
		key := strings.ToLower(payment.WalletAddress + ":" + payment.TokenContract)
		groups[key] = append(groups[key], payment)
	}

	plans := make([]evmGroupScanPlan, 0, len(groups))
	for _, group := range groups {
		fromBlock := int64(0)
		for _, payment := range group {
			paymentFromBlock := payment.ScanFromBlock - evmScanOverlapBlocks
			if paymentFromBlock < payment.StartBlock {
				paymentFromBlock = payment.StartBlock
			}
			if fromBlock == 0 || paymentFromBlock < fromBlock {
				fromBlock = paymentFromBlock
			}
		}
		if fromBlock <= 0 || fromBlock > confirmedHead {
			continue
		}
		toBlock := confirmedHead
		if toBlock-fromBlock+1 > maxEVMBlockRange {
			toBlock = fromBlock + maxEVMBlockRange - 1
		}

		paymentByAmount := make(map[string]*model.CryptoPayment, len(group))
		for _, payment := range group {
			paymentByAmount[payment.RequestedAmount] = payment
		}
		filter := map[string]interface{}{
			"fromBlock": fmt.Sprintf("0x%x", fromBlock),
			"toBlock":   fmt.Sprintf("0x%x", toBlock),
			"address":   group[0].TokenContract,
			"topics":    []interface{}{erc20TransferTopic, nil, evmRecipientTopic(group[0].WalletAddress)},
		}
		var logs []evmLog
		if err := callEVMRPC(ctx, rpcURL, "eth_getLogs", []interface{}{filter}, &logs); err != nil {
			return 0, evmRPCStageError(
				fmt.Sprintf("eth_getLogs failed for blocks %d-%d", fromBlock, toBlock),
				err,
			)
		}

		plan := evmGroupScanPlan{
			payments:  group,
			nextBlock: toBlock + 1,
			transfers: make([]evmVerifiedTransfer, 0),
		}
		verifiedPayments := make(map[int]struct{})
		blockTimestamps := make(map[int64]int64)
		for _, logEntry := range logs {
			if logEntry.Removed || !strings.EqualFold(logEntry.Address, group[0].TokenContract) || len(logEntry.Topics) < 3 {
				continue
			}
			atomicValue, err := normalizeEVMAtomicValue(logEntry.Data)
			if err != nil {
				continue
			}
			payment := paymentByAmount[atomicValue]
			if payment == nil {
				continue
			}
			if _, alreadyVerified := verifiedPayments[payment.Id]; alreadyVerified {
				continue
			}
			blockNumber, err := parseHexInt64(logEntry.BlockNumber)
			if err != nil || blockNumber < payment.StartBlock {
				continue
			}
			blockTimestamp, exists := blockTimestamps[blockNumber]
			if !exists {
				var block evmBlock
				if err := callEVMRPC(ctx, rpcURL, "eth_getBlockByNumber", []interface{}{logEntry.BlockNumber, false}, &block); err != nil {
					return 0, evmRPCStageError(
						fmt.Sprintf("eth_getBlockByNumber failed for block %d", blockNumber),
						err,
					)
				}
				blockTimestamp, err = parseHexInt64(block.Timestamp)
				if err != nil {
					return 0, fmt.Errorf("invalid timestamp returned for EVM block %d: %w", blockNumber, err)
				}
				blockTimestamps[blockNumber] = blockTimestamp
			}
			if payment.ExpiresAt > 0 && blockTimestamp > payment.ExpiresAt {
				continue
			}

			var receipt evmReceipt
			if err := callEVMRPC(ctx, rpcURL, "eth_getTransactionReceipt", []interface{}{logEntry.TransactionHash}, &receipt); err != nil {
				return 0, evmRPCStageError(
					fmt.Sprintf("eth_getTransactionReceipt failed for transaction %s", shortEVMReference(logEntry.TransactionHash)),
					err,
				)
			}
			if receipt.Status != "0x1" {
				continue
			}
			receiptBlock, err := parseHexInt64(receipt.BlockNumber)
			if err != nil || receiptBlock != blockNumber {
				return 0, fmt.Errorf(
					"invalid EVM receipt block for transaction %s: log block %d, receipt block %q",
					shortEVMReference(logEntry.TransactionHash),
					blockNumber,
					receipt.BlockNumber,
				)
			}
			if receipt.TransactionHash != "" && !strings.EqualFold(receipt.TransactionHash, logEntry.TransactionHash) {
				return 0, fmt.Errorf("mismatched EVM receipt transaction hash for %s", shortEVMReference(logEntry.TransactionHash))
			}
			verifiedPayments[payment.Id] = struct{}{}
			plan.transfers = append(plan.transfers, evmVerifiedTransfer{
				payment:         payment,
				transactionHash: logEntry.TransactionHash,
				eventIndex:      logEntry.LogIndex,
				blockNumber:     blockNumber,
			})
		}
		plans = append(plans, plan)
	}

	// Do not mutate payment state until every RPC read for this endpoint has
	// succeeded. This lets the caller safely retry the complete scan against a
	// fallback endpoint without partially crediting orders or advancing cursors.
	for _, plan := range plans {
		retryPayment := make(map[int]struct{})
		for _, transfer := range plan.transfers {
			completed, err := model.CompleteCryptoPaymentOnce(
				transfer.payment.TradeNo,
				transfer.transactionHash,
				transfer.eventIndex,
				transfer.blockNumber,
			)
			if err != nil {
				retryPayment[transfer.payment.Id] = struct{}{}
				common.SysError("failed to complete EVM crypto payment: " + err.Error())
			} else if completed {
				NotifyTopUpSuccessAsync(TopUpSuccessEmailParams{
					TradeNo:    transfer.payment.TradeNo,
					PaidAmount: transfer.payment.DisplayAmount,
					Currency:   transfer.payment.TokenSymbol,
				})
			}
		}

		for _, payment := range plan.payments {
			if _, shouldRetry := retryPayment[payment.Id]; shouldRetry {
				continue
			}
			if err := model.UpdateCryptoPaymentScanBlock(payment.Id, plan.nextBlock); err != nil {
				common.SysError("failed to update EVM crypto scan cursor: " + err.Error())
			}
		}
	}
	return confirmedHead, nil
}

func scanEVMPayments(config CryptoNetworkConfig, payments []*model.CryptoPayment) (int64, error) {
	endpoints, err := evmRPCEndpoints(config)
	if err != nil {
		return 0, err
	}
	errorsByEndpoint := make([]error, 0, len(endpoints))
	for index, endpoint := range endpoints {
		confirmedHead, endpointErr := scanEVMPaymentsWithEndpoint(config, endpoint, payments)
		if endpointErr == nil {
			reportEVMScannerFallback(index, errorsByEndpoint)
			return confirmedHead, nil
		}
		errorsByEndpoint = append(errorsByEndpoint, fmt.Errorf("RPC %d: %w", index+1, safeEVMRPCError(endpointErr)))
	}
	return 0, fmt.Errorf("all EVM RPC endpoints failed: %w", errors.Join(errorsByEndpoint...))
}

type tronTokenInfo struct {
	Address  string `json:"address"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

type tronTransfer struct {
	TransactionId  string        `json:"transaction_id"`
	TokenInfo      tronTokenInfo `json:"token_info"`
	BlockTimestamp int64         `json:"block_timestamp"`
	From           string        `json:"from"`
	To             string        `json:"to"`
	Type           string        `json:"type"`
	Value          string        `json:"value"`
}

type tronTransfersResponse struct {
	Data    []tronTransfer `json:"data"`
	Success bool           `json:"success"`
	Meta    struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

type tronTransactionInfo struct {
	Id          string `json:"id"`
	BlockNumber int64  `json:"blockNumber"`
	Receipt     struct {
		Result string `json:"result"`
	} `json:"receipt"`
}

func tronHeaders(config CryptoNetworkConfig) map[string]string {
	return map[string]string{"TRON-PRO-API-KEY": config.APIKey}
}

func getTronTransactionInfo(ctx context.Context, config CryptoNetworkConfig, transactionId string) (*tronTransactionInfo, error) {
	endpoint := strings.TrimRight(config.RPCURL, "/") + "/walletsolidity/gettransactioninfobyid"
	info := &tronTransactionInfo{}
	if err := doJSONRequest(ctx, http.MethodPost, endpoint, tronHeaders(config), map[string]string{"value": transactionId}, info); err != nil {
		return nil, err
	}
	if !strings.EqualFold(info.Id, transactionId) || info.Receipt.Result != "SUCCESS" {
		return nil, errors.New("TRON transaction is not successful")
	}
	return info, nil
}

func scanTronPaymentGroup(ctx context.Context, config CryptoNetworkConfig, active []*model.CryptoPayment) error {
	minTimestamp := active[0].CreateTimeMillis
	paymentByAmount := make(map[string]*model.CryptoPayment, len(active))
	for _, payment := range active {
		paymentByAmount[payment.RequestedAmount] = payment
		if payment.CreateTimeMillis < minTimestamp {
			minTimestamp = payment.CreateTimeMillis
		}
	}
	ctx, cancel := context.WithTimeout(ctx, cryptoPaymentRequestTimeout)
	defer cancel()

	fingerprint := ""
	for page := 0; page < maxTronPagesPerScan; page++ {
		query := url.Values{}
		query.Set("only_confirmed", "true")
		query.Set("only_to", "true")
		query.Set("contract_address", active[0].TokenContract)
		query.Set("min_timestamp", strconv.FormatInt(minTimestamp, 10))
		query.Set("order_by", "block_timestamp,asc")
		query.Set("limit", "200")
		if fingerprint != "" {
			query.Set("fingerprint", fingerprint)
		}
		endpoint := strings.TrimRight(config.RPCURL, "/") + "/v1/accounts/" + url.PathEscape(active[0].WalletAddress) + "/transactions/trc20?" + query.Encode()
		response := tronTransfersResponse{}
		if err := doJSONRequest(ctx, http.MethodGet, endpoint, tronHeaders(config), nil, &response); err != nil {
			return err
		}
		if !response.Success {
			return errors.New("TRON API returned an unsuccessful response")
		}

		for _, transfer := range response.Data {
			if transfer.Type != "Transfer" || transfer.To != active[0].WalletAddress || transfer.TokenInfo.Address != active[0].TokenContract {
				continue
			}
			payment := paymentByAmount[transfer.Value]
			if payment == nil || transfer.BlockTimestamp < payment.CreateTimeMillis {
				continue
			}
			if payment.ExpiresAt > 0 && transfer.BlockTimestamp > payment.ExpiresAt*1000 {
				continue
			}
			if time.Now().UnixMilli()-transfer.BlockTimestamp < config.ConfirmationSeconds*1000 {
				continue
			}
			info, err := getTronTransactionInfo(ctx, config, transfer.TransactionId)
			if err != nil {
				continue
			}
			completed, err := model.CompleteCryptoPaymentOnce(payment.TradeNo, transfer.TransactionId, "", info.BlockNumber)
			if err != nil {
				common.SysError("failed to complete TRON crypto payment: " + err.Error())
			} else if completed {
				NotifyTopUpSuccessAsync(TopUpSuccessEmailParams{
					TradeNo:    payment.TradeNo,
					PaidAmount: payment.DisplayAmount,
					Currency:   payment.TokenSymbol,
				})
			}
		}

		fingerprint = response.Meta.Fingerprint
		if fingerprint == "" || len(response.Data) == 0 {
			break
		}
	}
	return nil
}

func scanTronPayments(config CryptoNetworkConfig, payments []*model.CryptoPayment) error {
	if strings.TrimSpace(config.RPCURL) == "" {
		return nil
	}
	groups := make(map[string][]*model.CryptoPayment)
	for _, payment := range payments {
		if payment.NetworkType != model.CryptoNetworkTron {
			continue
		}
		key := payment.WalletAddress + ":" + payment.TokenContract
		groups[key] = append(groups[key], payment)
	}
	for _, group := range groups {
		if err := scanTronPaymentGroup(context.Background(), config, group); err != nil {
			return err
		}
	}
	return nil
}

func scanPendingCryptoPayments() {
	payments, err := model.ListPendingCryptoPayments(500)
	if err != nil {
		common.SysError("failed to list pending crypto payments: " + err.Error())
		return
	}
	if len(payments) == 0 {
		return
	}
	hasEVMPayments := false
	hasTronPayments := false
	for _, payment := range payments {
		switch payment.NetworkType {
		case model.CryptoNetworkEVM:
			hasEVMPayments = true
		case model.CryptoNetworkTron:
			hasTronPayments = true
		}
	}

	evmScannedThrough := int64(0)
	if hasEVMPayments {
		evmConfig, evmErr := GetCryptoNetworkConfig(model.CryptoNetworkEVM)
		if evmErr == nil {
			if validateErr := ValidateCryptoNetworkConfig(evmConfig, operation_setting.GetCryptoAmountDecimals()); validateErr == nil {
				if evmScannedThrough, err = scanEVMPayments(evmConfig, payments); err != nil {
					reportEVMScannerResult(err)
					evmScannedThrough = 0
				} else {
					reportEVMScannerResult(nil)
				}
			} else {
				common.SysError("EVM crypto payment scan skipped due to invalid configuration: " + validateErr.Error())
			}
		} else {
			common.SysError("EVM crypto payment scan skipped: " + evmErr.Error())
		}
	}

	tronScanComplete := false
	if hasTronPayments {
		tronConfig, tronErr := GetCryptoNetworkConfig(model.CryptoNetworkTron)
		if tronErr == nil {
			if validateErr := ValidateCryptoNetworkConfig(tronConfig, operation_setting.GetCryptoAmountDecimals()); validateErr == nil {
				if err := scanTronPayments(tronConfig, payments); err != nil {
					common.SysError("TRON crypto payment scan failed: " + err.Error())
				} else {
					tronScanComplete = true
				}
			} else {
				common.SysError("TRON crypto payment scan skipped due to invalid configuration: " + validateErr.Error())
			}
		} else {
			common.SysError("TRON crypto payment scan skipped: " + tronErr.Error())
		}
	}

	if err := model.ExpireCryptoPayments(common.GetTimestamp(), evmScannedThrough, tronScanComplete); err != nil {
		common.SysError("failed to expire crypto payments: " + err.Error())
	}
}

func StartCryptoPaymentTask() {
	if !common.IsMasterNode {
		return
	}
	startCryptoPaymentTaskOnce.Do(func() {
		gopool.Go(func() {
			ticker := time.NewTicker(cryptoPaymentPollInterval)
			defer ticker.Stop()
			scanPendingCryptoPayments()
			for range ticker.C {
				scanPendingCryptoPayments()
			}
		})
	})
}
