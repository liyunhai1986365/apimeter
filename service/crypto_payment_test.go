package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCryptoPaymentServiceTestDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.CryptoPayment{}, &model.Log{}, &model.AffiliateTopUpReward{}))

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})
}

func TestCryptoTokenAmountFromUSD(t *testing.T) {
	config := CryptoNetworkConfig{TokenDecimals: 6, TokenPerUSD: "1.25"}
	atomic, display, err := CryptoTokenAmountFromUSD(decimal.RequireFromString("10.50"), config)
	require.NoError(t, err)
	require.Equal(t, "13125000", atomic)
	require.Equal(t, "13.125", display)
}

func TestValidateCryptoNetworkConfig(t *testing.T) {
	evm := CryptoNetworkConfig{
		NetworkType:   model.CryptoNetworkEVM,
		NetworkName:   "Base",
		ChainId:       "8453",
		RPCURL:        "https://rpc.example.com",
		WalletAddress: "0x1111111111111111111111111111111111111111",
		TokenContract: "0x2222222222222222222222222222222222222222",
		TokenSymbol:   "USDT",
		TokenDecimals: 6,
		TokenPerUSD:   "1",
		Confirmations: 12,
	}
	require.NoError(t, ValidateCryptoNetworkConfig(evm, 3))
	require.Error(t, ValidateCryptoNetworkConfig(evm, 4))
	evm.WalletAddress = "not-an-address"
	require.Error(t, ValidateCryptoNetworkConfig(evm, 3))

	tron := CryptoNetworkConfig{
		NetworkType:         model.CryptoNetworkTron,
		NetworkName:         "TRON",
		RPCURL:              "https://api.trongrid.io",
		WalletAddress:       "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		TokenContract:       "TXLAQ63Xg1NAzckPwKHvzw7CSEmLMEqcdj",
		TokenSymbol:         "USDT",
		TokenDecimals:       6,
		TokenPerUSD:         "1",
		ConfirmationSeconds: 60,
	}
	require.NoError(t, ValidateCryptoNetworkConfig(tron, 3))
}

func TestNormalizeEVMRPCURLs(t *testing.T) {
	normalized, err := NormalizeEVMRPCURLs(" https://primary.example.com/\nhttps://backup.example.com,https://primary.example.com ")
	require.NoError(t, err)
	require.Equal(t, "https://primary.example.com\nhttps://backup.example.com", normalized)

	_, err = NormalizeEVMRPCURLs("https://one.example.com\nnot-a-url")
	require.Error(t, err)
}

