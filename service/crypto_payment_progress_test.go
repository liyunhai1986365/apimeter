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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestEVMProgressWaitsForConfirmationsAcrossExpiry(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(fmt.Sprintf("removed_%v", removed), func(t *testing.T) {
			setupCryptoPaymentServiceTestDB(t)
			require.NoError(t, model.DB.Create(&model.User{Id: 1, Username: "progress-user"}).Error)
			now := time.Now()
			payment := createEVMPaymentServiceTestOrder(t, "progress", now.Add(-time.Minute))
			require.NoError(t, model.DB.Model(payment).Update("expires_at", now.Unix()-1).Error)
			head := int64(102)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct{ Method string }
				require.NoError(t, common.DecodeJson(r.Body, &request))
				var result interface{}
				switch request.Method {
				case "eth_chainId":
					result = "0x2105"
				case "eth_blockNumber":
					result = fmt.Sprintf("0x%x", head)
				case "eth_getLogs":
					result = []map[string]interface{}{{
						"address":     payment.TokenContract,
						"topics":      []string{erc20TransferTopic, "0x0", evmRecipientTopic(payment.WalletAddress)},
						"data":        fmt.Sprintf("0x%064x", decimal.RequireFromString(payment.RequestedAmount).BigInt()),
						"blockNumber": "0x65", "transactionHash": "0xabc", "logIndex": "0x0",
						"removed": removed && head > 102,
					}}
				case "eth_getBlockByNumber":
					result = map[string]string{"timestamp": fmt.Sprintf("0x%x", now.Unix()-2)}
				case "eth_getTransactionReceipt":
					result = map[string]string{"status": "0x1", "blockNumber": "0x65", "transactionHash": "0xabc"}
				default:
					t.Errorf("unexpected method %s", request.Method)
				}
				body, err := common.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "result": result})
				require.NoError(t, err)
				_, _ = w.Write(body)
			}))
			defer server.Close()
			config := evmPaymentServiceTestConfig(payment, server.URL)
			config.Confirmations = 2
			confirmedHead, err := scanEVMPayments(config, []*model.CryptoPayment{payment})
			require.NoError(t, err)
			require.Equal(t, int64(100), confirmedHead)
			stored, err := model.GetCryptoPaymentForUser(payment.TradeNo, 1)
			require.NoError(t, err)
			require.Equal(t, "confirming", stored.ReadProgress().Stage)
			require.Equal(t, int64(1), stored.ReadProgress().Confirmations)
			require.Equal(t, int64(2), stored.ReadProgress().RequiredConfirmations)
			require.Equal(t, int64(101), stored.ScanFromBlock)
			require.Empty(t, stored.TransactionHash, "observations must not become settlement records")
			var user model.User
			require.NoError(t, model.DB.First(&user, 1).Error)
			require.Zero(t, user.Quota)
			require.NoError(t, model.ExpireCryptoPayments(now.Unix(), confirmedHead, false))
			require.NoError(t, model.DB.First(stored, stored.Id).Error)
			require.Equal(t, model.CryptoPaymentStatusPending, stored.Status)

			head = 103
			confirmedHead, err = scanEVMPayments(config, []*model.CryptoPayment{stored})
			require.NoError(t, err)
			require.NoError(t, model.ExpireCryptoPayments(now.Unix(), confirmedHead, false))
			require.NoError(t, model.DB.First(stored, stored.Id).Error)
			require.NoError(t, model.DB.First(&user, 1).Error)
			if removed {
				require.Equal(t, model.CryptoPaymentStatusExpired, stored.Status)
				require.Empty(t, stored.ReadProgress().TransactionHash)
				require.Zero(t, user.Quota)
			} else {
				require.Equal(t, model.CryptoPaymentStatusSuccess, stored.Status)
				require.Equal(t, "completed", CryptoPaymentOrderWithProgress(stored).Progress.Stage)
				require.Equal(t, int64(10*common.QuotaPerUnit), int64(user.Quota))
				waitForTopUpSuccessEmail(t, stored.TradeNo)
			}
		})
	}
}

