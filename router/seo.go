package router

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

const (
	defaultSEODescription = "A unified AI API gateway for OpenAI, Claude, Gemini, DeepSeek and private model providers, with centralized keys, billing and routing."
	seoBlockStart         = "<!--seo-meta-start-->"
	seoBlockEnd           = "<!--seo-meta-end-->"
)

var seoBlockPattern = regexp.MustCompile(`(?s)<!--seo-meta-start-->.*?<!--seo-meta-end-->`)

type seoPage struct {
	Status      int
	Title       string
	Description string
	Canonical   string
	Robots      string
	Image       string
	Type        string
	JSONLD      string
}

func renderSEOMetadata(c *gin.Context, page string) string {
	metadata := resolveSEOPage(c)
	block := buildSEOBlock(metadata)
	if seoBlockPattern.MatchString(page) {
		return seoBlockPattern.ReplaceAllStringFunc(page, func(string) string {
			return block
		})
	}

	// Keep compatibility with older/custom frontend templates without SEO markers.
	title := html.EscapeString(metadata.Title)
	for _, placeholder := range []string{"New API", "loading"} {
		page = strings.ReplaceAll(page, "<title>"+placeholder+"</title>", "<title>"+title+"</title>")
		page = strings.ReplaceAll(page, `name="title" content="`+placeholder+`"`, `name="title" content="`+title+`"`)
	}
	return strings.Replace(page, "</head>", block+"\n  </head>", 1)
}

func buildSEOBlock(page seoPage) string {
	title := html.EscapeString(page.Title)
	description := html.EscapeString(page.Description)
	canonical := html.EscapeString(page.Canonical)
	robots := html.EscapeString(page.Robots)
	image := html.EscapeString(page.Image)
	pageType := html.EscapeString(page.Type)

	var builder strings.Builder
	builder.WriteString(seoBlockStart)
	builder.WriteString("\n    <title>")
	builder.WriteString(title)
	builder.WriteString("</title>\n")
	builder.WriteString(`    <meta name="title" content="` + title + `" />` + "\n")
	builder.WriteString(`    <meta name="description" content="` + description + `" />` + "\n")
	builder.WriteString(`    <meta name="robots" content="` + robots + `" />` + "\n")
	if canonical != "" {
		builder.WriteString(`    <link rel="canonical" href="` + canonical + `" />` + "\n")
	}
	builder.WriteString(`    <meta property="og:type" content="` + pageType + `" />` + "\n")
	builder.WriteString(`    <meta property="og:title" content="` + title + `" />` + "\n")
	builder.WriteString(`    <meta property="og:description" content="` + description + `" />` + "\n")
	if canonical != "" {
		builder.WriteString(`    <meta property="og:url" content="` + canonical + `" />` + "\n")
	}
	if image != "" {
		builder.WriteString(`    <meta property="og:image" content="` + image + `" />` + "\n")
	}
	builder.WriteString(`    <meta name="twitter:card" content="summary_large_image" />` + "\n")
	builder.WriteString(`    <meta name="twitter:title" content="` + title + `" />` + "\n")
	builder.WriteString(`    <meta name="twitter:description" content="` + description + `" />` + "\n")
	if image != "" {
		builder.WriteString(`    <meta name="twitter:image" content="` + image + `" />` + "\n")
	}
	if page.JSONLD != "" {
		builder.WriteString(`    <script type="application/ld+json">` + page.JSONLD + `</script>` + "\n")
	}
	builder.WriteString("    " + seoBlockEnd)
	return builder.String()
}

