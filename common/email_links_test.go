package common

import (
	"strings"
	"testing"
)

func TestRewriteEmailSiteLinksReplacesOnlyMainSiteOrigin(t *testing.T) {
	content := `<p><a href="https://modelsell.com/user/reset?token=one">重置密码</a></p>` +
		`<p>https://modelsell.com/wallet</p>` +
		`<p><a href="https://docs.example.com/modelsell.com">文档</a></p>` +
		`<p><a href="https://modelsell.com.evil.example/phishing">外部链接</a></p>`

	rewritten := RewriteEmailSiteLinks(content, "https://modelsell.com/", "https://agent.example.com/")

	if !strings.Contains(rewritten, "https://agent.example.com/user/reset?token=one") {
		t.Fatalf("expected reset link to use agent site, got %s", rewritten)
	}
	if !strings.Contains(rewritten, "https://agent.example.com/wallet") {
		t.Fatalf("expected visible wallet URL to use agent site, got %s", rewritten)
	}
	if !strings.Contains(rewritten, "https://docs.example.com/modelsell.com") {
		t.Fatalf("expected external documentation link to stay unchanged, got %s", rewritten)
	}
	if !strings.Contains(rewritten, "https://modelsell.com.evil.example/phishing") {
		t.Fatalf("expected suffix-attack host to stay unchanged, got %s", rewritten)
	}
}

func TestRewriteEmailSiteLinksAcceptsHTTPSourceLinksAndTargetPort(t *testing.T) {
	content := `<a href='http://modelsell.com/console/topup'>Top up</a>`

	rewritten := RewriteEmailSiteLinks(content, "https://modelsell.com", "http://agent.local:3000/base")

	if rewritten != `<a href='http://agent.local:3000/console/topup'>Top up</a>` {
		t.Fatalf("unexpected rewritten content: %s", rewritten)
	}
}

func TestRewriteEmailSiteLinksIgnoresInvalidOrSameSiteTargets(t *testing.T) {
	content := `<a href="https://modelsell.com/wallet">Wallet</a>`

	if got := RewriteEmailSiteLinks(content, "not-a-url", "https://agent.example.com"); got != content {
		t.Fatalf("invalid source should not change content: %s", got)
	}
	if got := RewriteEmailSiteLinks(content, "https://modelsell.com", "https://modelsell.com/other"); got != content {
		t.Fatalf("same site should not change content: %s", got)
	}
}
