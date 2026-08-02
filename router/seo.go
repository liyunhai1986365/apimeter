package router

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

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
	defaultSEODescription  = "A unified AI API gateway for OpenAI, Claude, Gemini, DeepSeek and private model providers, with centralized keys, billing and routing."
	seoBlockStart          = "<!--seo-meta-start-->"
	seoBlockEnd            = "<!--seo-meta-end-->"
	seoShellBootStyle      = `<style data-seo-shell-boot="true">html[data-seo-client="pending"] [data-seo-shell="true"]{display:none!important}</style>`
	seoShellBootScript     = `<script data-seo-shell-boot="true">document.documentElement.dataset.seoClient="pending";window.setTimeout(function(){delete document.documentElement.dataset.seoClient},8000)</script>`
	seoModelDirectoryLimit = 500
)

var seoBlockPattern = regexp.MustCompile(`(?s)<!--seo-meta-start-->.*?<!--seo-meta-end-->`)
var seoRootPattern = regexp.MustCompile(`<div\b[^>]*[[:space:]]id=["']root["'][^>]*>\s*</div>`)

type seoPage struct {
	Status      int
	Path        string
	Title       string
	Description string
	Canonical   string
	Robots      string
	Image       string
	Type        string
	JSONLD      string
	Model       *model.Pricing
}

func renderSEOMetadata(c *gin.Context, page string) string {
	metadata := resolveSEOPage(c)
	block := buildSEOBlock(metadata)
	if seoBlockPattern.MatchString(page) {
		page = seoBlockPattern.ReplaceAllStringFunc(page, func(string) string {
			return block
		})
	} else {
		// Keep compatibility with older/custom frontend templates without SEO markers.
		title := html.EscapeString(metadata.Title)
		for _, placeholder := range []string{"New API", "loading"} {
			page = strings.ReplaceAll(page, "<title>"+placeholder+"</title>", "<title>"+title+"</title>")
			page = strings.ReplaceAll(page, `name="title" content="`+placeholder+`"`, `name="title" content="`+title+`"`)
		}
		page = strings.Replace(page, "</head>", block+"\n  </head>", 1)
	}
	return renderSEOShell(c, page, metadata)
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
	builder.WriteString(`    <meta property="og:site_name" content="` + html.EscapeString(seoSiteName(page.Title)) + `" />` + "\n")
	builder.WriteString(`    <meta property="og:title" content="` + title + `" />` + "\n")
	builder.WriteString(`    <meta property="og:description" content="` + description + `" />` + "\n")
	if canonical != "" {
		builder.WriteString(`    <meta property="og:url" content="` + canonical + `" />` + "\n")
	}
	if image != "" {
		builder.WriteString(`    <meta property="og:image" content="` + image + `" />` + "\n")
	}
	builder.WriteString(`    <meta name="twitter:card" content="summary" />` + "\n")
	builder.WriteString(`    <meta name="twitter:title" content="` + title + `" />` + "\n")
	builder.WriteString(`    <meta name="twitter:description" content="` + description + `" />` + "\n")
	if image != "" {
		builder.WriteString(`    <meta name="twitter:image" content="` + image + `" />` + "\n")
	}
	if page.JSONLD != "" {
		builder.WriteString(`    <script type="application/ld+json" data-seo-jsonld="true">` + page.JSONLD + `</script>` + "\n")
	}
	if page.Robots == "index, follow" {
		// JS-capable browsers hide the crawlable fallback before first paint. If
		// the client bundle never starts, the inline timeout reveals it again.
		builder.WriteString("    " + seoShellBootStyle + "\n")
		builder.WriteString("    " + seoShellBootScript + "\n")
	}
	builder.WriteString("    " + seoBlockEnd)
	return builder.String()
}

func seoSiteName(title string) string {
	if _, siteName, ok := strings.Cut(title, " | "); ok && strings.TrimSpace(siteName) != "" {
		return strings.TrimSpace(siteName)
	}
	return strings.TrimSpace(title)
}

func renderSEOShell(c *gin.Context, page string, metadata seoPage) string {
	if metadata.Robots != "index, follow" || !seoRootPattern.MatchString(page) {
		return page
	}
	shell := buildSEOShell(c, metadata)
	return seoRootPattern.ReplaceAllStringFunc(page, func(root string) string {
		closingTag := strings.LastIndex(root, `</div>`)
		if closingTag == -1 {
			return root
		}
		return root[:closingTag] + shell + root[closingTag:]
	})
}

