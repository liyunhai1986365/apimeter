package model

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	BillingStatementWorkflowVersion = 1

	BillingDisputeStatusPending   = "pending"
	BillingDisputeStatusAccepted  = "accepted"
	BillingDisputeStatusRejected  = "rejected"
	BillingDisputeStatusWithdrawn = "withdrawn"

	BillingAdjustmentBalancePending = "pending"
	BillingAdjustmentBalanceSynced  = "synced"
	BillingAdjustmentBalanceFailed  = "failed"

	BillingStatementEventGenerated       = "generated"
	BillingStatementEventConfirmed       = "confirmed"
	BillingStatementEventDisputed        = "disputed"
	BillingStatementEventDisputeAccepted = "dispute_accepted"
	BillingStatementEventDisputeRejected = "dispute_rejected"
	BillingStatementEventAdjusted        = "adjusted"

	BillingActorSystem = "system"
	BillingActorUser   = "user"
	BillingActorAdmin  = "admin"
)

var (
	ErrBillingStatementNotConfirmable     = errors.New("账单当前不可确认")
	ErrBillingStatementRevisionChanged    = errors.New("账单已更新，请刷新后重新操作")
	ErrBillingStatementAlreadyConfirmed   = errors.New("账单已确认")
	ErrBillingStatementDisputeExists      = errors.New("当前账单版本已有处理中申诉")
	ErrBillingStatementInvalidDispute     = errors.New("申诉内容不完整")
	ErrBillingStatementInvalidAdjustment  = errors.New("调账金额或原因无效")
	ErrBillingStatementNegativeSettlement = errors.New("调账后账单金额不能小于零")
	ErrBillingStatementBalancePending     = errors.New("账单余额同步尚未完成")
)

type BillingStatementDispute struct {
	Id                int    `json:"id"`
	DisputeNo         string `json:"dispute_no" gorm:"type:varchar(64);uniqueIndex"`
	StatementNo       string `json:"statement_no" gorm:"type:varchar(64);index:idx_billing_dispute_statement_revision,priority:1;index"`
	UserId            int    `json:"user_id" gorm:"index"`
	StatementRevision int    `json:"statement_revision" gorm:"index:idx_billing_dispute_statement_revision,priority:2"`
	ReasonType        string `json:"reason_type" gorm:"type:varchar(32);index"`
	Description       string `json:"description" gorm:"type:text"`
	ExpectedAmount    int64  `json:"expected_amount" gorm:"type:bigint;default:0"`
	HasExpectedAmount bool   `json:"has_expected_amount" gorm:"default:false"`
	Status            string `json:"status" gorm:"type:varchar(32);index;default:'pending'"`
	Resolution        string `json:"resolution" gorm:"type:text"`
	OperatorUserId    int    `json:"operator_user_id" gorm:"index;default:0"`
	OperatorUsername  string `json:"operator_username" gorm:"type:varchar(64);default:''"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime;column:created_at;index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
	ResolvedAt        int64  `json:"resolved_at" gorm:"bigint;default:0"`
}

type BillingStatementAdjustment struct {
	Id                int    `json:"id"`
	AdjustmentNo      string `json:"adjustment_no" gorm:"type:varchar(64);uniqueIndex"`
	IdempotencyKey    string `json:"idempotency_key" gorm:"type:varchar(64);uniqueIndex"`
	StatementNo       string `json:"statement_no" gorm:"type:varchar(64);index"`
	UserId            int    `json:"user_id" gorm:"index"`
	StatementRevision int    `json:"statement_revision" gorm:"default:1"`
	Amount            int64  `json:"amount" gorm:"type:bigint"`
	AmountBefore      int64  `json:"amount_before" gorm:"type:bigint"`
	AmountAfter       int64  `json:"amount_after" gorm:"type:bigint"`
	Reason            string `json:"reason" gorm:"type:text"`
	DisputeId         int    `json:"dispute_id" gorm:"index;default:0"`
	OperatorUserId    int    `json:"operator_user_id" gorm:"index"`
	OperatorUsername  string `json:"operator_username" gorm:"type:varchar(64)"`
	BalanceSyncStatus string `json:"balance_sync_status" gorm:"type:varchar(32);index;default:'pending'"`
	BalanceSyncError  string `json:"balance_sync_error" gorm:"type:text"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime;column:created_at;index"`
	SyncedAt          int64  `json:"synced_at" gorm:"bigint;default:0"`
}

