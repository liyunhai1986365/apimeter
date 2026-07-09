package common

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

type emailTemplateKind struct {
	Label       string
	Title       string
	Intro       string
	ActionText  string
	AccentColor string
}

var (
	emailCodePattern = regexp.MustCompile(`<strong>\s*([0-9A-Za-z]{4,12})\s*</strong>`)
	emailLinkPattern = regexp.MustCompile(`<a\s+[^>]*href=['"]([^'"]+)['"][^>]*>.*?</a>`)
)

func emailKind(subject string, content string) emailTemplateKind {
	lower := strings.ToLower(subject + " " + content)
	switch {
	case strings.Contains(lower, "announcement-type:"):
		return emailTemplateKind{
			Label:       "Platform announcement",
			Title:       subject,
			Intro:       "平台发布了新的公告，请查看下方详情。",
			AccentColor: "#4f46e5",
		}
	case strings.Contains(subject, "邮箱验证") || strings.Contains(lower, "verification"):
		return emailTemplateKind{
			Label:       "Security check",
			Title:       "验证您的邮箱",
			Intro:       "请使用下方验证码完成邮箱验证。验证码仅在短时间内有效。",
			AccentColor: "#2563eb",
		}
	case strings.Contains(subject, "密码重置") || strings.Contains(lower, "reset"):
		return emailTemplateKind{
			Label:       "Action required",
			Title:       "重置您的密码",
			Intro:       "我们收到了密码重置请求。请确认是您本人操作后再继续。",
			ActionText:  "Reset password",
			AccentColor: "#0f766e",
		}
	case strings.Contains(subject, "额度") || strings.Contains(content, "额度") || strings.Contains(content, "充值"):
		return emailTemplateKind{
			Label:       "Balance alert",
			Title:       subject,
			Intro:       "您的账户额度已接近预警线。请及时处理，避免影响 API 调用。",
			ActionText:  "Open billing",
			AccentColor: "#b45309",
		}
	case strings.Contains(subject, "通道") || strings.Contains(content, "通道") || strings.Contains(content, "渠道"):
		return emailTemplateKind{
			Label:       "Operations update",
			Title:       subject,
			Intro:       "系统检测到通道状态变化，请查看下方详情。",
			AccentColor: "#7c3aed",
		}
	default:
		return emailTemplateKind{
			Label:       "System notice",
			Title:       subject,
			Intro:       "这是一条来自系统的通知，请查看下方详情。",
			AccentColor: "#334155",
		}
	}
}

func renderEmailTemplate(subject string, content string) string {
	kind := emailKind(subject, content)
	body := strings.TrimSpace(content)
	if body == "" {
		body = html.EscapeString(subject)
	}

	codeBlock := ""
	if match := emailCodePattern.FindStringSubmatch(body); len(match) == 2 {
		code := html.EscapeString(match[1])
		codeBlock = fmt.Sprintf(`
			<tr>
				<td style="padding:22px 0 6px;">
					<div style="font-family:ui-monospace,SFMono-Regular,Consolas,'Liberation Mono',monospace;font-size:32px;line-height:40px;letter-spacing:8px;font-weight:700;color:#0f172a;background:#f8fafc;border:1px solid #dbe4ef;border-radius:14px;text-align:center;padding:18px 16px;">%s</div>
				</td>
			</tr>`, code)
	}

	actionBlock := ""
	if kind.ActionText != "" {
		if match := emailLinkPattern.FindStringSubmatch(body); len(match) == 2 {
			link := html.EscapeString(match[1])
			actionBlock = fmt.Sprintf(`
			<tr>
				<td style="padding:18px 0 4px;">
					<a href="%s" style="display:inline-block;background:%s;color:#ffffff;text-decoration:none;font-size:15px;line-height:20px;font-weight:700;border-radius:10px;padding:13px 18px;">%s</a>
				</td>
			</tr>`, link, kind.AccentColor, html.EscapeString(kind.ActionText))
		}
	}

	return fmt.Sprintf(`<!doctype html>
<html>
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
</head>
<body style="margin:0;padding:0;background:#eef2f7;color:#0f172a;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,'Helvetica Neue',Arial,sans-serif;">
	<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="width:100%%;background:#eef2f7;padding:28px 12px;">
		<tr>
			<td align="center">
				<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="max-width:640px;background:#ffffff;border:1px solid #dbe4ef;border-radius:18px;overflow:hidden;box-shadow:0 18px 45px rgba(15,23,42,0.08);">
					<tr>
						<td style="padding:0;background:%s;height:6px;line-height:6px;font-size:1px;">&nbsp;</td>
					</tr>
					<tr>
						<td style="padding:30px 32px 18px;">
							<div style="font-size:13px;line-height:18px;font-weight:700;color:%s;text-transform:uppercase;letter-spacing:.06em;">%s</div>
							<h1 style="margin:10px 0 8px;font-size:26px;line-height:34px;font-weight:750;color:#0f172a;">%s</h1>
							<p style="margin:0;color:#475569;font-size:15px;line-height:24px;">%s</p>
						</td>
					</tr>
					<tr>
						<td style="padding:0 32px 30px;">
							<table role="presentation" width="100%%" cellspacing="0" cellpadding="0" border="0" style="background:#ffffff;border-collapse:collapse;">
								%s
								%s
								<tr>
									<td style="padding:18px 0 0;">
										<div style="border:1px solid #e2e8f0;border-radius:14px;background:#f8fafc;padding:18px 20px;color:#334155;font-size:15px;line-height:25px;">%s</div>
									</td>
								</tr>
							</table>
						</td>
					</tr>
					<tr>
						<td style="padding:18px 32px 28px;background:#f8fafc;border-top:1px solid #e2e8f0;color:#64748b;font-size:13px;line-height:21px;">
							<div style="font-weight:700;color:#334155;margin-bottom:4px;">%s</div>
							<div>如果您没有发起相关操作，可以忽略本邮件。为保障账户安全，请勿将验证码、链接或通知内容转发给他人。</div>
						</td>
					</tr>
				</table>
			</td>
		</tr>
	</table>
</body>
</html>`,
		html.EscapeString(subject),
		kind.AccentColor,
		kind.AccentColor,
		html.EscapeString(kind.Label),
		html.EscapeString(kind.Title),
		html.EscapeString(kind.Intro),
		codeBlock,
		actionBlock,
		body,
		html.EscapeString(SystemName),
	)
}
