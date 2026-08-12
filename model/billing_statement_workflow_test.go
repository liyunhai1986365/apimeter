package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateBillingStatementWorkflowConfirmsAllHistoricalStatements(t *testing.T) {
	truncateTables(t)
	now := time.Now().Unix()
	legacy := []BillingStatement{
		{
			StatementNo:      "BILL-M-11-2026-06",
			UserId:           11,
			Period:           BillingStatementPeriodMonth,
			PeriodValue:      "2026-06",
			SettlementAmount: 1200,
			Status:           BillingStatementStatusOpen,
			GeneratedAt:      now,
			FinalizedAt:      now,
			WorkflowVersion:  0,
		},
		{
			StatementNo:      "BILL-M-12-2026-07",
			UserId:           12,
			Period:           BillingStatementPeriodMonth,
			PeriodValue:      "2026-07",
			SettlementAmount: 800,
			Status:           BillingStatementStatusException,
			GeneratedAt:      now,
			WorkflowVersion:  0,
		},
	}
	require.NoError(t, LOG_DB.Create(&legacy).Error)

	require.NoError(t, migrateBillingStatementWorkflow())

	var rows []BillingStatement
	require.NoError(t, LOG_DB.Where("statement_no IN ?", []string{legacy[0].StatementNo, legacy[1].StatementNo}).Order("id asc").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, BillingStatementStatusConfirmed, row.Status)
		require.Equal(t, BillingStatementConfirmationConfirmed, row.ConfirmationStatus)
		require.Equal(t, 1, row.Revision)
		require.Equal(t, 1, row.ConfirmedRevision)
		require.Equal(t, BillingStatementWorkflowVersion, row.WorkflowVersion)
		require.NotZero(t, row.ConfirmedAt)
		require.Equal(t, row.SettlementAmount, row.BaseSettlementAmount)
	}
	require.Equal(t, BillingStatementReconciliationMatched, rows[0].ReconciliationStatus)
	require.Equal(t, BillingStatementReconciliationException, rows[1].ReconciliationStatus)
}

func TestBillingAdjustmentWithSeparateMainAndLogDatabases(t *testing.T) {
	oldDB, oldLogDB := DB, LOG_DB
	mainDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:billing-main-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	logDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:billing-log-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	DB, LOG_DB = mainDB, logDB
	t.Cleanup(func() {
		DB, LOG_DB = oldDB, oldLogDB
	})
	require.NoError(t, DB.AutoMigrate(&User{}, &BillingBalanceAdjustmentOperation{}))
	require.NoError(t, LOG_DB.AutoMigrate(
		&BillingStatement{},
		&BillingStatementSummary{},
		&BillingStatementDispute{},
		&BillingStatementAdjustment{},
		&BillingStatementEvent{},
	))

	user := User{Username: "separate-db-user", Password: "password", Quota: 5_000, UsedQuota: 1_000}
	require.NoError(t, DB.Create(&user).Error)
	statement := BillingStatement{
		StatementNo:          "BILL-M-SEPARATE-2026-07",
		UserId:               user.Id,
		Period:               BillingStatementPeriodMonth,
		PeriodValue:          "2026-07",
		SettlementAmount:     500,
		BaseSettlementAmount: 500,
		Status:               BillingStatementStatusOpen,
		ReconciliationStatus: BillingStatementReconciliationMatched,
		ConfirmationStatus:   BillingStatementConfirmationPending,
		Revision:             1,
		WorkflowVersion:      BillingStatementWorkflowVersion,
	}
	require.NoError(t, LOG_DB.Create(&statement).Error)

	adjustment, adjusted, err := AdjustBillingStatement(BillingStatementAdjustmentInput{
		StatementNo:      statement.StatementNo,
		Amount:           100,
		Reason:           "补收遗漏的使用金额",
		OperatorUserId:   99,
		OperatorUsername: "admin",
		IdempotencyKey:   "separate-db-adjustment",
	})
	require.NoError(t, err)
	require.Equal(t, BillingAdjustmentBalanceSynced, adjustment.BalanceSyncStatus)
	require.Equal(t, int64(600), adjusted.SettlementAmount)

	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	require.Equal(t, 4_900, updatedUser.Quota)
	require.Equal(t, 1_100, updatedUser.UsedQuota)
	var operation BillingBalanceAdjustmentOperation
	require.NoError(t, DB.Where("adjustment_no = ?", adjustment.AdjustmentNo).First(&operation).Error)
	require.Equal(t, BillingAdjustmentBalanceSynced, operation.Status)
}