// BillingBalanceAdjustmentOperation is the main-database idempotency and
// balance audit record. The statement projection itself lives in LOG_DB.
type BillingBalanceAdjustmentOperation struct {
	Id              int    `json:"id"`
	AdjustmentNo    string `json:"adjustment_no" gorm:"type:varchar(64);uniqueIndex"`
	IdempotencyKey  string `json:"idempotency_key" gorm:"type:varchar(64);uniqueIndex"`
	StatementNo     string `json:"statement_no" gorm:"type:varchar(64);index"`
	UserId          int    `json:"user_id" gorm:"index"`
	Amount          int64  `json:"amount" gorm:"type:bigint"`
	Status          string `json:"status" gorm:"type:varchar(32);index;default:'pending'"`
	BalanceBefore   int64  `json:"balance_before" gorm:"type:bigint;default:0"`
	BalanceAfter    int64  `json:"balance_after" gorm:"type:bigint;default:0"`
	UsedQuotaBefore int64  `json:"used_quota_before" gorm:"type:bigint;default:0"`
	UsedQuotaAfter  int64  `json:"used_quota_after" gorm:"type:bigint;default:0"`
	ErrorMessage    string `json:"error_message" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime;column:created_at;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"autoUpdateTime;column:updated_at"`
}

type BillingStatementEvent struct {
	Id            int    `json:"id"`
	StatementNo   string `json:"statement_no" gorm:"type:varchar(64);index"`
	UserId        int    `json:"user_id" gorm:"index"`
	Revision      int    `json:"revision" gorm:"default:1"`
	EventType     string `json:"event_type" gorm:"type:varchar(32);index"`
	ActorType     string `json:"actor_type" gorm:"type:varchar(16);index"`
	ActorUserId   int    `json:"actor_user_id" gorm:"index;default:0"`
	ActorUsername string `json:"actor_username" gorm:"type:varchar(64);default:''"`
	Detail        string `json:"detail" gorm:"type:text"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime;column:created_at;index"`
}

type BillingStatementWorkflowDetail struct {
	Statement   BillingStatement             `json:"statement"`
	Summaries   []BillingStatementSummary    `json:"summaries"`
	Disputes    []BillingStatementDispute    `json:"disputes"`
	Adjustments []BillingStatementAdjustment `json:"adjustments"`
	Events      []BillingStatementEvent      `json:"events"`
}

