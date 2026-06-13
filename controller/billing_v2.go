package controller

import (
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func billingV2LedgerQuery(c *gin.Context, userId int) model.AccountLedgerQuery {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	return model.AccountLedgerQuery{
		UserId:      userId,
		StartTime:   startTime,
		EndTime:     endTime,
		LedgerMonth: c.Query("month"),
		EntryType:   c.Query("entry_type"),
		Limit:       limit,
		Offset:      offset,
	}
}

func billingV2StatementQuery(c *gin.Context, userId int) model.BillingStatementQuery {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	return model.BillingStatementQuery{
		UserId:     userId,
		StartMonth: c.Query("start_month"),
		EndMonth:   c.Query("end_month"),
		Period:     c.Query("period"),
		Limit:      limit,
		Offset:     offset,
	}
}

func GetBillingCurrentPeriod(c *gin.Context) {
	userId := c.GetInt("id")
	now := time.Now().UTC()
	month := now.Format("2006-01")
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Unix()
	end := now.Unix()
	total, err := model.GetBillingUsageTotals(model.BillingUsageItemQuery{
		UserId:    userId,
		StartTime: start,
		EndTime:   end,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"month":   month,
		"status":  "estimated",
		"summary": total,
	})
}

func ListAccountLedgerEntries(c *gin.Context) {
	items, err := model.GetAccountLedgerEntries(billingV2LedgerQuery(c, c.GetInt("id")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func ListBillingMonthlyStatements(c *gin.Context) {
	items, err := model.GetBillingStatements(billingV2StatementQuery(c, c.GetInt("id")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func GenerateBillingMonthlyStatement(c *gin.Context) {
	month := c.Query("month")
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}
	statement, summaries, err := model.GenerateMonthlyBillingStatement(c.GetInt("id"), month)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{
		"statement": statement,
		"summaries": summaries,
	})
}

func GetBillingMonthlyStatementSummaries(c *gin.Context) {
	items, err := model.GetBillingStatementSummaries(c.Param("statement_no"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func ListDailyBillingReconciliations(c *gin.Context) {
	items, err := model.GetDailyBillingReconciliations(billingV2StatementQuery(c, c.GetInt("id")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func BackfillBillingV2(c *gin.Context) {
	startTime, _ := strconv.ParseInt(c.Query("start_time"), 10, 64)
	endTime, _ := strconv.ParseInt(c.Query("end_time"), 10, 64)
	batchSize, _ := strconv.Atoi(c.Query("batch_size"))
	result, err := model.BackfillBillingV2FromLogs(model.BillingV2BackfillOptions{
		StartTimestamp: startTime,
		EndTimestamp:   endTime,
		BatchSize:      batchSize,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
