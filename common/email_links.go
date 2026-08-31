package common

import (
	"net/url"
	"regexp"
	"strings"
	"sync"
)

var emailSiteLinkPatternCache sync.Map

// RewriteEmailSiteLinks replaces absolute links that point at sourceSiteURL
// with targetSiteURL while preserving each link's path, query, and fragment.
// URLs for external sites are intentionally left unchanged.
func RewriteEmailSiteLinks(content string, sourceSiteURL string, targetSiteURL string) string {
	sourceOrigin, sourceHost, ok := emailSiteOrigin(sourceSiteURL)
	if !ok {
		return content
	}
	targetOrigin, targetHost, ok := emailSiteOrigin(targetSiteURL)
	if !ok || strings.EqualFold(sourceOrigin, targetOrigin) || strings.EqualFold(sourceHost, targetHost) {
		return content
	}

	pattern := emailSiteLinkPattern(sourceHost)
	return pattern.ReplaceAllString(content, targetOrigin+`${1}`)
}

func emailSiteLinkPattern(host string) *regexp.Regexp {
	key := strings.ToLower(host)
	if cached, ok := emailSiteLinkPatternCache.Load(key); ok {
		return cached.(*regexp.Regexp)
	}
	pattern := regexp.MustCompile(`(?i)https?://` + regexp.QuoteMeta(host) + `($|[/?#\s'"<>()[\]])`)
	actual, _ := emailSiteLinkPatternCache.LoadOrStore(key, pattern)
	return actual.(*regexp.Regexp)
}

func emailSiteOrigin(rawURL string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", "", false
	}
	return strings.ToLower(parsed.Scheme) + "://" + parsed.Host, parsed.Host, true
}