type BillingAdminStatementItem struct {
	BillingStatement
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type BillingAdminStatementPage struct {
	Items  []BillingAdminStatementItem `json:"items"`
	Total  int64                       `json:"total"`
	Limit  int                         `json:"limit"`
	Offset int                         `json:"offset"`
}

type BillingAdminStatementQuery struct {
	UserId             int
	Username           string
	Month              string
	ConfirmationStatus string
	Limit              int
	Offset             int
}

type BillingStatementAdjustmentInput struct {
	StatementNo      string
	Amount           int64
	Reason           string
	DisputeId        int
	OperatorUserId   int
	OperatorUsername string
	IdempotencyKey   string
}

func migrateBillingStatementWorkflow() error {
	if err := LOG_DB.AutoMigrate(
		&BillingStatementDispute{},
		&BillingStatementAdjustment{},
		&BillingStatementEvent{},
	); err != nil {
		return err
	}
	if err := DB.AutoMigrate(&BillingBalanceAdjustmentOperation{}); err != nil {
		return err
	}

	// Product policy explicitly grandfathers every historical statement as
	// confirmed. Reconciliation remains a separate dimension so an old
	// exception is still diagnosable; WorkflowVersion makes this one-shot.
	var legacy []BillingStatement
	if err := LOG_DB.Where("workflow_version = ?", 0).Find(&legacy).Error; err != nil {
		return err
	}
	for i := range legacy {
		row := &legacy[i]
		updates := map[string]interface{}{
			"base_settlement_amount": row.SettlementAmount,
			"revision":               1,
			"workflow_version":       BillingStatementWorkflowVersion,
		}
		updates["status"] = BillingStatementStatusConfirmed
		updates["confirmation_status"] = BillingStatementConfirmationConfirmed
		updates["confirmed_revision"] = 1
		confirmedAt := row.FinalizedAt
		if confirmedAt == 0 {
			confirmedAt = row.GeneratedAt
		}
		if confirmedAt == 0 {
			confirmedAt = time.Now().Unix()
		}
		updates["confirmed_at"] = confirmedAt
		if row.Status == BillingStatementStatusException {
			updates["reconciliation_status"] = BillingStatementReconciliationException
		} else {
			updates["reconciliation_status"] = BillingStatementReconciliationMatched
		}
		if err := LOG_DB.Model(&BillingStatement{}).Where("id = ?", row.Id).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func ConfirmBillingStatement(statementNo string, userId int, expectedRevision int, username string) (BillingStatement, error) {
	var result BillingStatement
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("statement_no = ? AND user_id = ?", strings.TrimSpace(statementNo), userId).First(&result).Error; err != nil {
			return err
		}
		if result.Revision != expectedRevision {
			return ErrBillingStatementRevisionChanged
		}
		if result.ConfirmationStatus == BillingStatementConfirmationConfirmed && result.ConfirmedRevision == result.Revision {
			return ErrBillingStatementAlreadyConfirmed
		}
		if result.ReconciliationStatus != BillingStatementReconciliationMatched || result.DifferenceAmount != 0 || result.ConfirmationStatus == BillingStatementConfirmationDisputed {
			return ErrBillingStatementNotConfirmable
		}
		var pendingAdjustments int64
		if err := tx.Model(&BillingStatementAdjustment{}).
			Where("statement_no = ? AND balance_sync_status <> ?", result.StatementNo, BillingAdjustmentBalanceSynced).
			Count(&pendingAdjustments).Error; err != nil {
			return err
		}
		if pendingAdjustments > 0 {
			return ErrBillingStatementBalancePending
		}
		now := time.Now().Unix()
		result.ConfirmationStatus = BillingStatementConfirmationConfirmed
		result.ConfirmedRevision = result.Revision
		result.ConfirmedAt = now
		result.Status = BillingStatementStatusConfirmed
		if err := tx.Save(&result).Error; err != nil {
			return err
		}
		return recordBillingStatementEventTx(tx, result, BillingStatementEventConfirmed, BillingActorUser, userId, username, "")
	})
	return result, err
}

func CreateBillingStatementDispute(statementNo string, userId int, expectedRevision int, reasonType string, description string, expectedAmount *int64, username string) (BillingStatementDispute, error) {
	reasonType = strings.TrimSpace(reasonType)
	description = strings.TrimSpace(description)
	if reasonType == "" || utf8.RuneCountInString(description) < 5 || utf8.RuneCountInString(description) > 2000 {
		return BillingStatementDispute{}, ErrBillingStatementInvalidDispute
	}
	var dispute BillingStatementDispute
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		var statement BillingStatement
		if err := lockForUpdate(tx).Where("statement_no = ? AND user_id = ?", strings.TrimSpace(statementNo), userId).First(&statement).Error; err != nil {
			return err
		}
		if statement.Revision != expectedRevision {
			return ErrBillingStatementRevisionChanged
		}
		if statement.ConfirmationStatus == BillingStatementConfirmationConfirmed && statement.ConfirmedRevision == statement.Revision {
			return ErrBillingStatementAlreadyConfirmed
		}
		if statement.ReconciliationStatus != BillingStatementReconciliationMatched {
			return ErrBillingStatementNotConfirmable
		}
		var count int64
		if err := tx.Model(&BillingStatementDispute{}).
			Where("statement_no = ? AND statement_revision = ? AND status = ?", statement.StatementNo, statement.Revision, BillingDisputeStatusPending).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrBillingStatementDisputeExists
		}
		dispute = BillingStatementDispute{
			DisputeNo:         "DSP-" + strings.ToUpper(common.GetUUID()),
			StatementNo:       statement.StatementNo,
			UserId:            userId,
			StatementRevision: statement.Revision,
			ReasonType:        reasonType,
			Description:       description,
			Status:            BillingDisputeStatusPending,
		}
		if expectedAmount != nil {
			dispute.ExpectedAmount = *expectedAmount
			dispute.HasExpectedAmount = true
		}
		if err := tx.Create(&dispute).Error; err != nil {
			return err
		}
		statement.ConfirmationStatus = BillingStatementConfirmationDisputed
		statement.Status = BillingStatementConfirmationDisputed
		if err := tx.Save(&statement).Error; err != nil {
			return err
		}
		return recordBillingStatementEventTx(tx, statement, BillingStatementEventDisputed, BillingActorUser, userId, username, dispute.DisputeNo)
	})
	return dispute, err
}