func TestBillingStatementDisputeAdjustmentAndConfirmationWorkflow(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(
		&BillingStatementDispute{},
		&BillingStatementAdjustment{},
		&BillingStatementEvent{},
		&BillingBalanceAdjustmentOperation{},
	))

	user := User{
		Username:  "billing-workflow-user",
		Password:  "password",
		Quota:     10_000,
		UsedQuota: 2_000,
	}
	require.NoError(t, DB.Create(&user).Error)
	statement := BillingStatement{
		StatementNo:          "BILL-M-WORKFLOW-2026-07",
		UserId:               user.Id,
		Period:               BillingStatementPeriodMonth,
		PeriodValue:          "2026-07",
		SettlementAmount:     1_000,
		BaseSettlementAmount: 1_000,
		Status:               BillingStatementStatusOpen,
		ReconciliationStatus: BillingStatementReconciliationMatched,
		ConfirmationStatus:   BillingStatementConfirmationPending,
		Revision:             1,
		WorkflowVersion:      BillingStatementWorkflowVersion,
		GeneratedAt:          time.Now().Unix(),
	}
	require.NoError(t, LOG_DB.Create(&statement).Error)

	expectedAmount := int64(800)
	dispute, err := CreateBillingStatementDispute(
		statement.StatementNo,
		user.Id,
		1,
		"amount",
		"日志金额与账单金额不一致",
		&expectedAmount,
		user.Username,
	)
	require.NoError(t, err)
	require.Equal(t, BillingDisputeStatusPending, dispute.Status)

	input := BillingStatementAdjustmentInput{
		StatementNo:      statement.StatementNo,
		Amount:           -200,
		Reason:           "依据用户申诉减少账单金额",
		DisputeId:        dispute.Id,
		OperatorUserId:   99,
		OperatorUsername: "admin",
		IdempotencyKey:   "billing-workflow-adjustment-1",
	}
	adjustment, adjusted, err := AdjustBillingStatement(input)
	require.NoError(t, err)
	require.Equal(t, BillingAdjustmentBalanceSynced, adjustment.BalanceSyncStatus)
	require.Equal(t, int64(800), adjusted.SettlementAmount)
	require.Equal(t, int64(-200), adjusted.AdjustmentAmount)
	require.Equal(t, 2, adjusted.Revision)
	require.Equal(t, BillingStatementConfirmationPending, adjusted.ConfirmationStatus)

	var updatedUser User
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	require.Equal(t, 10_200, updatedUser.Quota)
	require.Equal(t, 1_800, updatedUser.UsedQuota)

	duplicate, duplicateStatement, err := AdjustBillingStatement(input)
	require.NoError(t, err)
	require.Equal(t, adjustment.AdjustmentNo, duplicate.AdjustmentNo)
	require.Equal(t, int64(800), duplicateStatement.SettlementAmount)
	require.NoError(t, DB.First(&updatedUser, user.Id).Error)
	require.Equal(t, 10_200, updatedUser.Quota)
	require.Equal(t, 1_800, updatedUser.UsedQuota)

	var resolved BillingStatementDispute
	require.NoError(t, LOG_DB.First(&resolved, dispute.Id).Error)
	require.Equal(t, BillingDisputeStatusAccepted, resolved.Status)

	confirmed, err := ConfirmBillingStatement(statement.StatementNo, user.Id, 2, user.Username)
	require.NoError(t, err)
	require.Equal(t, BillingStatementStatusConfirmed, confirmed.Status)
	require.Equal(t, BillingStatementConfirmationConfirmed, confirmed.ConfirmationStatus)
	require.Equal(t, 2, confirmed.ConfirmedRevision)

	detail, err := GetBillingStatementWorkflowDetail(statement.StatementNo, user.Id)
	require.NoError(t, err)
	require.Len(t, detail.Adjustments, 1)
	require.Len(t, detail.Disputes, 1)
	require.GreaterOrEqual(t, len(detail.Events), 3)

	page, err := GetAdminBillingStatements(BillingAdminStatementQuery{
		Username: user.Username,
		Month:    statement.PeriodValue,
		Limit:    20,
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	require.Equal(t, user.Username, page.Items[0].Username)
}
