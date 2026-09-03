package model

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCryptoPaymentTestDB(t *testing.T) {
	t.Helper()
	originalDB := DB
	originalLogDB := LOG_DB
	originalSQLite := common.UsingSQLite
	originalMySQL := common.UsingMySQL
	originalPostgreSQL := common.UsingPostgreSQL

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &CryptoPayment{}, &Log{}, &AffiliateTopUpReward{}))

	t.Cleanup(func() {
		DB = originalDB
		LOG_DB = originalLogDB
		common.UsingSQLite = originalSQLite
		common.UsingMySQL = originalMySQL
		common.UsingPostgreSQL = originalPostgreSQL
	})
}

func createCryptoPaymentTestOrder(t *testing.T, userId int, tradeNo string) *CryptoPayment {
	t.Helper()
	now := time.Now()
	payment, err := CreateCryptoPaymentOrder(CreateCryptoPaymentParams{
		TopUp: &TopUp{
			UserId:          userId,
			Amount:          10,
			TradeNo:         tradeNo,
			PaymentMethod:   PaymentMethodCryptoEVM,
			PaymentProvider: PaymentProviderCrypto,
			CreateTime:      now.Unix(),
			Status:          common.TopUpStatusPending,
		},
		NetworkType:      CryptoNetworkEVM,
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
	return payment
}

func TestCreateCryptoPaymentOrderReservesExactUniqueAmount(t *testing.T) {
	setupCryptoPaymentTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "crypto-user"}).Error)

	first := createCryptoPaymentTestOrder(t, 1, "crypto-order-1")
	second := createCryptoPaymentTestOrder(t, 1, "crypto-order-2")

	require.NotEqual(t, first.RequestedAmount, second.RequestedAmount)
	require.Equal(t, "10.001", first.DisplayAmount)
	require.Equal(t, "10.002", second.DisplayAmount)
	require.NotNil(t, first.ReservationKey)
	require.Len(t, *first.ReservationKey, 64)
}

func TestUniqueAmountCandidateUsesThreeDecimalsAndSmallestValuesFirst(t *testing.T) {
	first, err := uniqueAmountCandidate("10000000", 6, 3, 0)
	require.NoError(t, err)
	require.Equal(t, "10001000", first)

	lastWithLeadingZero, err := uniqueAmountCandidate("10000000", 6, 3, 98)
	require.NoError(t, err)
	require.Equal(t, "10099000", lastWithLeadingZero)

	firstWithoutLeadingZero, err := uniqueAmountCandidate("10000000", 6, 3, 99)
	require.NoError(t, err)
	require.Equal(t, "10100000", firstWithoutLeadingZero)

	roundedUp, err := uniqueAmountCandidate("10428571", 6, 3, 0)
	require.NoError(t, err)
	require.Equal(t, "10429000", roundedUp)
	display, err := formatAtomicAmount(roundedUp, 6, 3)
	require.NoError(t, err)
	require.Equal(t, "10.429", display)
}

func TestCompleteCryptoPaymentCreditsExactlyOnceAndRejectsReusedTransfer(t *testing.T) {
	setupCryptoPaymentTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "crypto-user"}).Error)
	first := createCryptoPaymentTestOrder(t, 1, "crypto-order-1")
	second := createCryptoPaymentTestOrder(t, 1, "crypto-order-2")

	completed, err := CompleteCryptoPaymentOnce(first.TradeNo, "0xabc", "0x1", 123)
	require.NoError(t, err)
	require.True(t, completed)
	completed, err = CompleteCryptoPaymentOnce(first.TradeNo, "0xabc", "0x1", 123)
	require.NoError(t, err)
	require.False(t, completed)

	var user User
	require.NoError(t, DB.First(&user, 1).Error)
	expectedQuota := int64(10 * common.QuotaPerUnit)
	require.Equal(t, expectedQuota, int64(user.Quota))

	err = CompleteCryptoPayment(second.TradeNo, "0xabc", "0x1", 123)
	require.Error(t, err)
	require.NoError(t, DB.First(&user, 1).Error)
	require.Equal(t, expectedQuota, int64(user.Quota))

	var secondPayment CryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", second.TradeNo).First(&secondPayment).Error)
	require.Equal(t, CryptoPaymentStatusPending, secondPayment.Status)
}

func TestExpireCryptoPaymentReleasesReservation(t *testing.T) {
	setupCryptoPaymentTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "crypto-user"}).Error)
	payment := createCryptoPaymentTestOrder(t, 1, "crypto-order-expire")
	require.NoError(t, DB.Model(&CryptoPayment{}).Where("id = ?", payment.Id).Update("expires_at", time.Now().Add(-time.Minute).Unix()).Error)
	require.NoError(t, UpdateCryptoPaymentScanBlock(payment.Id, 200))

	require.NoError(t, ExpireCryptoPayments(time.Now().Unix(), 199, false))

	require.NoError(t, DB.First(payment, payment.Id).Error)
	require.Equal(t, CryptoPaymentStatusExpired, payment.Status)
	require.Nil(t, payment.ReservationKey)
	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&topUp).Error)
	require.Equal(t, common.TopUpStatusFailed, topUp.Status)
}

func TestExpireCryptoPaymentWaitsForEVMScannerToCatchUp(t *testing.T) {
	setupCryptoPaymentTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "crypto-user"}).Error)
	payment := createCryptoPaymentTestOrder(t, 1, "crypto-order-reconciling")
	require.NoError(t, DB.Model(&CryptoPayment{}).Where("id = ?", payment.Id).Update("expires_at", time.Now().Add(-time.Minute).Unix()).Error)

	require.NoError(t, ExpireCryptoPayments(time.Now().Unix(), payment.ScanFromBlock+100, false))

	require.NoError(t, DB.First(payment, payment.Id).Error)
	require.Equal(t, CryptoPaymentStatusPending, payment.Status)
	require.NotNil(t, payment.ReservationKey)
	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&topUp).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestManualCompleteTopUpKeepsCryptoPaymentStateConsistent(t *testing.T) {
	setupCryptoPaymentTestDB(t)
	require.NoError(t, DB.Create(&User{Id: 1, Username: "crypto-user"}).Error)
	payment := createCryptoPaymentTestOrder(t, 1, "crypto-order-manual")

	require.NoError(t, ManualCompleteTopUp(payment.TradeNo, "127.0.0.1"))

	require.NoError(t, DB.First(payment, payment.Id).Error)
	require.Equal(t, CryptoPaymentStatusManual, payment.Status)
	require.NotZero(t, payment.CompleteTime)
	require.Nil(t, payment.ReservationKey)
	var topUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&topUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	require.Equal(t, payment.CompleteTime, topUp.CompleteTime)
}