func ResolveBillingStatementDispute(disputeId int, accept bool, resolution string, operatorUserId int, operatorUsername string) (BillingStatementDispute, error) {
	resolution = strings.TrimSpace(resolution)
	if utf8.RuneCountInString(resolution) < 2 || utf8.RuneCountInString(resolution) > 2000 {
		return BillingStatementDispute{}, ErrBillingStatementInvalidDispute
	}
	var dispute BillingStatementDispute
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", disputeId).First(&dispute).Error; err != nil {
			return err
		}
		if dispute.Status != BillingDisputeStatusPending {
			return errors.New("申诉已处理")
		}
		var statement BillingStatement
		if err := lockForUpdate(tx).Where("statement_no = ?", dispute.StatementNo).First(&statement).Error; err != nil {
			return err
		}
		now := time.Now().Unix()
		dispute.Resolution = resolution
		dispute.OperatorUserId = operatorUserId
		dispute.OperatorUsername = strings.TrimSpace(operatorUsername)
		dispute.ResolvedAt = now
		eventType := BillingStatementEventDisputeRejected
		if accept {
			dispute.Status = BillingDisputeStatusAccepted
			eventType = BillingStatementEventDisputeAccepted
		} else {
			dispute.Status = BillingDisputeStatusRejected
		}
		if err := tx.Save(&dispute).Error; err != nil {
			return err
		}
		statement.ConfirmationStatus = BillingStatementConfirmationPending
		statement.Status = BillingStatementStatusOpen
		if err := tx.Save(&statement).Error; err != nil {
			return err
		}
		return recordBillingStatementEventTx(tx, statement, eventType, BillingActorAdmin, operatorUserId, operatorUsername, resolution)
	})
	return dispute, err
}

