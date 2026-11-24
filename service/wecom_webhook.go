package service

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"golang.org/x/net/html"
)

const (
	weComMarkdownMaxBytes = 4096
	weComReceiptMaxBytes  = 16 << 10
)

func resolveWebhookFormat(webhookURL string, format string) (string, error) {
	switch format {
	case dto.WebhookFormatGeneric, dto.WebhookFormatWeCom:
		return format, nil
	case "":
		// Existing installations did not persist a format choice.
		if isWeComWebhookURL(webhookURL) {
			return dto.WebhookFormatWeCom, nil
		}
		return dto.WebhookFormatGeneric, nil
	default:
		return "", fmt.Errorf("invalid webhook format: %q", format)
	}
}

func isWeComWebhookURL(webhookURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(webhookURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "qyapi.weixin.qq.com") &&
		parsed.Path == "/cgi-bin/webhook/send" &&
		strings.TrimSpace(parsed.Query().Get("key")) != ""
}

type weComMarkdownPayload struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Content string `json:"content"`
	} `json:"markdown"`
}

func marshalWeComMarkdown(content string) ([]byte, error) {
	payload := weComMarkdownPayload{MsgType: "markdown"}
	payload.Markdown.Content = truncateUTF8ByBytes(content, weComMarkdownMaxBytes)
	return common.Marshal(payload)
}

// Notifications reuse email templates. Convert their HTML formatting and links
// to readable robot messages while preserving plain-text error details.
func weComNotificationMarkdown(content string) string {
	var result strings.Builder
	tokenizer := html.NewTokenizer(strings.NewReader(content))
	linkURL := ""
	skipTag := ""
	for {
		tokenType := tokenizer.Next()
		if tokenType == html.ErrorToken {
			return strings.TrimSpace(result.String())
		}
		raw := string(tokenizer.Raw())
		token := tokenizer.Token()
		if skipTag != "" {
			if tokenType == html.EndTagToken && token.Data == skipTag {
				skipTag = ""
			}
			continue
		}
		switch tokenType {
		case html.TextToken:
			result.WriteString(token.Data)
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			switch token.Data {
			case "script", "style":
				if tokenType == html.StartTagToken {
					skipTag = token.Data
				}
			case "br", "p", "div", "ul", "ol", "h1", "h2", "h3", "blockquote", "pre":
				result.WriteByte('\n')
			case "li":
				result.WriteByte('\n')
				if tokenType == html.StartTagToken {
					result.WriteString("- ")
				}
			case "a":
				if tokenType == html.StartTagToken {
					for _, attr := range token.Attr {
						if attr.Key == "href" {
							parsed, err := url.Parse(attr.Val)
							if err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") {
								linkURL = strings.NewReplacer("(", "%28", ")", "%29").Replace(parsed.String())
								result.WriteByte('[')
							}
							break
						}
					}
				} else if tokenType == html.EndTagToken && linkURL != "" {
					result.WriteString("](" + linkURL + ")")
					linkURL = ""
				}
			case "strong", "b", "em", "i", "span", "font", "code":
				// Keep the text without email-only formatting tags.
			default:
				result.WriteString(raw)
			}
		}
	}
}

func truncateUTF8ByBytes(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value
	}
	suffix := "\n...（内容已截断）"
	limit := maxBytes - len(suffix)
	if limit <= 0 {
		return ""
	}
	truncated := value[:limit]
	for !utf8.ValidString(truncated) && len(truncated) > 0 {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated + suffix
}

func validateWeComWebhookResponse(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, weComReceiptMaxBytes+1))
	if err != nil {
		return fmt.Errorf("failed to read wecom webhook response: %w", err)
	}
	if len(body) > weComReceiptMaxBytes {
		return fmt.Errorf("wecom webhook response exceeds %d bytes", weComReceiptMaxBytes)
	}
	var result struct {
		ErrCode *int   `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := common.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to decode wecom webhook response: %w", err)
	}
	if result.ErrCode == nil {
		return fmt.Errorf("wecom webhook response missing errcode")
	}
	if *result.ErrCode != 0 {
		return fmt.Errorf("wecom webhook request failed: errcode=%d errmsg=%s", *result.ErrCode, result.ErrMsg)
	}
	return nil
}
