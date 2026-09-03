package service

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupBillingStatementDeliveryTest(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalUsingSQLite := common.UsingSQLite
	originalUsingMySQL := common.UsingMySQL
	originalUsingPostgreSQL := common.UsingPostgreSQL
	originalRedisEnabled := common.RedisEnabled
	originalSender := BillingStatementEmailSenderFunc
	originalServerAddress := system_setting.ServerAddress
	originalQuotaPerUnit := common.QuotaPerUnit

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.User{},
		&model.BillingStatement{},
		&model.BillingStatementSummary{},
		&model.Agent{},
		&model.AgentUser{},
		&model.AgentDomain{},
	))
	model.DB = db
	model.LOG_DB = db
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	common.QuotaPerUnit = 500000
	system_setting.ServerAddress = "https://modelsell.com"
	model.InitColForTest()

	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.UsingSQLite = originalUsingSQLite
		common.UsingMySQL = originalUsingMySQL
		common.UsingPostgreSQL = originalUsingPostgreSQL
		common.RedisEnabled = originalRedisEnabled
		common.QuotaPerUnit = originalQuotaPerUnit
		BillingStatementEmailSenderFunc = originalSender
		system_setting.ServerAddress = originalServerAddress
		model.InitColForTest()
	})
}

func TestSendBillingStatementEmailSendsOneUserWithCSVAttachment(t *testing.T) {
	setupBillingStatementDeliveryTest(t)
	require.NoError(t, model.DB.Create(&model.User{
		Id:          21,
		Username:    "billing-user",
		DisplayName: "Billing User",
		Email:       "billing@example.com",
		Password:    "password123",
		Role:        common.RoleCommonUser,
		Status:      common.UserStatusEnabled,
		Group:       "default",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Agent{Id: 3, Name: "Agent", Slug: "billing-agent", Status: model.AgentStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentUser{AgentId: 3, UserId: 21, Status: model.AgentUserStatusEnabled}).Error)
	require.NoError(t, model.DB.Create(&model.AgentDomain{AgentId: 3, Domain: "agent.example.com", Status: model.AgentDomainStatusActive}).Error)

	statement := model.BillingStatement{
		StatementNo:          "BILL-202608-21",
		UserId:               21,
		Period:               model.BillingStatementPeriodMonth,
		PeriodValue:          "2026-08",
		SettlementAmount:     625000,
		RequestCount:         42,
		ReconciliationStatus: model.BillingStatementReconciliationMatched,
		ConfirmationStatus:   model.BillingStatementConfirmationPending,
		Status:               model.BillingStatementStatusOpen,
	}
	require.NoError(t, model.LOG_DB.Create(&statement).Error)
	require.NoError(t, model.LOG_DB.Create(&model.BillingStatementSummary{
		StatementNo:      statement.StatementNo,
		UserId:           statement.UserId,
		Period:           statement.Period,
		PeriodValue:      statement.PeriodValue,
		Dimension:        model.BillingStatementSummaryDimensionMonthModelGroup,
		ModelName:        "gpt-5",
		Group:            "default",
		GroupRatio:       0.8,
		RequestCount:     42,
		OriginalAmount:   750000,
		SettlementAmount: 625000,
	}).Error)

	var gotSubject, gotReceiver, gotContent string
	var gotAttachments []common.EmailAttachment
	BillingStatementEmailSenderFunc = func(subject string, receiver string, content string, attachments []common.EmailAttachment) error {
		gotSubject = subject
		gotReceiver = receiver
		gotContent = content
		gotAttachments = attachments
		return nil
	}

	result, err := SendBillingStatementEmail(statement.StatementNo)

	require.NoError(t, err)
	require.Equal(t, statement.StatementNo, result.StatementNo)
	require.Equal(t, "billing@example.com", result.Email)
	require.Equal(t, "billing@example.com", gotReceiver)
	require.Contains(t, gotSubject, "2026-08")
	require.Contains(t, gotContent, "https://agent.example.com/billing/monthly")
	require.Contains(t, gotContent, "US$1.250000")
	require.Len(t, gotAttachments, 1)
	require.Equal(t, "monthly-billing-2026-08-BILL-202608-21.csv", gotAttachments[0].Filename)
	require.Contains(t, string(gotAttachments[0].Data), "gpt-5")
	require.Contains(t, string(gotAttachments[0].Data), "-20%")
}

func TestSendBillingStatementEmailRejectsUserWithoutEmail(t *testing.T) {
	setupBillingStatementDeliveryTest(t)
	require.NoError(t, model.DB.Create(&model.User{
		Id: 22, Username: "no-email", Password: "password123", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default",
	}).Error)
	require.NoError(t, model.LOG_DB.Create(&model.BillingStatement{
		StatementNo: "BILL-202608-22", UserId: 22, Period: model.BillingStatementPeriodMonth, PeriodValue: "2026-08",
	}).Error)
	called := false
	BillingStatementEmailSenderFunc = func(string, string, string, []common.EmailAttachment) error {
		called = true
		return nil
	}

	_, err := SendBillingStatementEmail("BILL-202608-22")

	require.ErrorIs(t, err, ErrBillingStatementUserEmailMissing)
	require.False(t, called)
}