func AdjustBillingStatement(input BillingStatementAdjustmentInput) (BillingStatementAdjustment, BillingStatement, error) {
	input.StatementNo = strings.TrimSpace(input.StatementNo)
	input.Reason = strings.TrimSpace(input.Reason)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.Amount == 0 || utf8.RuneCountInString(input.Reason) < 2 || utf8.RuneCountInString(input.Reason) > 2000 || input.IdempotencyKey == "" || len(input.IdempotencyKey) > 64 {
		return BillingStatementAdjustment{}, BillingStatement{}, ErrBillingStatementInvalidAdjustment
	}

	var existing BillingStatementAdjustment
	if err := LOG_DB.Where("idempotency_key = ?", input.IdempotencyKey).First(&existing).Error; err == nil {
		if existing.BalanceSyncStatus != BillingAdjustmentBalanceSynced {
			if retryErr := RetryBillingAdjustmentBalance(existing.AdjustmentNo); retryErr != nil {
				return existing, BillingStatement{}, retryErr
			}
			existing.BalanceSyncStatus = BillingAdjustmentBalanceSynced
		}
		statement, statementErr := GetBillingStatementByNo(existing.StatementNo, 0)
		return existing, statement, statementErr
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return BillingStatementAdjustment{}, BillingStatement{}, err
	}

	adjustmentNo := "ADJ-" + strings.ToUpper(common.GetUUID())
	operation := BillingBalanceAdjustmentOperation{
		AdjustmentNo:   adjustmentNo,
		IdempotencyKey: input.IdempotencyKey,
		StatementNo:    input.StatementNo,
		Amount:         input.Amount,
		Status:         BillingAdjustmentBalancePending,
	}

	var adjustment BillingStatementAdjustment
	var statement BillingStatement
	err := LOG_DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("statement_no = ?", input.StatementNo).First(&statement).Error; err != nil {
			return err
		}
		if (input.Amount > 0 && statement.SettlementAmount > math.MaxInt64-input.Amount) ||
			(input.Amount < 0 && statement.SettlementAmount < math.MinInt64-input.Amount) {
			return ErrBillingStatementInvalidAdjustment
		}
		amountAfter := statement.SettlementAmount + input.Amount
		if amountAfter < 0 {
			return ErrBillingStatementNegativeSettlement
		}
		operation.UserId = statement.UserId
		operationDB := DB
		if DB == LOG_DB {
			operationDB = tx
		}
		if err := operationDB.Where("idempotency_key = ?", input.IdempotencyKey).FirstOrCreate(&operation).Error; err != nil {
			return err
		}
		if input.DisputeId > 0 {
			var dispute BillingStatementDispute
			if err := tx.Where("id = ? AND statement_no = ? AND status = ?", input.DisputeId, statement.StatementNo, BillingDisputeStatusPending).First(&dispute).Error; err != nil {
				return err
			}
		}
		adjustment = BillingStatementAdjustment{
			AdjustmentNo:      operation.AdjustmentNo,
			IdempotencyKey:    input.IdempotencyKey,
			StatementNo:       statement.StatementNo,
			UserId:            statement.UserId,
			StatementRevision: statement.Revision,
			Amount:            input.Amount,
			AmountBefore:      statement.SettlementAmount,
			AmountAfter:       amountAfter,
			Reason:            input.Reason,
			DisputeId:         input.DisputeId,
			OperatorUserId:    input.OperatorUserId,
			OperatorUsername:  strings.TrimSpace(input.OperatorUsername),
			BalanceSyncStatus: BillingAdjustmentBalancePending,
		}
		if err := tx.Create(&adjustment).Error; err != nil {
			return err
		}
		statement.AdjustmentAmount += input.Amount
		statement.SettlementAmount = amountAfter
		statement.Revision++
		statement.ConfirmationStatus = BillingStatementConfirmationPending
		statement.Status = BillingStatementStatusOpen
		if err := tx.Save(&statement).Error; err != nil {
			return err
		}
		if input.DisputeId > 0 {
			now := time.Now().Unix()
			if err := tx.Model(&BillingStatementDispute{}).
				Where("id = ? AND statement_no = ? AND status = ?", input.DisputeId, statement.StatementNo, BillingDisputeStatusPending).
				Updates(map[string]interface{}{
					"status":            BillingDisputeStatusAccepted,
					"resolution":        input.Reason,
					"operator_user_id":  input.OperatorUserId,
					"operator_username": input.OperatorUsername,
					"resolved_at":       now,
				}).Error; err != nil {
				return err
			}
		}
		return recordBillingStatementEventTx(tx, statement, BillingStatementEventAdjusted, BillingActorAdmin, input.OperatorUserId, input.OperatorUsername, adjustment.AdjustmentNo)
	})
	if err != nil {
		return adjustment, statement, err
	}

	if err := syncBillingAdjustmentBalance(operation.AdjustmentNo); err != nil {
		_ = LOG_DB.Model(&BillingStatementAdjustment{}).Where("adjustment_no = ?", operation.AdjustmentNo).Updates(map[string]interface{}{
			"balance_sync_status": BillingAdjustmentBalanceFailed,
			"balance_sync_error":  err.Error(),
		}).Error
		return adjustment, statement, fmt.Errorf("账单已调账，但余额同步失败，可在后台重试: %w", err)
	}
	if err := LOG_DB.Model(&BillingStatementAdjustment{}).Where("adjustment_no = ?", operation.AdjustmentNo).Updates(map[string]interface{}{
		"balance_sync_status": BillingAdjustmentBalanceSynced,
		"balance_sync_error":  "",
		"synced_at":           time.Now().Unix(),
	}).Error; err != nil {
		return adjustment, statement, err
	}
	adjustment.BalanceSyncStatus = BillingAdjustmentBalanceSynced
	adjustment.SyncedAt = time.Now().Unix()
	return adjustment, statement, nil
}

