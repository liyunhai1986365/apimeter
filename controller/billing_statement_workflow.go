package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

type confirmBillingStatementRequest struct {
	Revision int `json:"revision"`
}

type createBillingDisputeRequest struct {
	Revision          int    `json:"revision"`
	ReasonType        string `json:"reason_type"`
	Description       string `json:"description"`
	ExpectedAmount    int64  `json:"expected_amount"`
	HasExpectedAmount bool   `json:"has_expected_amount"`
}

type resolveBillingDisputeRequest struct {
	Action     string `json:"action"`
	Resolution string `json:"resolution"`
}

type adjustBillingStatementRequest struct {
	Amount         int64  `json:"amount"`
	Reason         string `json:"reason"`
	DisputeId      int    `json:"dispute_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

func GetBillingStatementWorkflow(c *gin.Context) {
	detail, err := model.GetBillingStatementWorkflowDetail(c.Param("statement_no"), c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func ConfirmBillingStatement(c *gin.Context) {
	var req confirmBillingStatementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	statement, err := model.ConfirmBillingStatement(c.Param("statement_no"), c.GetInt("id"), req.Revision, c.GetString("username"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, statement)
}

func CreateBillingStatementDispute(c *gin.Context) {
	var req createBillingDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	var expectedAmount *int64
	if req.HasExpectedAmount {
		expectedAmount = &req.ExpectedAmount
	}
	dispute, err := model.CreateBillingStatementDispute(
		c.Param("statement_no"),
		c.GetInt("id"),
		req.Revision,
		req.ReasonType,
		req.Description,
		expectedAmount,
		c.GetString("username"),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dispute)
}

func ListAdminBillingStatements(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	page, err := model.GetAdminBillingStatements(model.BillingAdminStatementQuery{
		UserId:             userId,
		Username:           c.Query("username"),
		Month:              c.Query("month"),
		ConfirmationStatus: c.Query("confirmation_status"),
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, page)
}

func GetAdminBillingStatement(c *gin.Context) {
	detail, err := model.GetBillingStatementWorkflowDetail(c.Param("statement_no"), 0)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, detail)
}

func AdjustAdminBillingStatement(c *gin.Context) {
	var req adjustBillingStatementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	adjustment, statement, err := model.AdjustBillingStatement(model.BillingStatementAdjustmentInput{
		StatementNo:      c.Param("statement_no"),
		Amount:           req.Amount,
		Reason:           req.Reason,
		DisputeId:        req.DisputeId,
		OperatorUserId:   c.GetInt("id"),
		OperatorUsername: c.GetString("username"),
		IdempotencyKey:   req.IdempotencyKey,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"adjustment": adjustment, "statement": statement})
}

func RetryAdminBillingAdjustment(c *gin.Context) {
	if err := model.RetryBillingAdjustmentBalance(c.Param("adjustment_no")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"adjustment_no": c.Param("adjustment_no")})
}

func ResolveAdminBillingDispute(c *gin.Context) {
	disputeId, err := strconv.Atoi(c.Param("dispute_id"))
	if err != nil || disputeId <= 0 {
		common.ApiError(c, model.ErrBillingStatementInvalidDispute)
		return
	}
	var req resolveBillingDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	action := strings.TrimSpace(req.Action)
	if action != "accept" && action != "reject" {
		common.ApiError(c, model.ErrBillingStatementInvalidDispute)
		return
	}
	dispute, err := model.ResolveBillingStatementDispute(
		disputeId,
		action == "accept",
		req.Resolution,
		c.GetInt("id"),
		c.GetString("username"),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, dispute)
}

func GenerateAdminBillingMonthlyStatement(c *gin.Context) {
	userId, _ := strconv.Atoi(c.Query("user_id"))
	month := strings.TrimSpace(c.Query("month"))
	if userId <= 0 || month == "" {
		common.ApiError(c, model.ErrBillingStatementInvalidAdjustment)
		return
	}
	statement, summaries, err := model.GenerateMonthlyBillingStatement(userId, month)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"statement": statement, "summaries": summaries})
}
