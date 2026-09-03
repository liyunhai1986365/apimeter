package controller

import (
	"fmt"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
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

func billingV2BreakdownQuery(c *gin.Context, userId int) model.BillingBreakdownQuery {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	return model.BillingBreakdownQuery{
		UserId:        userId,
		Period:        c.Query("period"),
		StartDate:     c.Query("start_date"),
		EndDate:       c.Query("end_date"),
		Month:         c.Query("month"),
		ModelName:     c.Query("model_name"),
		Group:         c.Query("group"),
		BillingSource: c.Query("billing_source"),
		BillingMode:   c.Query("billing_mode"),
		Limit:         limit,
		Offset:        offset,
	}
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

func ListBillingBreakdowns(c *gin.Context) {
	items, err := model.GetBillingBreakdownRows(billingV2BreakdownQuery(c, c.GetInt("id")))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, items)
}

func ExportBillingBreakdowns(c *gin.Context) {
	query := billingV2BreakdownQuery(c, c.GetInt("id"))
	query.NoPagination = true
	rows, err := model.GetBillingBreakdownRows(query)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	writeBillingBreakdownCSV(c, rows, billingBreakdownExportFileName(query))
}

func ExportBillingMonthlyStatement(c *gin.Context) {
	exportBillingMonthlyStatement(c, c.GetInt("id"))
}

func exportBillingMonthlyStatement(c *gin.Context, userId int) {
	statement, err := model.GetBillingStatementByNo(c.Param("statement_no"), userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summaries, err := model.GetBillingStatementSummaries(statement.StatementNo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	data, err := service.BuildBillingStatementCSV(statement, summaries)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeBillingCSV(c, data, service.BillingStatementCSVFileName(statement))
}

func writeBillingBreakdownCSV(c *gin.Context, rows []model.BillingBreakdownRow, fileName string) {
	data, err := service.BuildBillingBreakdownCSV(rows)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	writeBillingCSV(c, data, fileName)
}

func writeBillingCSV(c *gin.Context, data []byte, fileName string) {
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Data(200, "text/csv; charset=utf-8", data)
}

func billingBreakdownExportFileName(query model.BillingBreakdownQuery) string {
	period := query.Period
	if period == "" {
		period = model.BillingStatementPeriodMonth
	}
	if period == model.BillingStatementPeriodDay {
		start := query.StartDate
		end := query.EndDate
		if start == "" {
			start = "start"
		}
		if end == "" {
			end = "end"
		}
		return fmt.Sprintf("daily-billing-%s-%s.csv", start, end)
	}
	month := query.Month
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}
	return fmt.Sprintf("monthly-billing-%s.csv", month)
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

func GenerateRecentMonthlyBillingStatements(c *gin.Context) {
	batchSize, _ := strconv.Atoi(c.Query("batch_size"))
	result, err := model.GenerateRecentMonthlyBillingStatements(time.Now().UTC(), batchSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, result)
}