func resolveSEOPage(c *gin.Context) seoPage {
	siteName := indexPageTitle(c)
	origin := requestOrigin(c)
	path := "/"
	if c != nil && c.Request != nil && c.Request.URL != nil {
		path = normalizeSEOPath(c.Request.URL.EscapedPath())
	}

	page := seoPage{
		Status:      http.StatusOK,
		Title:       siteName,
		Description: defaultSEODescription,
		Canonical:   absoluteSiteURL(origin, path),
		Robots:      "noindex, nofollow",
		Image:       absoluteSiteURL(origin, indexPageLogo(c)),
		Type:        "website",
	}

	switch path {
	case "/":
		page.Title = "Unified AI API Gateway | " + siteName
		page.Robots = "index, follow"
		page.JSONLD = websiteJSONLD(siteName, origin, page.Image)
	case "/pricing":
		if seoModuleIsPublic(c, "pricing") {
			page.Title = "Model Pricing | " + siteName
			page.Description = "Compare supported AI models, capabilities and API pricing on " + siteName + "."
			page.Robots = "index, follow"
		}
	case "/subscription":
		if seoModuleIsPublic(c, "subscription") {
			page.Title = "Subscriptions | " + siteName
			page.Description = "Explore API subscription plans available on " + siteName + "."
			page.Robots = "index, follow"
		}
	case "/rankings":
		if seoModuleIsPublic(c, "rankings") {
			page.Title = "Rankings | " + siteName
			page.Description = "View current API usage rankings on " + siteName + "."
			page.Robots = "index, follow"
		}
	case "/about":
		page.Title = "About | " + siteName
		page.Description = "Learn more about " + siteName + " and its unified AI API gateway."
		page.Robots = "index, follow"
	case "/user-agreement":
		page.Title = "User Agreement | " + siteName
		page.Description = "Read the user agreement for " + siteName + "."
		page.Robots = "index, follow"
	case "/privacy-policy":
		page.Title = "Privacy Policy | " + siteName
		page.Description = "Read the privacy policy for " + siteName + "."
		page.Robots = "index, follow"
	default:
		if modelName, description, ok := seoPricingModel(c, path); ok {
			page.Title = modelName + " Pricing | " + siteName
			page.Description = description
			page.Robots = "index, follow"
			page.JSONLD = breadcrumbJSONLD(origin, modelName)
		} else if !isKnownFrontendPath(path) {
			page.Status = http.StatusNotFound
			page.Title = "Page Not Found | " + siteName
			page.Canonical = ""
		}
	}

	return page
}

func indexPageTitle(c *gin.Context) string {
	title := strings.TrimSpace(common.SystemName)
	if agentCtx := seoAgentContext(c); agentCtx != nil {
		if branding, ok := agentservice.ParseBranding(agentCtx.Branding); ok && branding.SiteName != "" {
			title = branding.SiteName
		}
	}
	if title == "" {
		return "New API"
	}
	return title
}

func indexPageLogo(c *gin.Context) string {
	logo := strings.TrimSpace(common.Logo)
	if agentCtx := seoAgentContext(c); agentCtx != nil {
		if branding, ok := agentservice.ParseBranding(agentCtx.Branding); ok && branding.Logo != "" {
			logo = branding.Logo
		}
	}
	if logo == "" {
		return "/logo.png"
	}
	return logo
}

func seoAgentContext(c *gin.Context) *types.AgentContext {
	if c == nil {
		return nil
	}
	agentCtx, _ := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext)
	return agentCtx
}

func seoPricingModel(c *gin.Context, path string) (string, string, bool) {
	if common.GetTheme() != "default" || !seoModuleIsPublic(c, "pricing") || !strings.HasPrefix(path, "/pricing/") {
		return "", "", false
	}
	encodedName := strings.TrimPrefix(path, "/pricing/")
	if encodedName == "" || strings.Contains(encodedName, "/") {
		return "", "", false
	}
	modelName, err := url.PathUnescape(encodedName)
	if err != nil {
		return "", "", false
	}
	for _, pricing := range seoPublicPricing(c) {
		matchesAlias := false
		for _, alias := range pricing.AliasModels {
			if alias == modelName {
				matchesAlias = true
				break
			}
		}
		if pricing.ModelName != modelName && !matchesAlias {
			continue
		}
		description := strings.TrimSpace(pricing.Description)
		if description == "" {
			description = fmt.Sprintf("View %s capabilities and API pricing on %s.", modelName, indexPageTitle(c))
		}
		return modelName, description, true
	}
	return "", "", false
}

