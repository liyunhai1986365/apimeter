package common

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSendAllEmailTemplatesManual(t *testing.T) {
	if os.Getenv("SEND_EMAIL_TEMPLATE_TESTS") != "1" {
		t.Skip("set SEND_EMAIL_TEMPLATE_TESTS=1 to send real email template samples")
	}

	receiver := os.Getenv("EMAIL_TEMPLATE_TEST_RECEIVER")
	if receiver == "" {
		t.Fatal("EMAIL_TEMPLATE_TEST_RECEIVER is required")
	}

	dbPath := os.Getenv("EMAIL_TEMPLATE_TEST_DB")
	if dbPath == "" {
		dbPath = "../dev-one-api.db"
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open options database: %v", err)
	}

	options := map[string]string{}
	rows, err := db.Table("options").
		Select("key, value").
		Where("key IN ?", []string{"SMTPServer", "SMTPPort", "SMTPAccount", "SMTPFrom", "SMTPToken", "SMTPSSLEnabled", "SMTPForceAuthLogin", "SystemName"}).
		Rows()
	if err != nil {
		t.Fatalf("query smtp options: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatalf("scan smtp option: %v", err)
		}
		options[key] = value
	}
	if options["SMTPServer"] == "" || options["SMTPAccount"] == "" || options["SMTPToken"] == "" {
		t.Fatal("SMTP options are incomplete")
	}

	SMTPServer = options["SMTPServer"]
	SMTPAccount = options["SMTPAccount"]
	SMTPFrom = options["SMTPFrom"]
	SMTPToken = options["SMTPToken"]
	SMTPSSLEnabled = options["SMTPSSLEnabled"] == "true"
	SMTPForceAuthLogin = options["SMTPForceAuthLogin"] == "true"
	if options["SystemName"] != "" {
		SystemName = options["SystemName"]
	}
	if port, err := strconv.Atoi(options["SMTPPort"]); err == nil && port > 0 {
		SMTPPort = port
	}

	samples := []struct {
		subject string
		content string
	}{
		{
			subject: fmt.Sprintf("[邮件模板测试] %s邮箱验证邮件", SystemName),
			content: fmt.Sprintf("<p>您好，你正在进行%s邮箱验证。</p><p>您的验证码为: <strong>123456</strong></p><p>验证码 10 分钟内有效，如果不是本人操作，请忽略。</p>", SystemName),
		},
		{
			subject: fmt.Sprintf("[邮件模板测试] %s密码重置", SystemName),
			content: fmt.Sprintf("<p>您好，你正在进行%s密码重置。</p><p>点击 <a href='https://modelsell.com/user/reset?email=xsheji@qq.com&token=test-token'>此处</a> 进行密码重置。</p><p>如果链接无法点击，请复制链接到浏览器打开。</p>", SystemName),
		},
		{
			subject: "[邮件模板测试] 您的额度即将用尽",
			content: "您的额度即将用尽，当前剩余额度为 $0.82，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='https://modelsell.com/wallet'>https://modelsell.com/wallet</a>",
		},
		{
			subject: "[邮件模板测试] 您的订阅额度即将用尽",
			content: "您的订阅额度即将用尽，当前剩余额度为 410,000 点额度，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='https://modelsell.com/wallet'>https://modelsell.com/wallet</a>",
		},
		{
			subject: "[邮件模板测试] 通道「示例通道」（#1001）已被禁用",
			content: "渠道状态变更：通道「示例通道」（#1001）已被禁用，原因：连续请求失败超过阈值。",
		},
		{
			subject: "[邮件模板测试] 上游模型巡检通知",
			content: "上游模型巡检完成：检查通道 12 个，发现模型新增 8 个，移除 2 个，失败通道 1 个。请前往管理后台查看详情。",
		},
		{
			subject: "[邮件模板测试] 系统通知",
			content: "这是一封普通系统通知模板测试邮件，用于确认基础通知样式、页眉、内容卡片和页脚是否正常显示。",
		},
	}

	for _, sample := range samples {
		if err := SendEmail(sample.subject, receiver, sample.content); err != nil {
			t.Fatalf("send %q: %v", sample.subject, err)
		}
	}
}