func TestEVMProgressKeepsLastObservationOnRPCFailure(t *testing.T) {
	setupCryptoPaymentServiceTestDB(t)
	payment := createEVMPaymentServiceTestOrder(t, "retry-progress", time.Now())
	checkedAt := time.Now().Unix() - 20
	require.NoError(t, model.UpdateCryptoPaymentProgress(payment, model.CryptoPaymentProgress{
		Stage: "confirming", CheckedAt: checkedAt, TransactionHash: "0xabc", Confirmations: 1,
	}))
	server := newEVMPaymentRPCServer(t, payment, time.Now(), "eth_getTransactionReceipt", "unavailable", nil)
	defer server.Close()
	_, err := scanEVMPayments(evmPaymentServiceTestConfig(payment, server.URL), []*model.CryptoPayment{payment})
	require.Error(t, err)
	stored, err := model.GetCryptoPaymentForUser(payment.TradeNo, payment.UserId)
	require.NoError(t, err)
	progress := stored.ReadProgress()
	require.True(t, progress.Retrying)
	require.Equal(t, checkedAt, progress.CheckedAt)
	require.Equal(t, int64(1), progress.Confirmations)
	require.Equal(t, "0xabc", progress.TransactionHash)
	require.Equal(t, payment.StartBlock, stored.ScanFromBlock)
	_, err = model.GetCryptoPaymentForUser(payment.TradeNo, payment.UserId+1)
	require.Error(t, err, "progress must remain scoped to the order owner")
}

func TestCryptoProgressResponseUsesAuthoritativeStatusAndHidesSnapshot(t *testing.T) {
	payment := &model.CryptoPayment{Status: model.CryptoPaymentStatusPending, CreateTime: time.Now().Unix() - 120}
	response := CryptoPaymentOrderWithProgress(payment)
	require.Equal(t, "checking", response.Progress.Stage)
	require.True(t, response.Progress.Stale)
	payment.ProgressSnapshot = `{"stage":"confirming","retrying":true,"checked_at":1}`
	for _, status := range []string{model.CryptoPaymentStatusSuccess, model.CryptoPaymentStatusManual, model.CryptoPaymentStatusExpired} {
		payment.Status = status
		response = CryptoPaymentOrderWithProgress(payment)
		require.False(t, response.Progress.Stale)
		require.False(t, response.Progress.Retrying)
		expected := "completed"
		if status == model.CryptoPaymentStatusExpired {
			expected = "expired"
		}
		require.Equal(t, expected, response.Progress.Stage)
		body, err := common.Marshal(response)
		require.NoError(t, err)
		require.NotContains(t, string(body), "ProgressSnapshot")
		require.NotContains(t, string(body), "progress_snapshot")
	}
}

func TestTronProgressProtectsTransferWaitingAcrossExpiry(t *testing.T) {
	setupCryptoPaymentServiceTestDB(t)
	payment := createEVMPaymentServiceTestOrder(t, "tron-progress", time.Now().Add(-2*time.Minute))
	payment.NetworkType = model.CryptoNetworkTron
	payment.ExpiresAt = time.Now().Unix() - 1
	require.NoError(t, model.DB.Save(payment).Error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var result interface{}
		if strings.HasPrefix(r.URL.Path, "/v1/accounts/") {
			require.Equal(t, "true", r.URL.Query().Get("only_confirmed"))
			result = tronTransfersResponse{Success: true, Data: []tronTransfer{{
				TransactionId: "tron-hash", BlockTimestamp: time.Now().Add(-20 * time.Second).UnixMilli(),
				To: payment.WalletAddress, Type: "Transfer", Value: payment.RequestedAmount,
				TokenInfo: tronTokenInfo{Address: payment.TokenContract},
			}}}
		} else {
			require.Equal(t, "/walletsolidity/gettransactioninfobyid", r.URL.Path)
			result = map[string]interface{}{"id": "tron-hash", "blockNumber": 123, "receipt": map[string]string{"result": "SUCCESS"}}
		}
		body, err := common.Marshal(result)
		require.NoError(t, err)
		_, _ = w.Write(body)
	}))
	defer server.Close()
	err := scanTronPaymentGroup(context.Background(), CryptoNetworkConfig{RPCURL: server.URL, ConfirmationSeconds: 60}, []*model.CryptoPayment{payment})
	require.NoError(t, err)
	stored, err := model.GetCryptoPaymentForUser(payment.TradeNo, payment.UserId)
	require.NoError(t, err)
	progress := stored.ReadProgress()
	require.Equal(t, "confirming", progress.Stage)
	require.Equal(t, int64(123), progress.BlockNumber)
	require.Equal(t, int64(60), progress.RequiredSeconds)
	require.GreaterOrEqual(t, progress.ConfirmedSeconds, int64(19))
	require.Less(t, progress.ConfirmedSeconds, int64(60))
	require.Zero(t, progress.RequiredConfirmations, "TRON must not invent an EVM confirmation target")
	require.NoError(t, model.ExpireCryptoPayments(time.Now().Unix(), 0, true))
	require.NoError(t, model.DB.First(stored, stored.Id).Error)
	require.Equal(t, model.CryptoPaymentStatusPending, stored.Status)
}