func TestGetEVMStartBlockVerifiesChain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Method string `json:"method"`
		}
		require.NoError(t, common.DecodeJson(r.Body, &request))
		w.Header().Set("Content-Type", "application/json")
		if request.Method == "eth_chainId" {
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x2105"}`))
			return
		}
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"0x64"}`))
	}))
	defer server.Close()

	config := CryptoNetworkConfig{RPCURL: server.URL, ChainId: "8453"}
	block, err := GetEVMStartBlock(context.Background(), config)
	require.NoError(t, err)
	require.Equal(t, int64(101), block)

	config.ChainId = "1"
	_, err = GetEVMStartBlock(context.Background(), config)
	require.ErrorContains(t, err, "chain ID mismatch")
}

func TestScanEVMPaymentsFallsBackAndCreditsTransferAtNextBlock(t *testing.T) {
	setupCryptoPaymentServiceTestDB(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "crypto-user"}).Error)
	now := time.Now()
	payment, err := model.CreateCryptoPaymentOrder(model.CreateCryptoPaymentParams{
		TopUp: &model.TopUp{
			UserId:          1,
			Amount:          10,
			TradeNo:         "crypto-rpc-fallback",
			PaymentMethod:   model.PaymentMethodCryptoEVM,
			PaymentProvider: model.PaymentProviderCrypto,
			CreateTime:      now.Unix(),
			Status:          common.TopUpStatusPending,
		},
		NetworkType:      model.CryptoNetworkEVM,
		NetworkName:      "Base",
		ChainId:          "8453",
		WalletAddress:    "0x1111111111111111111111111111111111111111",
		TokenContract:    "0x2222222222222222222222222222222222222222",
		TokenSymbol:      "USDT",
		TokenDecimals:    6,
		BaseAtomicAmount: "10000000",
		UniqueDigits:     3,
		StartBlock:       100,
		CreateTimeMillis: now.UnixMilli(),
		ExpiresAt:        now.Add(30 * time.Minute).Unix(),
	})
	require.NoError(t, err)
	latePayment, err := model.CreateCryptoPaymentOrder(model.CreateCryptoPaymentParams{
		TopUp: &model.TopUp{
			UserId:          1,
			Amount:          10,
			TradeNo:         "crypto-late-transfer",
			PaymentMethod:   model.PaymentMethodCryptoEVM,
			PaymentProvider: model.PaymentProviderCrypto,
			CreateTime:      now.Add(-2 * time.Minute).Unix(),
			Status:          common.TopUpStatusPending,
		},
		NetworkType:      model.CryptoNetworkEVM,
		NetworkName:      "Base",
		ChainId:          "8453",
		WalletAddress:    payment.WalletAddress,
		TokenContract:    payment.TokenContract,
		TokenSymbol:      payment.TokenSymbol,
		TokenDecimals:    payment.TokenDecimals,
		BaseAtomicAmount: "10000000",
		UniqueDigits:     3,
		StartBlock:       100,
		CreateTimeMillis: now.Add(-2 * time.Minute).UnixMilli(),
		ExpiresAt:        now.Add(-time.Minute).Unix(),
	})
	require.NoError(t, err)

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryCalls++
		http.Error(w, "archive access unavailable", http.StatusForbidden)
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		var request struct {
			Method string `json:"method"`
		}
		require.NoError(t, common.DecodeJson(r.Body, &request))
		var result interface{}
		switch request.Method {
		case "eth_chainId":
			result = "0x2105"
		case "eth_blockNumber":
			result = "0x66"
		case "eth_getLogs":
			result = []map[string]interface{}{
				{
					"address":         payment.TokenContract,
					"topics":          []string{erc20TransferTopic, "0x" + strings.Repeat("0", 64), evmRecipientTopic(payment.WalletAddress)},
					"data":            fmt.Sprintf("0x%064x", decimal.RequireFromString(payment.RequestedAmount).BigInt()),
					"blockNumber":     "0x65",
					"transactionHash": "0xabc",
					"logIndex":        "0x1",
					"removed":         false,
				},
				{
					"address":         latePayment.TokenContract,
					"topics":          []string{erc20TransferTopic, "0x" + strings.Repeat("0", 64), evmRecipientTopic(latePayment.WalletAddress)},
					"data":            fmt.Sprintf("0x%064x", decimal.RequireFromString(latePayment.RequestedAmount).BigInt()),
					"blockNumber":     "0x65",
					"transactionHash": "0xlate",
					"logIndex":        "0x2",
					"removed":         false,
				},
			}
		case "eth_getBlockByNumber":
			result = map[string]string{"timestamp": fmt.Sprintf("0x%x", now.Unix())}
		case "eth_getTransactionReceipt":
			result = map[string]string{"status": "0x1", "blockNumber": "0x65"}
		default:
			t.Fatalf("unexpected RPC method %s", request.Method)
		}
		payload, marshalErr := common.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": result})
		require.NoError(t, marshalErr)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer secondary.Close()

	config := CryptoNetworkConfig{
		NetworkType:   model.CryptoNetworkEVM,
		NetworkName:   "Base",
		ChainId:       "8453",
		RPCURL:        primary.URL + "\n" + secondary.URL,
		WalletAddress: payment.WalletAddress,
		TokenContract: payment.TokenContract,
		TokenSymbol:   payment.TokenSymbol,
		TokenDecimals: payment.TokenDecimals,
		TokenPerUSD:   "1",
		Confirmations: 1,
	}
	confirmedHead, err := scanEVMPayments(config, []*model.CryptoPayment{payment, latePayment})
	require.NoError(t, err)
	require.Equal(t, int64(101), confirmedHead)
	require.Positive(t, primaryCalls)
	require.GreaterOrEqual(t, secondaryCalls, 5)

	require.NoError(t, model.DB.First(payment, payment.Id).Error)
	require.Equal(t, model.CryptoPaymentStatusSuccess, payment.Status)
	require.Equal(t, "0xabc", payment.TransactionHash)
	require.NoError(t, model.DB.First(latePayment, latePayment.Id).Error)
	require.Equal(t, model.CryptoPaymentStatusPending, latePayment.Status)
	require.Equal(t, int64(102), latePayment.ScanFromBlock)
	require.NoError(t, model.ExpireCryptoPayments(now.Unix(), confirmedHead, false))
	require.NoError(t, model.DB.First(latePayment, latePayment.Id).Error)
	require.Equal(t, model.CryptoPaymentStatusExpired, latePayment.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	require.Equal(t, int64(10*common.QuotaPerUnit), int64(user.Quota))
}
