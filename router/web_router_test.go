package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRenderIndexPageUsesSystemNameForInitialTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "ModelSell"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	html := []byte(`<head><!--seo-meta-start--><title>New API</title><meta name="title" content="New API" /><!--seo-meta-end--></head>`)

	rendered := renderIndexPage(c, html)

	require.Contains(t, string(rendered), `<title>AI Model APIs, Pricing &amp; Access | ModelSell</title>`)
	require.Contains(t, string(rendered), `<meta name="robots" content="index, follow" />`)
	require.Contains(t, string(rendered), `property="og:title" content="AI Model APIs, Pricing &amp; Access | ModelSell"`)
	require.NotContains(t, string(rendered), `<title>New API</title>`)
}

func TestRenderIndexPageUsesAgentSiteNameForInitialTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "ModelSell"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		Branding: `{"site_name":"Agent & Site"}`,
	})
	html := []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head>`)

	rendered := renderIndexPage(c, html)

	require.Contains(t, string(rendered), `<title>AI Model APIs, Pricing &amp; Access | Agent &amp; Site</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="AI Model APIs, Pricing &amp; Access | Agent &amp; Site" />`)
	require.NotContains(t, string(rendered), `<title>New API</title>`)
}

func TestRenderIndexPageReplacesLoadingPlaceholderTitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "ModelSell"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	html := []byte(`<title>loading</title><meta name="title" content="loading" />`)

	rendered := renderIndexPage(c, html)

	require.Contains(t, string(rendered), `<title>AI Model APIs, Pricing &amp; Access | ModelSell</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="AI Model APIs, Pricing &amp; Access | ModelSell" />`)
	require.NotContains(t, string(rendered), `<title>loading</title>`)
}

func TestResolveSEOPageUsesCanonicalPathWithoutQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = ""
	t.Cleanup(func() {
		system_setting.ServerAddress = oldServerAddress
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "https://example.com/pricing?search=gpt&group=vip", nil)
	c.Request.Host = "example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	page := resolveSEOPage(c)

	require.Equal(t, http.StatusOK, page.Status)
	require.Equal(t, "https://example.com/pricing", page.Canonical)
	require.Equal(t, "index, follow", page.Robots)
}

func TestRequestOriginUsesConfiguredPrimarySiteForMainDomain(t *testing.T) {
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com/base?ignored=true"
	t.Cleanup(func() {
		system_setting.ServerAddress = oldServerAddress
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://www.modelsell.com/pricing", nil)

	require.Equal(t, "https://modelsell.com", requestOrigin(c))
}

func TestRequestOriginKeepsAgentCustomDomain(t *testing.T) {
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://modelsell.com"
	t.Cleanup(func() {
		system_setting.ServerAddress = oldServerAddress
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://modelsell.com/pricing", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		Domain: "agent.example.com",
	})

	require.Equal(t, "https://agent.example.com", requestOrigin(c))
}

func TestRequestOriginIgnoresDefaultLocalhostOnPublicRequest(t *testing.T) {
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "http://localhost:3000"
	t.Cleanup(func() {
		system_setting.ServerAddress = oldServerAddress
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://gateway.example.com/pricing", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	require.Equal(t, "https://gateway.example.com", requestOrigin(c))
}

func TestResolveSEOPageMarksUnknownPathAsRealNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/definitely-not-a-route", nil)

	page := resolveSEOPage(c)

	require.Equal(t, http.StatusNotFound, page.Status)
	require.Equal(t, "noindex, nofollow", page.Robots)
	require.Empty(t, page.Canonical)
}

func TestServeRobotsTXTIncludesRequestScopedSitemap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = ""
	t.Cleanup(func() {
		system_setting.ServerAddress = oldServerAddress
	})
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "https://agent.example.com/robots.txt", nil)
	c.Request.Host = "agent.example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	serveRobotsTXT(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Disallow: /api/")
	require.Contains(t, recorder.Body.String(), "Sitemap: https://agent.example.com/sitemap.xml")
}

func TestSEOSystemRoutesSupportHeadRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/robots.txt", serveRobotsTXT)
	engine.HEAD("/robots.txt", serveRobotsTXT)
	engine.GET("/sitemap.xml", serveSitemapXML)
	engine.HEAD("/sitemap.xml", serveSitemapXML)

	for _, path := range []string{"/robots.txt", "/sitemap.xml"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodHead, path, nil)
		engine.ServeHTTP(recorder, request)

		require.Equal(t, http.StatusOK, recorder.Code, path)
		require.NotContains(t, recorder.Header().Get("Content-Type"), "text/html", path)
	}
}

func TestBuildSEOBlockEscapesValues(t *testing.T) {
	block := buildSEOBlock(seoPage{
		Title:       `A & B <Site>`,
		Description: `quoted "description"`,
		Robots:      "index, follow",
		Type:        "website",
	})

	require.Contains(t, block, `A &amp; B &lt;Site&gt;`)
	require.Contains(t, block, `quoted &#34;description&#34;`)
	require.Equal(t, 1, strings.Count(block, seoBlockStart))
}

func TestRenderSEOMetadataKeepsDollarSignsLiteral(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldSystemName := common.SystemName
	common.SystemName = "Price $1"
	t.Cleanup(func() {
		common.SystemName = oldSystemName
	})
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	rendered := renderSEOMetadata(c, `<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head>`)

	require.Contains(t, rendered, `AI Model APIs, Pricing &amp; Access | Price $1`)
}