func seoPublicPricing(c *gin.Context) []model.Pricing {
	usableGroups := service.GetUserUsableGroups("")
	if agentCtx := seoAgentContext(c); agentCtx != nil {
		usableGroups = make(map[string]string)
		for _, group := range agentservice.VisibleGroupsForUser(agentCtx, "") {
			if group.SystemGroupName != "" {
				usableGroups[group.SystemGroupName] = group.Description
			}
		}
		if len(usableGroups) == 0 {
			return []model.Pricing{}
		}
	}

	pricing := model.GetPricing()
	filtered := make([]model.Pricing, 0, len(pricing))
	for _, item := range pricing {
		if common.StringsContains(item.EnableGroup, "all") {
			filtered = append(filtered, item)
			continue
		}
		for _, group := range item.EnableGroup {
			if _, ok := usableGroups[group]; ok {
				filtered = append(filtered, item)
				break
			}
		}
	}
	return filtered
}

func seoModuleIsPublic(c *gin.Context, module string) bool {
	raw := ""
	common.OptionMapRWMutex.RLock()
	raw = common.OptionMap["HeaderNavModules"]
	common.OptionMapRWMutex.RUnlock()
	if agentCtx := seoAgentContext(c); agentCtx != nil {
		if branding, ok := agentservice.ParseBranding(agentCtx.Branding); ok && branding.HeaderNavModules != "" {
			raw = branding.HeaderNavModules
		}
	}

	enabled, requireAuth := true, false
	if strings.TrimSpace(raw) == "" {
		return true
	}
	var modules map[string]any
	if err := common.UnmarshalJsonStr(raw, &modules); err != nil {
		return true
	}
	switch value := modules[module].(type) {
	case bool:
		enabled = value
	case string:
		enabled = seoBool(value, enabled)
	case map[string]any:
		enabled = seoBool(value["enabled"], enabled)
		requireAuth = seoBool(value["requireAuth"], requireAuth)
	}
	return enabled && !requireAuth
}

func seoBool(value any, fallback bool) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1":
			return true
		case "false", "0":
			return false
		}
	case float64:
		if typed == 1 {
			return true
		}
		if typed == 0 {
			return false
		}
	}
	return fallback
}

func normalizeSEOPath(path string) string {
	if path == "" || path == "/index.html" {
		return "/"
	}
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}
	return path
}

func isKnownFrontendPath(path string) bool {
	known := map[string]struct{}{
		"/401": {}, "/403": {}, "/404": {}, "/500": {}, "/503": {},
		"/about": {}, "/agent-management": {}, "/agents": {}, "/billing": {},
		"/channels": {}, "/chat2link": {}, "/console": {}, "/console/channel": {},
		"/console/deployment": {}, "/console/log": {}, "/console/midjourney": {},
		"/console/models": {}, "/console/personal": {}, "/console/playground": {},
		"/console/redemption": {}, "/console/setting": {}, "/console/subscription": {},
		"/console/task": {}, "/console/token": {}, "/console/topup": {}, "/console/user": {},
		"/dashboard": {}, "/forbidden": {}, "/forgot-password": {}, "/invite": {},
		"/keys": {}, "/login": {}, "/model-billing": {}, "/model-monitor": {},
		"/model-profit": {}, "/models": {}, "/oauth": {}, "/otp": {},
		"/playground": {}, "/pricing": {}, "/privacy-policy": {}, "/profile": {},
		"/provider": {}, "/rankings": {}, "/redemption-codes": {}, "/register": {},
		"/reset": {}, "/setup": {}, "/sign-in": {}, "/sign-up": {},
		"/subscription": {}, "/subscriptions": {}, "/suppliers": {},
		"/system-settings": {}, "/system-tasks": {}, "/usage-logs": {},
		"/user-agreement": {}, "/user-subscription": {}, "/user/reset": {},
		"/users": {}, "/wallet": {},
	}
	if _, ok := known[path]; ok {
		return true
	}
	for _, prefix := range []string{
		"/billing/", "/chat/", "/console/chat/", "/dashboard/", "/errors/",
		"/models/", "/oauth/", "/system-settings/", "/usage-logs/",
	} {
		if strings.HasPrefix(path, prefix) && strings.TrimPrefix(path, prefix) != "" {
			return true
		}
	}
	return false
}

