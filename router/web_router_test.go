package router

import (
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
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

func TestRequestOriginUsesCurrentWWWHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://www.modelsell.com/pricing", nil)

	require.Equal(t, "https://www.modelsell.com", requestOrigin(c))
}

func TestRequestOriginUsesCurrentIPHost(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "http://38.145.213.6:3000/pricing", nil)

	require.Equal(t, "http://38.145.213.6:3000", requestOrigin(c))
}

func TestRequestOriginKeepsAgentCustomDomain(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://modelsell.com/pricing", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	common.SetContextKey(c, constant.ContextKeyAgentContext, &types.AgentContext{
		Domain: "agent.example.com",
	})

	require.Equal(t, "https://agent.example.com", requestOrigin(c))
}

func TestRequestOriginUsesCurrentPublicHost(t *testing.T) {
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
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "https://agent.example.com/robots.txt", nil)
	c.Request.Host = "agent.example.com"
	c.Request.Header.Set("X-Forwarded-Proto", "https")

	serveRobotsTXT(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Disallow: /api/")
	require.NotContains(t, recorder.Body.String(), "Disallow: /sign-in")
	require.NotContains(t, recorder.Body.String(), "Disallow: /dashboard/")
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
	require.Contains(t, block, `property="og:site_name" content="A &amp; B &lt;Site&gt;"`)
	require.Contains(t, block, `name="twitter:card" content="summary"`)
	require.Equal(t, 1, strings.Count(block, seoBlockStart))
}

func TestBuildSEOBlockHidesShellBeforeClientFirstPaint(t *testing.T) {
	block := buildSEOBlock(seoPage{
		Title:  "ModelSell",
		Robots: "index, follow",
		Type:   "website",
	})

	require.Contains(t, block, seoShellBootStyle)
	require.Contains(t, block, seoShellBootScript)
	require.NotContains(t, block, `setTimeout`)
	require.Less(t, strings.Index(block, seoShellBootStyle), strings.Index(block, seoBlockEnd))
	require.Less(t, strings.Index(block, seoShellBootScript), strings.Index(block, seoBlockEnd))
}