func RetryBillingAdjustmentBalance(adjustmentNo string) error {
	if err := syncBillingAdjustmentBalance(strings.TrimSpace(adjustmentNo)); err != nil {
		_ = LOG_DB.Model(&BillingStatementAdjustment{}).Where("adjustment_no = ?", adjustmentNo).Updates(map[string]interface{}{
			"balance_sync_status": BillingAdjustmentBalanceFailed,
			"balance_sync_error":  err.Error(),
		}).Error
		return err
	}
	return LOG_DB.Model(&BillingStatementAdjustment{}).Where("adjustment_no = ?", adjustmentNo).Updates(map[string]interface{}{
		"balance_sync_status": BillingAdjustmentBalanceSynced,
		"balance_sync_error":  "",
		"synced_at":           time.Now().Unix(),
	}).Error
}

func syncBillingAdjustmentBalance(adjustmentNo string) error {
	var userId int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var operation BillingBalanceAdjustmentOperation
		if err := lockForUpdate(tx).Where("adjustment_no = ?", adjustmentNo).First(&operation).Error; err != nil {
			return err
		}
		userId = operation.UserId
		if operation.Status == BillingAdjustmentBalanceSynced {
			return nil
		}
		var user User
		if err := lockForUpdate(tx).Where("id = ?", operation.UserId).First(&user).Error; err != nil {
			return err
		}
		quotaBefore := int64(user.Quota)
		usedBefore := int64(user.UsedQuota)
		if (operation.Amount > 0 && (quotaBefore < math.MinInt64+operation.Amount || usedBefore > math.MaxInt64-operation.Amount)) ||
			(operation.Amount < 0 && (quotaBefore > math.MaxInt64+operation.Amount || usedBefore < -operation.Amount)) {
			return errors.New("调账后的用户余额或已用额度超出允许范围")
		}
		balanceAfter := quotaBefore - operation.Amount
		usedAfter := usedBefore + operation.Amount
		if balanceAfter > int64(maxInt()) || balanceAfter < int64(minInt()) || usedAfter > int64(maxInt()) || usedAfter < 0 {
			return errors.New("调账后的用户余额或已用额度超出允许范围")
		}
		operation.BalanceBefore = quotaBefore
		operation.BalanceAfter = balanceAfter
		operation.UsedQuotaBefore = usedBefore
		operation.UsedQuotaAfter = usedAfter
		operation.Status = BillingAdjustmentBalanceSynced
		operation.ErrorMessage = ""
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]interface{}{
			"quota":      int(balanceAfter),
			"used_quota": int(usedAfter),
		}).Error; err != nil {
			return err
		}
		return tx.Save(&operation).Error
	})
	if err != nil {
		_ = DB.Model(&BillingBalanceAdjustmentOperation{}).Where("adjustment_no = ?", adjustmentNo).Updates(map[string]interface{}{
			"status":        BillingAdjustmentBalanceFailed,
			"error_message": err.Error(),
		}).Error
		return err
	}
	if userId > 0 {
		_ = InvalidateUserCache(userId)
	}
	return nil
}