func requestOrigin(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return strings.TrimRight(system_setting.ServerAddress, "/")
	}
	host := c.Request.Host
	if agentCtx := seoAgentContext(c); agentCtx != nil && agentCtx.Domain != "" {
		host = agentCtx.Domain
	}
	if host == "" {
		return strings.TrimRight(system_setting.ServerAddress, "/")
	}
	scheme := "http"
	if forwarded := c.GetHeader("X-Forwarded-Proto"); forwarded != "" {
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
	} else if c.Request.TLS != nil {
		scheme = "https"
	}
	if scheme != "http" && scheme != "https" {
		scheme = "https"
	}
	return (&url.URL{Scheme: scheme, Host: host}).String()
}

func absoluteSiteURL(origin string, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return parsed.String()
	}
	if origin == "" {
		return path
	}
	return strings.TrimRight(origin, "/") + "/" + strings.TrimLeft(path, "/")
}

func websiteJSONLD(siteName string, origin string, image string) string {
	items := []map[string]any{
		{"@type": "WebSite", "name": siteName, "url": absoluteSiteURL(origin, "/")},
		{"@type": "Organization", "name": siteName, "url": absoluteSiteURL(origin, "/"), "logo": image},
	}
	payload := map[string]any{"@context": "https://schema.org", "@graph": items}
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func breadcrumbJSONLD(origin string, modelName string) string {
	payload := map[string]any{
		"@context": "https://schema.org",
		"@type":    "BreadcrumbList",
		"itemListElement": []map[string]any{
			{"@type": "ListItem", "position": 1, "name": "Model Pricing", "item": absoluteSiteURL(origin, "/pricing")},
			{"@type": "ListItem", "position": 2, "name": modelName, "item": absoluteSiteURL(origin, "/pricing/"+url.PathEscape(modelName))},
		},
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func serveRobotsTXT(c *gin.Context) {
	c.Set(middleware.RouteTagKey, "web")
	c.Header("Cache-Control", "public, max-age=3600")
	origin := requestOrigin(c)
	body := "User-agent: *\nAllow: /\n" +
		"Disallow: /api/\nDisallow: /v1/\nDisallow: /console/\nDisallow: /dashboard/\n" +
		"Disallow: /system-settings/\nDisallow: /oauth/\nDisallow: /sign-in\nDisallow: /sign-up\n" +
		"Sitemap: " + absoluteSiteURL(origin, "/sitemap.xml") + "\n"
	c.String(http.StatusOK, body)
}

func serveSitemapXML(c *gin.Context) {
	c.Set(middleware.RouteTagKey, "web")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Type", "application/xml; charset=utf-8")
	origin := requestOrigin(c)
	paths := []string{"/", "/about"}
	if seoModuleIsPublic(c, "subscription") {
		paths = append(paths, "/subscription")
	}
	if seoModuleIsPublic(c, "rankings") {
		paths = append(paths, "/rankings")
	}
	if strings.TrimSpace(system_setting.GetLegalSettings().UserAgreement) != "" {
		paths = append(paths, "/user-agreement")
	}
	if strings.TrimSpace(system_setting.GetLegalSettings().PrivacyPolicy) != "" {
		paths = append(paths, "/privacy-policy")
	}
	if seoModuleIsPublic(c, "pricing") {
		paths = append(paths, "/pricing")
		if common.GetTheme() == "default" {
			pricing := seoPublicPricing(c)
			for index, item := range pricing {
				if index >= 49000 {
					break
				}
				paths = append(paths, "/pricing/"+url.PathEscape(item.ModelName))
			}
		}
	}
	sort.Strings(paths)

	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	builder.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, path := range paths {
		builder.WriteString("  <url><loc>")
		builder.WriteString(html.EscapeString(absoluteSiteURL(origin, path)))
		builder.WriteString("</loc></url>\n")
	}
	builder.WriteString("</urlset>\n")
	c.String(http.StatusOK, builder.String())
}