func TestBuildSEOBlockDoesNotHidePagesWithoutShell(t *testing.T) {
	block := buildSEOBlock(seoPage{
		Title:  "Sign in",
		Robots: "noindex, nofollow",
		Type:   "website",
	})

	require.NotContains(t, block, `data-seo-shell-boot="true"`)
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
	require.Contains(t, jsonLD, `"@id":"https://example.com/pricing/gpt-4.1-mini#service"`)
	require.Contains(t, jsonLD, `"name":"gpt-4.1-mini API"`)
	require.Contains(t, jsonLD, `"provider":{"@id":"https://example.com/#organization"}`)
	require.Contains(t, jsonLD, `"mainEntity":{"@id":"https://example.com/pricing/gpt-4.1-mini#service"}`)
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

func TestCategoryPageJSONLDListsModelsAndBreadcrumbs(t *testing.T) {
	jsonLD := categoryPageJSONLD("https://example.com", seoCategoryPage{
		seoCategoryDefinition: seoCategoryDefinition{
			Slug: "image",
			Name: "Image Generation",
		},
		Models: []model.Pricing{
			{ModelName: "gpt-image-1"},
			{ModelName: "provider/image-edit"},
		},
	}, "Compare image model APIs.")

	require.Contains(t, jsonLD, `"@type":"CollectionPage"`)
	require.Contains(t, jsonLD, `"@id":"https://example.com/pricing/categories/image#webpage"`)
	require.Contains(t, jsonLD, `"name":"gpt-image-1 API pricing"`)
	require.Contains(t, jsonLD, `"url":"https://example.com/pricing/provider%2Fimage-edit"`)
	require.Contains(t, jsonLD, `"@type":"BreadcrumbList"`)
}

func TestBuildSEOShellRendersCategoryDirectoryAndModelLinks(t *testing.T) {
	page := seoPage{
		Path:        "/pricing/categories/video",
		Description: "Compare video model APIs.",
		Category: &seoCategoryPage{
			seoCategoryDefinition: seoCategoryDefinition{
				Slug:        "video",
				Name:        "Video Generation",
				Description: "Video generation models.",
			},
			Models: []model.Pricing{{ModelName: "seedance-2.0", QuotaType: 1, ModelPrice: 0.2}},
		},
	}

	shell := buildSEOShell(nil, page)
	require.Contains(t, shell, `Video Generation model API pricing`)
	require.Contains(t, shell, `seedance-2.0 API pricing`)
	require.Contains(t, shell, `$0.2 per request`)
	require.Contains(t, shell, `href="/pricing/seedance-2.0"`)
}

func TestSEOCategoriesFromPricingMatchesFrontendCategorySemantics(t *testing.T) {
	categories := seoCategoriesFromPricing([]model.Pricing{
		{ModelName: "default-text"},
		{ModelName: "explicit-text", Category: "text"},
		{ModelName: "image-model", Category: "IMAGE"},
		{ModelName: "custom-model", Category: "custom"},
	})

	require.Len(t, categories, 2)
	require.Equal(t, "text", categories[0].Slug)
	require.Equal(t, []string{"default-text", "explicit-text"}, []string{
		categories[0].Models[0].ModelName,
		categories[0].Models[1].ModelName,
	})
	require.Equal(t, "image", categories[1].Slug)
	require.Equal(t, "image-model", categories[1].Models[0].ModelName)
}

func TestSEOVendorsFromPricingBuildsStablePublicDirectories(t *testing.T) {
	providers := seoVendorsFromPricing(
		[]model.Pricing{
			{ModelName: "gpt-5", VendorID: 1},
			{ModelName: "claude-sonnet", VendorID: 2},
			{ModelName: "unassigned"},
		},
		[]model.PricingVendor{
			{ID: 3, Name: "Unused"},
			{ID: 1, Name: "Vendor AI"},
			{ID: 2, Name: "Vendor.AI"},
		},
	)

	require.Len(t, providers, 2)
	require.Equal(t, "vendor-ai", providers[0].Slug)
	require.Equal(t, "vendor-ai-2", providers[1].Slug)
	require.Equal(t, "gpt-5", providers[0].Models[0].ModelName)
	require.Equal(t, "provider-8", seoVendorSlugBase("通义千问", 8))
}

func TestProviderPageJSONLDListsModelsAndBreadcrumbs(t *testing.T) {
	jsonLD := providerPageJSONLD("https://example.com", seoVendorPage{
		Slug: "openai",
		Vendor: model.PricingVendor{
			ID:          5,
			Name:        "OpenAI",
			Description: "AI model provider.",
		},
		Models: []model.Pricing{
			{ModelName: "gpt-5"},
			{ModelName: "openai/image-1"},
		},
	}, "Compare OpenAI model APIs.")

	require.Contains(t, jsonLD, `"@id":"https://example.com/providers/openai#webpage"`)
	require.Contains(t, jsonLD, `"@id":"https://example.com/providers/openai#provider"`)
	require.Contains(t, jsonLD, `"name":"gpt-5 API pricing"`)
	require.Contains(t, jsonLD, `"url":"https://example.com/pricing/openai%2Fimage-1"`)
	require.Contains(t, jsonLD, `"@type":"BreadcrumbList"`)
}

func TestBuildSEOShellRendersProviderModels(t *testing.T) {
	page := seoPage{
		Path:        "/providers/openai",
		Description: "Compare OpenAI model APIs.",
		Vendor: &seoVendorPage{
			Slug: "openai",
			Vendor: model.PricingVendor{
				Name:        "OpenAI",
				Description: "AI model provider.",
			},
			Models: []model.Pricing{{ModelName: "gpt-5", Category: "text"}},
		},
	}

	shell := buildSEOShell(nil, page)
	require.Contains(t, shell, `OpenAI AI models &amp; APIs`)
	require.Contains(t, shell, `Available OpenAI model APIs`)
	require.Contains(t, shell, `href="/pricing/gpt-5"`)
	require.Contains(t, shell, `href="/providers"`)
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

func TestBuildSEOShellAddsPublicPageContentAndNavigation(t *testing.T) {
	oldTheme := common.GetTheme()
	common.SetTheme("default")
	t.Cleanup(func() { common.SetTheme(oldTheme) })

	home := buildSEOShell(nil, seoPage{Path: "/", Description: "AI model APIs."})
	require.Contains(t, home, `One API for leading AI model providers`)
	require.Contains(t, home, `OpenAI, Anthropic Claude, Google Gemini, DeepSeek`)
	require.Contains(t, home, `href="/rankings"`)

	about := buildSEOShell(nil, seoPage{Path: "/about", Description: "About."})
	require.Contains(t, about, `A unified gateway for AI applications`)
	require.Contains(t, about, html.EscapeString(common.SystemName)+` brings model discovery`)
	require.Contains(t, about, `href="/pricing"`)
}

func TestRenderSEOShellPreservesRootAttributes(t *testing.T) {
	page := `<html><body><div id="root" translate="no" class="notranslate"></div></body></html>`
	metadata := seoPage{
		Path:        "/about",
		Description: "About.",
		Robots:      "index, follow",
	}

	rendered := renderSEOShell(nil, page, metadata)

	require.Contains(t, rendered, `<div id="root" translate="no" class="notranslate"><div data-seo-shell="true"`)
	require.Contains(t, rendered, `A unified gateway for AI applications`)
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
	require.Equal(t, "noindex, nofollow", recorder.Header().Get("X-Robots-Tag"))
	require.Contains(t, recorder.Body.String(), `content="noindex, nofollow"`)
	require.Contains(t, recorder.Body.String(), `<div id="root"></div>`)
}

func TestServeFrontendPageReturnsOKForInviteRewards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assets := ThemeAssets{
		DefaultIndexPage: []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head><body><div id="root"></div></body>`),
		ClassicIndexPage: []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head><body><div id="root"></div></body>`),
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/invite-rewards", nil)
	serveFrontendPage(c, assets)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "noindex, nofollow", recorder.Header().Get("X-Robots-Tag"))
	require.Contains(t, recorder.Body.String(), `<div id="root"></div>`)
}

