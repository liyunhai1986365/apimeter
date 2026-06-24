package common

import (
	"strings"
	"testing"
)

func TestRenderEmailTemplateHighlightsVerificationCode(t *testing.T) {
	html := renderEmailTemplate("New API邮箱验证邮件", "<p>您好，你正在进行New API邮箱验证。</p><p>您的验证码为: <strong>123456</strong></p>")

	if !strings.Contains(html, "<!doctype html>") {
		t.Fatalf("expected full html document, got %s", html)
	}
	if !strings.Contains(html, "123456") {
		t.Fatalf("expected verification code to be preserved, got %s", html)
	}
	if !strings.Contains(html, "letter-spacing:8px") {
		t.Fatalf("expected verification code to use prominent code styling, got %s", html)
	}
	if !strings.Contains(html, "Security check") {
		t.Fatalf("expected verification template status label, got %s", html)
	}
}

func TestRenderEmailTemplatePromotesResetLinkToButton(t *testing.T) {
	html := renderEmailTemplate("New API密码重置", "<p>点击 <a href='https://example.com/reset'>此处</a> 进行密码重置。</p>")

	if !strings.Contains(html, "https://example.com/reset") {
		t.Fatalf("expected reset link to be preserved, got %s", html)
	}
	if !strings.Contains(html, "Reset password") {
		t.Fatalf("expected reset password button copy, got %s", html)
	}
	if !strings.Contains(html, "Action required") {
		t.Fatalf("expected reset template status label, got %s", html)
	}
}

func TestRenderEmailTemplateWrapsNotificationContent(t *testing.T) {
	html := renderEmailTemplate("系统通知", "您的设置已保存")

	if !strings.Contains(html, "您的设置已保存") {
		t.Fatalf("expected notification content to be preserved, got %s", html)
	}
	if !strings.Contains(html, "System notice") {
		t.Fatalf("expected generic notification status label, got %s", html)
	}
	if !strings.Contains(html, SystemName) {
		t.Fatalf("expected system name in template, got %s", html)
	}
}

func TestRenderEmailTemplateUsesOperationsStyleForChannelNotice(t *testing.T) {
	html := renderEmailTemplate("通道测试完成", "所有通道测试已完成")

	if !strings.Contains(html, "Operations update") {
		t.Fatalf("expected operations status label, got %s", html)
	}
	if !strings.Contains(html, "所有通道测试已完成") {
		t.Fatalf("expected channel notice content to be preserved, got %s", html)
	}
}
