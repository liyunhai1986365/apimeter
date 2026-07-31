package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

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
