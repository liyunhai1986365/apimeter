package router

import (
	"embed"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// ThemeAssets holds the embedded frontend assets for both themes.
type ThemeAssets struct {
	DefaultBuildFS   embed.FS
	DefaultIndexPage []byte
	ClassicBuildFS   embed.FS
	ClassicIndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets ThemeAssets) {
	defaultFS := common.EmbedFolder(assets.DefaultBuildFS, "web/default/dist")
	classicFS := common.EmbedFolder(assets.ClassicBuildFS, "web/classic/dist")
	themeFS := common.NewThemeAwareFS(defaultFS, classicFS)

	router.Use(gzip.Gzip(gzip.DefaultCompression))
	router.Use(middleware.GlobalWebRateLimit())
	router.Use(middleware.Cache())
	router.Use(static.Serve("/", themeFS))
	router.GET("/robots.txt", serveRobotsTXT)
	router.HEAD("/robots.txt", serveRobotsTXT)
	router.GET("/sitemap.xml", serveSitemapXML)
	router.HEAD("/sitemap.xml", serveSitemapXML)
	router.GET("/index.html", func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", currentIndexPage(c, assets))
	})
	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/v1" || strings.HasPrefix(path, "/v1/") || path == "/api" || strings.HasPrefix(path, "/api/") || path == "/assets" || strings.HasPrefix(path, "/assets/") {
			controller.RelayNotFound(c)
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			controller.RelayNotFound(c)
			return
		}
		serveFrontendPage(c, assets)
	})
}

func serveFrontendPage(c *gin.Context, assets ThemeAssets) {
	c.Set(middleware.RouteTagKey, "web")
	page := resolveSEOPage(c)
	c.Header("Cache-Control", "no-cache")
	if page.Robots != "index, follow" {
		c.Header("X-Robots-Tag", "noindex, nofollow")
	}
	c.Data(page.Status, "text/html; charset=utf-8", currentIndexPage(c, assets))
}

func currentIndexPage(c *gin.Context, assets ThemeAssets) []byte {
	if common.GetTheme() == "classic" {
		return renderIndexPage(c, assets.ClassicIndexPage)
	}
	return renderIndexPage(c, assets.DefaultIndexPage)
}

func renderIndexPage(c *gin.Context, indexPage []byte) []byte {
	page := string(indexPage)
	return []byte(renderSEOMetadata(c, page))
}
