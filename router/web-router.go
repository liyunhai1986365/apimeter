package router

import (
	"embed"
	"html"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	agentservice "github.com/QuantumNous/new-api/service/agent"
	"github.com/QuantumNous/new-api/types"
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
	router.GET("/index.html", func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", currentIndexPage(c, assets))
	})
	router.NoRoute(func(c *gin.Context) {
		c.Set(middleware.RouteTagKey, "web")
		if strings.HasPrefix(c.Request.RequestURI, "/v1") || strings.HasPrefix(c.Request.RequestURI, "/api") || strings.HasPrefix(c.Request.RequestURI, "/assets") {
			controller.RelayNotFound(c)
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", currentIndexPage(c, assets))
	})
}

func currentIndexPage(c *gin.Context, assets ThemeAssets) []byte {
	if common.GetTheme() == "classic" {
		return renderIndexPage(c, assets.ClassicIndexPage)
	}
	return renderIndexPage(c, assets.DefaultIndexPage)
}

func renderIndexPage(c *gin.Context, indexPage []byte) []byte {
	title := html.EscapeString(indexPageTitle(c))
	page := string(indexPage)
	for _, placeholder := range []string{"New API", "loading"} {
		page = strings.ReplaceAll(page, "<title>"+placeholder+"</title>", "<title>"+title+"</title>")
		page = strings.ReplaceAll(page, `name="title" content="`+placeholder+`"`, `name="title" content="`+title+`"`)
	}
	return []byte(page)
}

func indexPageTitle(c *gin.Context) string {
	title := strings.TrimSpace(common.SystemName)
	if c != nil {
		if agentCtx, ok := common.GetContextKeyType[*types.AgentContext](c, constant.ContextKeyAgentContext); ok && agentCtx != nil {
			if branding, ok := agentservice.ParseBranding(agentCtx.Branding); ok && branding.SiteName != "" {
				title = branding.SiteName
			}
		}
	}
	if title == "" {
		return "New API"
	}
	return title
}