func TestResolveSEOPageRemovesCanonicalFromPrivatePath(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "https://example.com/sign-in", nil)

	page := resolveSEOPage(c)

	require.Equal(t, "noindex, nofollow", page.Robots)
	require.Empty(t, page.Canonical)
}

func TestServeFrontendPageKeepsEachRequestOrigin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	assets := ThemeAssets{
		DefaultIndexPage: []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head><body><div id="root"></div></body>`),
		ClassicIndexPage: []byte(`<head><!--seo-meta-start--><title>New API</title><!--seo-meta-end--></head><body><div id="root"></div></body>`),
	}

	for _, origin := range []string{
		"https://modelsell.com",
		"https://www.modelsell.com",
		"http://38.145.213.6:3000",
	} {
		t.Run(origin, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, origin+"/pricing", nil)

			serveFrontendPage(c, assets)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Empty(t, recorder.Header().Get("Location"))
			require.Contains(t, recorder.Body.String(), `rel="canonical" href="`+origin+`/pricing"`)
		})
	}
}

func TestKnownFrontendPathsMatchPublicAndPrivateRoutes(t *testing.T) {
	for _, path := range []string{
		"/pricing",
		"/providers",
		"/privacy-policy",
		"/sign-in",
		"/invite-rewards",
		"/withdrawal-management",
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
