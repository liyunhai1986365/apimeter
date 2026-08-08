package main

import (
	"strings"
	"testing"
)

func TestInjectGoogleAnalyticsMarksServerLoaderForFrontendReuse(t *testing.T) {
	originalIndexPage := indexPage
	originalClassicIndexPage := classicIndexPage
	t.Cleanup(func() {
		indexPage = originalIndexPage
		classicIndexPage = originalClassicIndexPage
	})

	t.Setenv("GOOGLE_ANALYTICS_ID", "G-6B94BX72EW")
	indexPage = []byte("<!--Google Analytics-->\n")
	classicIndexPage = []byte("<!--Google Analytics-->\n")

	InjectGoogleAnalytics()

	for _, page := range [][]byte{indexPage, classicIndexPage} {
		html := string(page)
		if !strings.Contains(html, `id="google-analytics-script"`) {
			t.Fatalf("server loader is missing the reusable script id: %s", html)
		}
		if !strings.Contains(html, `data-measurement-id="G-6B94BX72EW"`) {
			t.Fatalf("server loader is missing its measurement id: %s", html)
		}
	}
}
