package controller

import (
	"bytes"
	"encoding/csv"
	"fmt"
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
	statementNo := c.Param("statement_no")
	statement, err := model.GetBillingStatementByNo(statementNo, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	summaries, err := model.GetBillingStatementSummaries(statementNo)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	rows := make([]model.BillingBreakdownRow, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Dimension != model.BillingStatementSummaryDimensionMonthModelGroup {
			continue
		}
		rows = append(rows, model.BillingBreakdownRow{
			Period:           summary.Period,
			PeriodValue:      summary.PeriodValue,
			ModelName:        summary.ModelName,
			Group:            summary.Group,
			BillingSource:    summary.BillingSource,
			BillingMode:      summary.BillingMode,
			RequestCount:     summary.RequestCount,
			InputTokens:      summary.InputTokens,
			OutputTokens:     summary.OutputTokens,
			CacheReadTokens:  summary.CacheReadTokens,
			CacheWriteTokens: summary.CacheWriteTokens,
			OriginalAmount:   summary.OriginalAmount,
			DiscountAmount:   summary.DiscountAmount,
			SettlementAmount: summary.SettlementAmount,
		})
	}
	writeBillingBreakdownCSV(c, rows, fmt.Sprintf("monthly-billing-%s-%s.csv", statement.PeriodValue, statement.StatementNo))
}

func writeBillingBreakdownCSV(c *gin.Context, rows []model.BillingBreakdownRow, fileName string) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buf)
	_ = writer.Write([]string{
		"账期",
		"模型",
		"分组",
		"请求数",
		"输入 Tokens",
		"输出 Tokens",
		"缓存读取 Tokens",
		"缓存写入 Tokens",
		"原价(USD)",
		"分组折扣(USD)",
		"结算金额(USD)",
	})
	for _, row := range rows {
		_ = writer.Write([]string{
			row.PeriodValue,
			row.ModelName,
			row.Group,
			strconv.FormatInt(row.RequestCount, 10),
			strconv.FormatInt(row.InputTokens, 10),
			strconv.FormatInt(row.OutputTokens, 10),
			strconv.FormatInt(row.CacheReadTokens, 10),
			strconv.FormatInt(row.CacheWriteTokens, 10),
			formatBillingCSVUSDAmount(row.OriginalAmount),
			formatBillingCSVUSDAmount(row.DiscountAmount),
			formatBillingCSVUSDAmount(row.SettlementAmount),
		})
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
	c.Data(200, "text/csv; charset=utf-8", buf.Bytes())
}

func formatBillingCSVUSDAmount(quota int64) string {
	if common.QuotaPerUnit <= 0 {
		return "0.000000"
	}
	return strconv.FormatFloat(float64(quota)/common.QuotaPerUnit, 'f', 6, 64)
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