func buildSEOShell(c *gin.Context, metadata seoPage) string {
	siteName := indexPageTitle(c)
	var builder strings.Builder
	builder.WriteString(`<div data-seo-shell="true" style="min-height:100vh;background:#fff;color:#111827;font-family:system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif">`)
	builder.WriteString(`<header style="border-bottom:1px solid #e5e7eb"><nav aria-label="Primary" style="max-width:1120px;margin:0 auto;padding:18px 24px;display:flex;align-items:center;justify-content:space-between;gap:24px">`)
	builder.WriteString(`<a href="/" style="color:inherit;text-decoration:none;font-weight:700">` + html.EscapeString(siteName) + `</a>`)
	builder.WriteString(`<div style="display:flex;flex-wrap:wrap;gap:18px;font-size:14px">`)
	for _, link := range seoShellLinks(c) {
		builder.WriteString(`<a href="` + html.EscapeString(link.Path) + `" style="color:#4b5563;text-decoration:none">` + html.EscapeString(link.Label) + `</a>`)
	}
	builder.WriteString(`</div></nav></header>`)
	builder.WriteString(`<main style="max-width:1120px;margin:0 auto;padding:72px 24px 96px">`)
	builder.WriteString(`<p style="margin:0 0 12px;color:#4f46e5;font-weight:600">` + html.EscapeString(siteName) + `</p>`)
	builder.WriteString(`<h1 style="margin:0;max-width:900px;font-size:clamp(36px,7vw,72px);line-height:1.08;letter-spacing:-0.04em">` + html.EscapeString(seoShellHeading(metadata)) + `</h1>`)
	builder.WriteString(`<p style="margin:24px 0 0;max-width:760px;color:#4b5563;font-size:18px;line-height:1.7">` + html.EscapeString(metadata.Description) + `</p>`)
	builder.WriteString(seoShellPageContent(c, metadata))
	builder.WriteString(`</main></div>`)
	return builder.String()
}

type seoShellLink struct {
	Path  string
	Label string
}

type seoPriceItem struct {
	Label string
	Value string
}

func seoShellLinks(c *gin.Context) []seoShellLink {
	links := []seoShellLink{{Path: "/", Label: "Home"}}
	if seoModuleIsPublic(c, "pricing") {
		links = append(links, seoShellLink{Path: "/pricing", Label: "Model Pricing"})
	}
	if common.GetTheme() == "default" && seoModuleIsPublic(c, "subscription") {
		links = append(links, seoShellLink{Path: "/subscription", Label: "Subscriptions"})
	}
	if common.GetTheme() == "default" && seoModuleIsPublic(c, "rankings") {
		links = append(links, seoShellLink{Path: "/rankings", Label: "Rankings"})
	}
	links = append(links, seoShellLink{Path: "/about", Label: "About"})
	return links
}

func seoShellHeading(metadata seoPage) string {
	if metadata.Model != nil {
		return metadata.Model.ModelName + " API pricing & access"
	}
	switch metadata.Path {
	case "/":
		return "AI model APIs, pricing and access"
	case "/pricing":
		return "AI model API pricing and comparison"
	case "/subscription":
		return "Subscriptions"
	case "/rankings":
		return "Rankings"
	case "/about":
		return "About"
	case "/user-agreement":
		return "User Agreement"
	case "/privacy-policy":
		return "Privacy Policy"
	default:
		return metadata.Title
	}
}

