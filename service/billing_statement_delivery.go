package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"html"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

var ErrBillingStatementUserEmailMissing = errors.New("账单所属用户未绑定有效邮箱")

type BillingStatementEmailSender func(subject string, receiver string, content string, attachments []common.EmailAttachment) error

var BillingStatementEmailSenderFunc BillingStatementEmailSender = common.SendEmailWithAttachments

type BillingStatementEmailResult struct {
	StatementNo string `json:"statement_no"`
	UserId      int    `json:"user_id"`
	Email       string `json:"email"`
	FileName    string `json:"file_name"`
}

func BillingStatementCSVFileName(statement model.BillingStatement) string {
	return fmt.Sprintf("monthly-billing-%s-%s.csv", statement.PeriodValue, statement.StatementNo)
}

func BuildBillingStatementCSV(statement model.BillingStatement, summaries []model.BillingStatementSummary) ([]byte, error) {
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
			GroupRatio:       summary.GroupRatio,
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
	return BuildBillingBreakdownCSV(rows)
}

func BuildBillingBreakdownCSV(rows []model.BillingBreakdownRow) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF")
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"账期",
		"模型",
		"供应商",
		"请求数",
		"输入 Tokens",
		"输出 Tokens",
		"缓存读取 Tokens",
		"缓存写入 Tokens",
		"原价(USD)",
		"折扣",
		"结算金额(USD)",
	}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write([]string{
			row.PeriodValue,
			row.ModelName,
			row.Group,
			strconv.FormatInt(row.RequestCount, 10),
			strconv.FormatInt(row.InputTokens, 10),
			strconv.FormatInt(row.OutputTokens, 10),
			strconv.FormatInt(row.CacheReadTokens, 10),
			strconv.FormatInt(row.CacheWriteTokens, 10),
			formatBillingCSVUSDAmount(row.OriginalAmount),
			formatBillingCSVRatio(row.GroupRatio),
			formatBillingCSVUSDAmount(row.SettlementAmount),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func SendBillingStatementEmail(statementNo string) (BillingStatementEmailResult, error) {
	result := BillingStatementEmailResult{StatementNo: strings.TrimSpace(statementNo)}
	statement, err := model.GetBillingStatementByNo(result.StatementNo, 0)
	if err != nil {
		return result, err
	}
	result.UserId = statement.UserId

	user, err := model.GetUserById(statement.UserId, false)
	if err != nil {
		return result, err
	}
	receiver := strings.TrimSpace(user.Email)
	if err := common.Validate.Var(receiver, "required,email"); err != nil {
		return result, ErrBillingStatementUserEmailMissing
	}
	result.Email = receiver

	summaries, err := model.GetBillingStatementSummaries(statement.StatementNo)
	if err != nil {
		return result, err
	}
	csvData, err := BuildBillingStatementCSV(statement, summaries)
	if err != nil {
		return result, err
	}
	result.FileName = BillingStatementCSVFileName(statement)

	siteURL, err := EmailSiteURLForUser(statement.UserId)
	if err != nil {
		return result, err
	}
	content := buildBillingStatementEmailContent(statement, *user, system_setting.ServerAddress)
	content = RewriteEmailContentForSite(content, siteURL)
	subject := fmt.Sprintf("%s %s 月度账单", common.SystemName, statement.PeriodValue)
	if err := BillingStatementEmailSenderFunc(subject, receiver, content, []common.EmailAttachment{
		{
			Filename:    result.FileName,
			ContentType: "text/csv",
			Data:        csvData,
		},
	}); err != nil {
		return result, err
	}
	return result, nil
}

func buildBillingStatementEmailContent(statement model.BillingStatement, user model.User, serverAddress string) string {
	displayName := strings.TrimSpace(user.DisplayName)
	if displayName == "" {
		displayName = strings.TrimSpace(user.Username)
	}
	if displayName == "" {
		displayName = "用户"
	}
	status := billingStatementEmailStatus(statement)
	viewBlock := ""
	if baseURL := strings.TrimRight(strings.TrimSpace(serverAddress), "/"); baseURL != "" {
		viewURL := html.EscapeString(baseURL + "/billing/monthly")
		viewBlock = fmt.Sprintf(`<p style="margin:18px 0 0;"><a href="%s">登录查看账单详情</a></p>`, viewURL)
	}
	return fmt.Sprintf(
		`<!-- billing-statement -->
<p style="margin:0 0 16px;">您好，%s：</p>
<p style="margin:0 0 16px;">您在 %s 的 %s 月度账单已生成。本邮件附带按模型与供应商整理的 CSV 明细，请及时核对。</p>
<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="border-collapse:collapse;">
  <tr><td style="padding:7px 0;color:#64748b;">账单编号</td><td style="padding:7px 0;text-align:right;font-weight:600;">%s</td></tr>
  <tr><td style="padding:7px 0;color:#64748b;">账期</td><td style="padding:7px 0;text-align:right;font-weight:600;">%s</td></tr>
  <tr><td style="padding:7px 0;color:#64748b;">结算金额</td><td style="padding:7px 0;text-align:right;font-weight:700;color:#0f766e;">US$%s</td></tr>
  <tr><td style="padding:7px 0;color:#64748b;">请求数</td><td style="padding:7px 0;text-align:right;font-weight:600;">%s</td></tr>
  <tr><td style="padding:7px 0;color:#64748b;">账单状态</td><td style="padding:7px 0;text-align:right;font-weight:600;">%s</td></tr>
</table>
%s`,
		html.EscapeString(displayName),
		html.EscapeString(common.SystemName),
		html.EscapeString(statement.PeriodValue),
		html.EscapeString(statement.StatementNo),
		html.EscapeString(statement.PeriodValue),
		formatBillingCSVUSDAmount(statement.SettlementAmount),
		strconv.FormatInt(statement.RequestCount, 10),
		html.EscapeString(status),
		viewBlock,
	)
}

func billingStatementEmailStatus(statement model.BillingStatement) string {
	if statement.ReconciliationStatus == model.BillingStatementReconciliationException {
		return "异常，待处理"
	}
	switch statement.ConfirmationStatus {
	case model.BillingStatementConfirmationConfirmed:
		return "已确认"
	case model.BillingStatementConfirmationDisputed:
		return "申诉处理中"
	default:
		return "待确认"
	}
}

func formatBillingCSVRatio(ratio float64) string {
	if ratio <= 0 || ratio >= 1 {
		return ""
	}
	discount := math.Round((1-ratio)*100000) / 1000
	return "-" + strconv.FormatFloat(discount, 'f', -1, 64) + "%"
}

func formatBillingCSVUSDAmount(quota int64) string {
	if common.QuotaPerUnit <= 0 {
		return "0.000000"
	}
	return strconv.FormatFloat(float64(quota)/common.QuotaPerUnit, 'f', 6, 64)
}