func TestModelPageJSONLDDescribesAPIServiceWithoutFakeOffer(t *testing.T) {
	jsonLD := modelPageJSONLD("https://example.com", model.Pricing{
		ModelName:       "gpt-4.1-mini",
		ModelRatio:      0.2,
		CompletionRatio: 4,
		SupportedEndpointTypes: []constant.EndpointType{
			constant.EndpointTypeOpenAI,
			constant.EndpointTypeOpenAIResponse,
		},
	}, "Compare gpt-4.1-mini API pricing.")

	require.Contains(t, jsonLD, `"@type":"Service"`)
	require.Contains(t, jsonLD, `"name":"gpt-4.1-mini API"`)
	require.Contains(t, jsonLD, `OpenAI Responses API`)
	require.Contains(t, jsonLD, `POST /v1/chat/completions`)
	require.Contains(t, jsonLD, `POST /v1/responses`)
	require.Contains(t, jsonLD, `"name":"Base input price"`)
	require.Contains(t, jsonLD, `"value":"$0.4 per 1M tokens"`)
	require.NotContains(t, jsonLD, `"Offer"`)
}

func TestSEOModelPriceItemsUsePublicBillingSemantics(t *testing.T) {
	require.Equal(t, []seoPriceItem{
		{Label: "Base input price", Value: "$0.5 per 1M tokens"},
		{Label: "Base output price", Value: "$2 per 1M tokens"},
	}, seoModelPriceItems(model.Pricing{
		ModelRatio:      0.25,
		CompletionRatio: 4,
	}))

	require.Equal(t, []seoPriceItem{
		{Label: "Base price", Value: "$0.02 per request"},
	}, seoModelPriceItems(model.Pricing{
		QuotaType:  1,
		ModelPrice: 0.02,
	}))

	require.Equal(t, []seoPriceItem{
		{Label: "Billing model", Value: "Dynamic usage-based pricing"},
	}, seoModelPriceItems(model.Pricing{
		BillingMode: "tiered_expr",
		BillingExpr: `tier("base", p * 2.5 + c * 15)`,
	}))
}

func TestModelPageJSONLDIncludesCanonicalModelAliases(t *testing.T) {
	jsonLD := modelPageJSONLD("https://example.com", model.Pricing{
		ModelName:    "primary-model",
		AliasModels:  []string{"primary-model-latest", "primary-model-preview"},
		Capabilities: []string{"function_calling", "vision"},
	}, "Primary model API pricing.")

	require.Contains(t, jsonLD, `"alternateName":["primary-model-latest","primary-model-preview"]`)
	require.Contains(t, jsonLD, `"category":"function calling, vision"`)
}

func TestBuildSEOShellRendersModelPriceAndSearchableDetails(t *testing.T) {
	page := seoPage{
		Path:        "/pricing/gpt-4.1-mini",
		Description: "gpt-4.1-mini API pricing.",
		Model: &model.Pricing{
			ModelName:              "gpt-4.1-mini",
			ModelRatio:             0.2,
			CompletionRatio:        4,
			AliasModels:            []string{"gpt-4.1-mini-latest"},
			Capabilities:           []string{"function_calling", "vision"},
			SupportedEndpointTypes: []constant.EndpointType{constant.EndpointTypeOpenAI},
		},
	}

	shell := buildSEOShell(nil, page)
	require.Contains(t, shell, `gpt-4.1-mini API pricing &amp; access`)
	require.Contains(t, shell, `$0.4 per 1M tokens`)
	require.Contains(t, shell, `$1.6 per 1M tokens`)
	require.Contains(t, shell, `OpenAI Chat Completions API`)
	require.Contains(t, shell, `POST /v1/chat/completions`)
	require.Contains(t, shell, `gpt-4.1-mini-latest`)
	require.Contains(t, shell, `function calling, vision`)
}

func TestServeFrontendPageReturnsNotFoundHTMLForUnknownPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assets := ThemeAssets{
		DefaultIndexPage: []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head><body><div id="root"></div></body>`),
		ClassicIndexPage: []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head><body><div id="root"></div></body>`),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/missing-page", nil)
	serveFrontendPage(c, assets)

	require.Equal(t, http.StatusNotFound, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	require.Contains(t, recorder.Body.String(), `content="noindex, nofollow"`)
	require.Contains(t, recorder.Body.String(), `<div id="root"></div>`)
}

func TestKnownFrontendPathsMatchPublicAndPrivateRoutes(t *testing.T) {
	for _, path := range []string{
		"/pricing",
		"/privacy-policy",
		"/sign-in",
		"/dashboard/overview",
		"/console/chat/example",
		"/oauth/github",
	} {
		require.True(t, isKnownFrontendPath(path), path)
	}
	require.False(t, isKnownFrontendPath("/missing-page"))
	require.False(t, isKnownFrontendPath("/pricing/unknown-model"))
}

func TestResolveSEOPageHandlesEncodedModelPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/pricing/vendor%2Fmodel", nil)

	page := resolveSEOPage(c)

	require.Contains(t, []int{http.StatusOK, http.StatusNotFound}, page.Status)
	require.Equal(t, "/pricing/vendor%2Fmodel", normalizeSEOPath(c.Request.URL.EscapedPath()))
}

func TestSEOModuleIsPublicUsesAgentNavigationOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		Branding: `{"header_nav_modules":"{\"pricing\":{\"enabled\":true,\"requireAuth\":true}}"}`,
	})

	require.False(t, seoModuleIsPublic(c, "pricing"))
}