func seoShellPageContent(c *gin.Context, metadata seoPage) string {
	if metadata.Path == "/" {
		return `<div style="margin-top:32px;display:flex;flex-wrap:wrap;gap:12px"><a href="/pricing" style="padding:12px 18px;border-radius:10px;background:#111827;color:#fff;text-decoration:none;font-weight:600">Explore AI model API pricing</a><a href="/sign-up" style="padding:12px 18px;border:1px solid #d1d5db;border-radius:10px;color:#111827;text-decoration:none;font-weight:600">Get started</a></div>` +
			`<section aria-labelledby="one-api" style="margin-top:56px;max-width:820px"><h2 id="one-api" style="font-size:28px">One API for leading AI model providers</h2><p style="color:#4b5563;font-size:16px;line-height:1.7">Use one gateway for OpenAI, Anthropic Claude, Google Gemini, DeepSeek and other model providers. Compare model capabilities and pricing before choosing the API that fits your application.</p><p><a href="/pricing" style="color:#4f46e5">Compare all available model APIs and prices</a></p></section>`
	}
	if metadata.Path == "/pricing" {
		pricing := seoPublicPricing(c)
		if len(pricing) == 0 {
			return ""
		}
		var builder strings.Builder
		builder.WriteString(`<section aria-labelledby="available-models" style="margin-top:48px"><h2 id="available-models" style="font-size:24px">Available models</h2><ul style="padding:0;display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px;list-style:none">`)
		for index, item := range pricing {
			if index >= seoModelDirectoryLimit {
				break
			}
			builder.WriteString(`<li><a href="/pricing/` + html.EscapeString(url.PathEscape(item.ModelName)) + `" style="display:block;padding:14px;border:1px solid #e5e7eb;border-radius:10px;color:#111827;text-decoration:none"><span style="display:block;font-family:ui-monospace,monospace;font-weight:600">` + html.EscapeString(item.ModelName) + ` API pricing</span>`)
			if price := seoPrimaryPrice(item); price != "" {
				builder.WriteString(`<span style="display:block;margin-top:6px;color:#6b7280;font-size:13px">` + html.EscapeString(price) + `</span>`)
			}
			builder.WriteString(`</a></li>`)
		}
		builder.WriteString(`</ul></section>`)
		return builder.String()
	}
	if metadata.Model != nil {
		var builder strings.Builder
		builder.WriteString(`<section aria-labelledby="model-api-access" style="margin-top:40px"><h2 id="model-api-access" style="font-size:24px">` + html.EscapeString(metadata.Model.ModelName) + ` API access</h2>`)
		if description := strings.TrimSpace(metadata.Model.Description); description != "" {
			builder.WriteString(`<p style="max-width:760px;color:#374151;line-height:1.7">` + html.EscapeString(description) + `</p>`)
		}
		builder.WriteString(`<p style="max-width:760px;color:#4b5563;line-height:1.7">Compare supported API endpoints and capabilities, then access ` + html.EscapeString(metadata.Model.ModelName) + ` through the unified API gateway.</p>`)
		builder.WriteString(`<dl style="margin-top:36px;display:flex;flex-wrap:wrap;gap:12px 32px">`)
		if metadata.Model.Category != "" {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Category</dt><dd style="margin:4px 0 0;font-weight:600">` + html.EscapeString(metadata.Model.Category) + `</dd></div>`)
		}
		if metadata.Model.ContextLength > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Context length</dt><dd style="margin:4px 0 0;font-weight:600">` + fmt.Sprintf("%d", metadata.Model.ContextLength) + ` tokens</dd></div>`)
		}
		if metadata.Model.MaxOutputTokens > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Maximum output</dt><dd style="margin:4px 0 0;font-weight:600">` + fmt.Sprintf("%d", metadata.Model.MaxOutputTokens) + ` tokens</dd></div>`)
		}
		if endpointNames := seoEndpointNames(metadata.Model.SupportedEndpointTypes); len(endpointNames) > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Supported APIs</dt><dd style="margin:4px 0 0;font-weight:600">` + html.EscapeString(strings.Join(endpointNames, ", ")) + `</dd></div>`)
		}
		if endpoints := seoEndpointPaths(metadata.Model.SupportedEndpointTypes); len(endpoints) > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">API endpoints</dt><dd style="margin:4px 0 0;font-weight:600;font-family:ui-monospace,monospace">` + html.EscapeString(strings.Join(endpoints, ", ")) + `</dd></div>`)
		}
		if len(metadata.Model.InputModalities) > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Input modalities</dt><dd style="margin:4px 0 0;font-weight:600">` + html.EscapeString(strings.Join(metadata.Model.InputModalities, ", ")) + `</dd></div>`)
		}
		if len(metadata.Model.OutputModalities) > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Output modalities</dt><dd style="margin:4px 0 0;font-weight:600">` + html.EscapeString(strings.Join(metadata.Model.OutputModalities, ", ")) + `</dd></div>`)
		}
		if len(metadata.Model.Capabilities) > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Capabilities</dt><dd style="margin:4px 0 0;font-weight:600">` + html.EscapeString(strings.Join(seoDisplayValues(metadata.Model.Capabilities), ", ")) + `</dd></div>`)
		}
		if len(metadata.Model.AliasModels) > 0 {
			builder.WriteString(`<div><dt style="color:#6b7280;font-size:13px">Also known as</dt><dd style="margin:4px 0 0;font-weight:600">` + html.EscapeString(strings.Join(metadata.Model.AliasModels, ", ")) + `</dd></div>`)
		}
		builder.WriteString(`</dl></section>`)
		priceItems := seoModelPriceItems(*metadata.Model)
		if len(priceItems) > 0 {
			builder.WriteString(`<section aria-labelledby="model-api-pricing" style="margin-top:48px"><h2 id="model-api-pricing" style="font-size:24px">` + html.EscapeString(metadata.Model.ModelName) + ` API pricing</h2><dl style="display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px">`)
			for _, item := range priceItems {
				builder.WriteString(`<div style="padding:16px;border:1px solid #e5e7eb;border-radius:10px"><dt style="color:#6b7280;font-size:13px">` + html.EscapeString(item.Label) + `</dt><dd style="margin:6px 0 0;font-weight:700">` + html.EscapeString(item.Value) + `</dd></div>`)
			}
			builder.WriteString(`</dl>`)
			if metadata.Model.BillingMode == "tiered_expr" {
				builder.WriteString(`<p style="color:#6b7280;font-size:14px;line-height:1.6">The final API price depends on the matched usage tier, request parameters and access group. Open the live pricing page for the current calculation.</p>`)
			} else {
				builder.WriteString(`<p style="color:#6b7280;font-size:14px;line-height:1.6">Base prices are shown in USD before group-specific adjustments. Open the live pricing page for current access-group prices.</p>`)
			}
			builder.WriteString(`</section>`)
		}
		related := seoRelatedModels(c, *metadata.Model, 8)
		if len(related) > 0 {
			builder.WriteString(`<section aria-labelledby="related-model-apis" style="margin-top:48px"><h2 id="related-model-apis" style="font-size:24px">Related model APIs</h2><ul style="padding-left:20px;line-height:1.9">`)
			for _, item := range related {
				builder.WriteString(`<li><a href="/pricing/` + html.EscapeString(url.PathEscape(item.ModelName)) + `" style="color:#4f46e5">` + html.EscapeString(item.ModelName) + ` API pricing</a></li>`)
			}
			builder.WriteString(`</ul></section>`)
		}
		builder.WriteString(`<p style="margin-top:36px"><a href="/pricing" style="color:#4f46e5">Back to AI model API pricing</a></p>`)
		return builder.String()
	}
	if metadata.Path == "/about" {
		return `<section aria-labelledby="unified-gateway" style="margin-top:48px;max-width:820px"><h2 id="unified-gateway" style="font-size:28px">A unified gateway for AI applications</h2><p style="color:#4b5563;font-size:16px;line-height:1.7">` + html.EscapeString(indexPageTitle(c)) + ` brings model discovery, compatible API access, centralized keys, usage visibility and billing controls into one platform.</p><p><a href="/pricing" style="color:#4f46e5">Explore supported AI models and API pricing</a></p></section>`
	}
	if metadata.Path == "/subscription" {
		return `<section aria-labelledby="api-plans" style="margin-top:48px;max-width:820px"><h2 id="api-plans" style="font-size:28px">API access plans</h2><p style="color:#4b5563;font-size:16px;line-height:1.7">Review available plans for predictable AI API access, then compare individual model prices and capabilities before subscribing.</p><p><a href="/pricing" style="color:#4f46e5">Compare model API pricing</a></p></section>`
	}
	if metadata.Path == "/rankings" {
		return `<section aria-labelledby="usage-rankings" style="margin-top:48px;max-width:820px"><h2 id="usage-rankings" style="font-size:28px">Model and provider activity</h2><p style="color:#4b5563;font-size:16px;line-height:1.7">Use current activity rankings alongside model capabilities and pricing to evaluate APIs for your application.</p><p><a href="/pricing" style="color:#4f46e5">Browse the AI model API directory</a></p></section>`
	}
	return ""
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
		Path:        path,
		Title:       siteName,
		Description: defaultSEODescription,
		Canonical:   absoluteSiteURL(origin, path),
		Robots:      "noindex, nofollow",
		Image:       absoluteSiteURL(origin, indexPageLogo(c)),
		Type:        "website",
	}

	switch path {
	case "/":
		page.Title = "AI Model APIs, Pricing & Access | " + siteName
		page.Description = "Compare and access AI model providers, API pricing, supported endpoints and capabilities on " + siteName + "."
		page.Robots = "index, follow"
		page.JSONLD = websiteJSONLD(siteName, origin, page.Image)
	case "/pricing":
		if seoModuleIsPublic(c, "pricing") {
			page.Title = "AI Model API Pricing & Comparison | " + siteName
			page.Description = "Compare AI model API pricing, supported endpoints, capabilities and access options on " + siteName + "."
			page.Robots = "index, follow"
			page.JSONLD = pricingCollectionJSONLD(origin, seoPublicPricing(c))
		}
	case "/subscription":
		if seoModuleIsPublic(c, "subscription") {
			page.Title = "Subscriptions | " + siteName
			page.Description = "Explore API subscription plans available on " + siteName + "."
			page.Robots = "index, follow"
			page.JSONLD = publicPageJSONLD(origin, page.Title, page.Description, path)
		}
	case "/rankings":
		if seoModuleIsPublic(c, "rankings") {
			page.Title = "Rankings | " + siteName
			page.Description = "View current API usage rankings on " + siteName + "."
			page.Robots = "index, follow"
			page.JSONLD = publicPageJSONLD(origin, page.Title, page.Description, path)
		}
	case "/about":
		page.Title = "About | " + siteName
		page.Description = "Learn more about " + siteName + " and its unified AI API gateway."
		page.Robots = "index, follow"
		page.JSONLD = publicPageJSONLD(origin, page.Title, page.Description, path)
	case "/user-agreement":
		page.Title = "User Agreement | " + siteName
		page.Description = "Read the user agreement for " + siteName + "."
		page.Robots = "index, follow"
		page.JSONLD = publicPageJSONLD(origin, page.Title, page.Description, path)
	case "/privacy-policy":
		page.Title = "Privacy Policy | " + siteName
		page.Description = "Read the privacy policy for " + siteName + "."
		page.Robots = "index, follow"
		page.JSONLD = publicPageJSONLD(origin, page.Title, page.Description, path)
	default:
		if pricing, ok := seoPricingModel(c, path); ok {
			page.Model = &pricing
			page.Title = pricing.ModelName + " API Pricing & Access | " + siteName
			page.Description = seoModelDescription(pricing, siteName)
			page.Canonical = absoluteSiteURL(origin, "/pricing/"+url.PathEscape(pricing.ModelName))
			page.Robots = "index, follow"
			page.JSONLD = modelPageJSONLD(origin, pricing, page.Description)
		} else if !isKnownFrontendPath(path) {
			page.Status = http.StatusNotFound
			page.Title = "Page Not Found | " + siteName
			page.Canonical = ""
		}
	}
	if page.Robots != "index, follow" {
		page.Canonical = ""
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

func seoPricingModel(c *gin.Context, path string) (model.Pricing, bool) {
	if common.GetTheme() != "default" || !seoModuleIsPublic(c, "pricing") || !strings.HasPrefix(path, "/pricing/") {
		return model.Pricing{}, false
	}
	encodedName := strings.TrimPrefix(path, "/pricing/")
	if encodedName == "" || strings.Contains(encodedName, "/") {
		return model.Pricing{}, false
	}
	modelName, err := url.PathUnescape(encodedName)
	if err != nil {
		return model.Pricing{}, false
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
		return pricing, true
	}
	return model.Pricing{}, false
}

func seoModelDescription(pricing model.Pricing, siteName string) string {
	price := seoPrimaryPrice(pricing)
	if price == "" {
		return fmt.Sprintf("Compare %s API pricing, supported endpoints, capabilities and access options on %s.", pricing.ModelName, siteName)
	}
	return fmt.Sprintf("%s API pricing: %s. Compare supported endpoints, capabilities and access options on %s.", pricing.ModelName, price, siteName)
}

func seoModelPriceItems(pricing model.Pricing) []seoPriceItem {
	if pricing.BillingMode == "tiered_expr" && strings.TrimSpace(pricing.BillingExpr) != "" {
		return []seoPriceItem{{Label: "Billing model", Value: "Dynamic usage-based pricing"}}
	}
	if pricing.QuotaType == 1 {
		return []seoPriceItem{{Label: "Base price", Value: seoUSDPrice(pricing.ModelPrice) + " per request"}}
	}
	items := []seoPriceItem{
		{Label: "Base input price", Value: seoUSDPrice(pricing.ModelRatio*2) + " per 1M tokens"},
		{Label: "Base output price", Value: seoUSDPrice(pricing.ModelRatio*2*pricing.CompletionRatio) + " per 1M tokens"},
	}
	return items
}

func seoPrimaryPrice(pricing model.Pricing) string {
	items := seoModelPriceItems(pricing)
	if len(items) == 0 {
		return ""
	}
	return items[0].Value
}

func seoUSDPrice(price float64) string {
	if price == 0 {
		return "$0 (free)"
	}
	return "$" + strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.8f", price), "0"), ".")
}

func seoDisplayValues(values []string) []string {
	display := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ReplaceAll(value, "_", " "))
		if value != "" {
			display = append(display, value)
		}
	}
	return display
}

func seoEndpointNames(endpointTypes []constant.EndpointType) []string {
	names := make([]string, 0, len(endpointTypes))
	seen := make(map[string]struct{}, len(endpointTypes))
	for _, endpointType := range endpointTypes {
		name := seoEndpointName(endpointType)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func seoEndpointPaths(endpointTypes []constant.EndpointType) []string {
	paths := make([]string, 0, len(endpointTypes))
	seen := make(map[string]struct{}, len(endpointTypes))
	for _, endpointType := range endpointTypes {
		info, ok := common.GetDefaultEndpointInfo(endpointType)
		path := strings.TrimSpace(info.Path)
		if !ok || path == "" {
			continue
		}
		endpoint := strings.TrimSpace(info.Method + " " + path)
		if _, ok := seen[endpoint]; ok {
			continue
		}
		seen[endpoint] = struct{}{}
		paths = append(paths, endpoint)
	}
	return paths
}

func seoEndpointName(endpointType constant.EndpointType) string {
	switch endpointType {
	case constant.EndpointTypeOpenAI:
		return "OpenAI Chat Completions API"
	case constant.EndpointTypeOpenAIResponse:
		return "OpenAI Responses API"
	case constant.EndpointTypeOpenAIResponseCompact:
		return "OpenAI Responses Compact API"
	case constant.EndpointTypeAnthropic:
		return "Anthropic Messages API"
	case constant.EndpointTypeGemini:
		return "Gemini API"
	case constant.EndpointTypeJinaRerank:
		return "Rerank API"
	case constant.EndpointTypeImageGeneration:
		return "Image Generation API"
	case constant.EndpointTypeOpenAIImageEdit:
		return "OpenAI Image Edit API"
	case constant.EndpointTypeGeminiImageGeneration:
		return "Gemini Image Generation API"
	case constant.EndpointTypeEmbeddings:
		return "Embeddings API"
	case constant.EndpointTypeOpenAIVideo:
		return "OpenAI Video API"
	case constant.EndpointTypeSeedance2NativeVideo:
		return "Seedance 2.0 Video API"
	default:
		return strings.TrimSpace(string(endpointType))
	}
}

func seoRelatedModels(c *gin.Context, current model.Pricing, limit int) []model.Pricing {
	if limit <= 0 || strings.TrimSpace(current.Category) == "" {
		return nil
	}
	related := make([]model.Pricing, 0, limit)
	for _, item := range seoPublicPricing(c) {
		if item.ModelName == current.ModelName || !strings.EqualFold(item.Category, current.Category) {
			continue
		}
		related = append(related, item)
		if len(related) == limit {
			break
		}
	}
	return related
}

func seoPublicPricing(c *gin.Context) []model.Pricing {
	if model.DB == nil {
		return []model.Pricing{}
	}
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
		"/invite-rewards": {}, "/keys": {}, "/login": {}, "/model-billing": {}, "/model-monitor": {},
		"/model-profit": {}, "/models": {}, "/oauth": {}, "/otp": {},
		"/playground": {}, "/pricing": {}, "/privacy-policy": {}, "/profile": {},
		"/provider": {}, "/rankings": {}, "/redemption-codes": {}, "/register": {},
		"/reset": {}, "/setup": {}, "/sign-in": {}, "/sign-up": {},
		"/subscription": {}, "/subscriptions": {}, "/suppliers": {},
		"/system-settings": {}, "/system-tasks": {}, "/usage-logs": {},
		"/user-agreement": {}, "/user-subscription": {}, "/user/reset": {},
		"/users": {}, "/wallet": {}, "/withdrawal-management": {},
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
	if agentCtx := seoAgentContext(c); agentCtx != nil && agentCtx.Domain != "" {
		return requestOriginForHost(c, agentCtx.Domain)
	}
	if c == nil || c.Request == nil {
		return ""
	}
	return requestOriginForHost(c, c.Request.Host)
}

func requestOriginForHost(c *gin.Context, host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	scheme := "http"
	if c != nil && c.Request != nil && c.GetHeader("X-Forwarded-Proto") != "" {
		forwarded := c.GetHeader("X-Forwarded-Proto")
		scheme = strings.ToLower(strings.TrimSpace(strings.Split(forwarded, ",")[0]))
	} else if c != nil && c.Request != nil && c.Request.TLS != nil {
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
	websiteID := absoluteSiteURL(origin, "/#website")
	organizationID := absoluteSiteURL(origin, "/#organization")
	items := []map[string]any{
		{"@type": "WebSite", "@id": websiteID, "name": siteName, "url": absoluteSiteURL(origin, "/"), "publisher": map[string]any{"@id": organizationID}},
		{"@type": "Organization", "@id": organizationID, "name": siteName, "url": absoluteSiteURL(origin, "/"), "logo": map[string]any{"@type": "ImageObject", "url": image}},
	}
	payload := map[string]any{"@context": "https://schema.org", "@graph": items}
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func modelPageJSONLD(origin string, pricing model.Pricing, description string) string {
	canonical := absoluteSiteURL(origin, "/pricing/"+url.PathEscape(pricing.ModelName))
	websiteID := absoluteSiteURL(origin, "/#website")
	organizationID := absoluteSiteURL(origin, "/#organization")
	serviceID := canonical + "#service"
	service := map[string]any{
		"@type":       "Service",
		"@id":         serviceID,
		"name":        pricing.ModelName + " API",
		"description": description,
		"provider":    map[string]any{"@id": organizationID},
	}
	if endpointNames := seoEndpointNames(pricing.SupportedEndpointTypes); len(endpointNames) > 0 {
		service["serviceType"] = strings.Join(endpointNames, ", ")
	}
	if channels := seoServiceChannels(origin, pricing.SupportedEndpointTypes); len(channels) > 0 {
		service["availableChannel"] = channels
	}
	if len(pricing.AliasModels) > 0 {
		service["alternateName"] = pricing.AliasModels
	}
	if len(pricing.Capabilities) > 0 {
		service["category"] = strings.Join(seoDisplayValues(pricing.Capabilities), ", ")
	}
	priceItems := seoModelPriceItems(pricing)
	if len(priceItems) > 0 {
		properties := make([]map[string]any, 0, len(priceItems))
		for _, item := range priceItems {
			properties = append(properties, map[string]any{
				"@type": "PropertyValue",
				"name":  item.Label,
				"value": item.Value,
			})
		}
		service["additionalProperty"] = properties
	}
	payload := map[string]any{
		"@context": "https://schema.org",
		"@graph": []map[string]any{
			{
				"@type":       "WebPage",
				"@id":         canonical + "#webpage",
				"name":        pricing.ModelName + " API Pricing & Access",
				"description": description,
				"url":         canonical,
				"isPartOf":    map[string]any{"@id": websiteID},
				"mainEntity":  map[string]any{"@id": serviceID},
			},
			service,
			{
				"@type": "BreadcrumbList",
				"@id":   canonical + "#breadcrumb",
				"itemListElement": []map[string]any{
					{"@type": "ListItem", "position": 1, "name": "AI Model API Pricing", "item": absoluteSiteURL(origin, "/pricing")},
					{"@type": "ListItem", "position": 2, "name": pricing.ModelName, "item": canonical},
				},
			},
		},
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func publicPageJSONLD(origin string, title string, description string, path string) string {
	canonical := absoluteSiteURL(origin, path)
	payload := map[string]any{
		"@context":    "https://schema.org",
		"@type":       "WebPage",
		"@id":         canonical + "#webpage",
		"name":        title,
		"description": description,
		"url":         canonical,
		"isPartOf":    map[string]any{"@id": absoluteSiteURL(origin, "/#website")},
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func seoServiceChannels(origin string, endpointTypes []constant.EndpointType) []map[string]any {
	channels := make([]map[string]any, 0, len(endpointTypes))
	seen := make(map[string]struct{}, len(endpointTypes))
	for _, endpointType := range endpointTypes {
		info, ok := common.GetDefaultEndpointInfo(endpointType)
		path := strings.TrimSpace(info.Path)
		if !ok || path == "" {
			continue
		}
		name := strings.TrimSpace(info.Method + " " + path)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		channels = append(channels, map[string]any{
			"@type":      "ServiceChannel",
			"name":       name,
			"serviceUrl": absoluteSiteURL(origin, path),
		})
	}
	return channels
}

func pricingCollectionJSONLD(origin string, pricing []model.Pricing) string {
	items := make([]map[string]any, 0, min(len(pricing), seoModelDirectoryLimit))
	for index, item := range pricing {
		if index >= seoModelDirectoryLimit {
			break
		}
		items = append(items, map[string]any{
			"@type":    "ListItem",
			"position": index + 1,
			"name":     item.ModelName + " API pricing",
			"url":      absoluteSiteURL(origin, "/pricing/"+url.PathEscape(item.ModelName)),
		})
	}
	payload := map[string]any{
		"@context":   "https://schema.org",
		"@type":      "CollectionPage",
		"@id":        absoluteSiteURL(origin, "/pricing#webpage"),
		"name":       "AI Model API Pricing & Comparison",
		"url":        absoluteSiteURL(origin, "/pricing"),
		"isPartOf":   map[string]any{"@id": absoluteSiteURL(origin, "/#website")},
		"mainEntity": map[string]any{"@type": "ItemList", "itemListElement": items},
	}
	data, err := common.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

type sitemapEntry struct {
	Location string
	LastMod  int64
}

func buildSitemapXML(origin string, entries []sitemapEntry) string {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Location < entries[j].Location
	})
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	builder.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, entry := range entries {
		builder.WriteString("  <url><loc>")
		builder.WriteString(html.EscapeString(absoluteSiteURL(origin, entry.Location)))
		builder.WriteString("</loc>")
		if entry.LastMod > 0 {
			builder.WriteString("<lastmod>")
			builder.WriteString(time.Unix(entry.LastMod, 0).UTC().Format(time.DateOnly))
			builder.WriteString("</lastmod>")
		}
		builder.WriteString("</url>\n")
	}
	builder.WriteString("</urlset>\n")
	return builder.String()
}

func serveRobotsTXT(c *gin.Context) {
	c.Set(middleware.RouteTagKey, "web")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Type", "text/plain; charset=utf-8")
	origin := requestOrigin(c)
	body := "User-agent: *\nAllow: /\n" +
		"Disallow: /api/\nDisallow: /v1/\n" +
		"Sitemap: " + absoluteSiteURL(origin, "/sitemap.xml") + "\n"
	c.String(http.StatusOK, body)
}

func serveSitemapXML(c *gin.Context) {
	c.Set(middleware.RouteTagKey, "web")
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Type", "application/xml; charset=utf-8")
	origin := requestOrigin(c)
	entries := []sitemapEntry{{Location: "/"}, {Location: "/about"}}
	if common.GetTheme() == "default" && seoModuleIsPublic(c, "subscription") {
		entries = append(entries, sitemapEntry{Location: "/subscription"})
	}
	if common.GetTheme() == "default" && seoModuleIsPublic(c, "rankings") {
		entries = append(entries, sitemapEntry{Location: "/rankings"})
	}
	if strings.TrimSpace(system_setting.GetLegalSettings().UserAgreement) != "" {
		entries = append(entries, sitemapEntry{Location: "/user-agreement"})
	}
	if strings.TrimSpace(system_setting.GetLegalSettings().PrivacyPolicy) != "" {
		entries = append(entries, sitemapEntry{Location: "/privacy-policy"})
	}
	if seoModuleIsPublic(c, "pricing") {
		entries = append(entries, sitemapEntry{Location: "/pricing"})
		if common.GetTheme() == "default" {
			pricing := seoPublicPricing(c)
			for index, item := range pricing {
				if index >= 49000 {
					break
				}
				entries = append(entries, sitemapEntry{
					Location: "/pricing/" + url.PathEscape(item.ModelName),
					LastMod:  item.UpdatedTime,
				})
			}
		}
	}
	c.String(http.StatusOK, buildSitemapXML(origin, entries))
}
