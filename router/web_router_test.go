package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
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

	require.Contains(t, string(rendered), `<title>Unified AI API Gateway | ModelSell</title>`)
	require.Contains(t, string(rendered), `<meta name="robots" content="index, follow" />`)
	require.Contains(t, string(rendered), `property="og:title" content="Unified AI API Gateway | ModelSell"`)
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

	require.Contains(t, string(rendered), `<title>Unified AI API Gateway | Agent &amp; Site</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="Unified AI API Gateway | Agent &amp; Site" />`)
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

	require.Contains(t, string(rendered), `<title>Unified AI API Gateway | ModelSell</title>`)
	require.Contains(t, string(rendered), `<meta name="title" content="Unified AI API Gateway | ModelSell" />`)
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
	require.Contains(t, recorder.Body.String(), "Sitemap: https://agent.example.com/sitemap.xml")
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

	require.Contains(t, rendered, `Unified AI API Gateway | Price $1`)
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