func GetBillingStatementWorkflowDetail(statementNo string, userId int) (BillingStatementWorkflowDetail, error) {
	var detail BillingStatementWorkflowDetail
	statement, err := GetBillingStatementByNo(statementNo, userId)
	if err != nil {
		return detail, err
	}
	detail.Statement = statement
	if detail.Summaries, err = GetBillingStatementSummaries(statement.StatementNo); err != nil {
		return detail, err
	}
	if err = LOG_DB.Where("statement_no = ?", statement.StatementNo).Order("id desc").Find(&detail.Disputes).Error; err != nil {
		return detail, err
	}
	if err = LOG_DB.Where("statement_no = ?", statement.StatementNo).Order("id asc").Find(&detail.Adjustments).Error; err != nil {
		return detail, err
	}
	if err = LOG_DB.Where("statement_no = ?", statement.StatementNo).Order("id asc").Find(&detail.Events).Error; err != nil {
		return detail, err
	}
	return detail, nil
}

func GetAdminBillingStatements(query BillingAdminStatementQuery) (BillingAdminStatementPage, error) {
	page := BillingAdminStatementPage{Limit: query.Limit, Offset: query.Offset}
	if page.Limit <= 0 || page.Limit > 100 {
		page.Limit = 20
	}
	if page.Offset < 0 {
		page.Offset = 0
	}
	userId := query.UserId
	if userId == 0 && strings.TrimSpace(query.Username) != "" {
		var user User
		if err := DB.Select("id").Where("username = ?", strings.TrimSpace(query.Username)).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return page, nil
			}
			return page, err
		}
		userId = user.Id
	}
	tx := LOG_DB.Model(&BillingStatement{}).Where("period = ?", BillingStatementPeriodMonth)
	if userId > 0 {
		tx = tx.Where("user_id = ?", userId)
	}
	if month := strings.TrimSpace(query.Month); month != "" {
		tx = tx.Where("period_value = ?", month)
	}
	if status := strings.TrimSpace(query.ConfirmationStatus); status != "" {
		tx = tx.Where("confirmation_status = ?", status)
	}
	if err := tx.Count(&page.Total).Error; err != nil {
		return page, err
	}
	var statements []BillingStatement
	if err := tx.Order("period_value desc, user_id asc").Limit(page.Limit).Offset(page.Offset).Find(&statements).Error; err != nil {
		return page, err
	}
	userIds := make([]int, 0, len(statements))
	for _, statement := range statements {
		userIds = append(userIds, statement.UserId)
	}
	var users []User
	if len(userIds) > 0 {
		if err := DB.Select("id", "username", "display_name", "email").Where("id IN ?", userIds).Find(&users).Error; err != nil {
			return page, err
		}
	}
	userMap := make(map[int]User, len(users))
	for _, user := range users {
		userMap[user.Id] = user
	}
	page.Items = make([]BillingAdminStatementItem, 0, len(statements))
	for _, statement := range statements {
		user := userMap[statement.UserId]
		page.Items = append(page.Items, BillingAdminStatementItem{
			BillingStatement: statement,
			Username:         user.Username,
			DisplayName:      user.DisplayName,
			Email:            user.Email,
		})
	}
	return page, nil
}

func recordBillingStatementEventTx(tx *gorm.DB, statement BillingStatement, eventType string, actorType string, actorUserId int, actorUsername string, detail string) error {
	return tx.Create(&BillingStatementEvent{
		StatementNo:   statement.StatementNo,
		UserId:        statement.UserId,
		Revision:      statement.Revision,
		EventType:     eventType,
		ActorType:     actorType,
		ActorUserId:   actorUserId,
		ActorUsername: strings.TrimSpace(actorUsername),
		Detail:        detail,
	}).Error
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func minInt() int {
	return -maxInt() - 1
}
