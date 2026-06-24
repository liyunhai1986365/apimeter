package router

import (
	"strings"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

type configurableAssetEndpoint struct {
	method string
	path   string
}

func SetVideoRouter(router *gin.Engine) {
	// Video proxy: accepts either session auth (dashboard) or token auth (API clients)
	videoProxyRouter := router.Group("/v1")
	videoProxyRouter.Use(middleware.RouteTag("relay"))
	videoProxyRouter.Use(middleware.TokenOrUserAuth())
	{
		videoProxyRouter.GET("/videos/:task_id/content", controller.VideoProxy)
	}

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.POST("/video/generations", controller.RelayTask)
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", controller.RelayTask)
	}
	// openai compatible API video routes
	// docs: https://platform.openai.com/docs/api-reference/videos/create
	{
		videoV1Router.POST("/videos", controller.RelayTask)
		videoV1Router.GET("/videos/:task_id", controller.RelayTaskFetch)
	}

	registerConfigurableNativeRoutes(router)

	klingV1Router := router.Group("/kling/v1")
	klingV1Router.Use(middleware.RouteTag("relay"))
	klingV1Router.Use(middleware.KlingRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		klingV1Router.POST("/videos/text2video", controller.RelayTask)
		klingV1Router.POST("/videos/image2video", controller.RelayTask)
		klingV1Router.GET("/videos/text2video/:task_id", controller.RelayTaskFetch)
		klingV1Router.GET("/videos/image2video/:task_id", controller.RelayTaskFetch)
	}

	// Jimeng official API routes - direct mapping to official API format
	jimengOfficialGroup := router.Group("jimeng")
	jimengOfficialGroup.Use(middleware.RouteTag("relay"))
	jimengOfficialGroup.Use(middleware.JimengRequestConvert(), middleware.TokenAuth(), middleware.Distribute())
	{
		// Maps to: /?Action=CVSync2AsyncSubmitTask&Version=2022-08-31 and /?Action=CVSync2AsyncGetResult&Version=2022-08-31
		jimengOfficialGroup.POST("/", controller.RelayTask)
	}
}

func registerConfigurableNativeRoutes(router *gin.Engine) {
	registeredNativeRoutes := map[string]bool{}
	registeredResourceRoutes := map[string]bool{}
	registerCommonConfigurableAssetRoutes(router, registeredResourceRoutes)
	for _, profile := range configurable.ListProfiles() {
		registerConfigurableNativeSubmitRoute(router, profile, registeredNativeRoutes)
		registerConfigurableNativeFetchRoute(router, profile, registeredNativeRoutes)
		registerConfigurableResourceRoutes(router, profile, registeredResourceRoutes)
	}
}

func registerConfigurableNativeSubmitRoute(router *gin.Engine, profile *configurable.Profile, registered map[string]bool) {
	endpoint := profile.VideoNativeSubmit()
	if endpoint.Method == "" || endpoint.Path == "" {
		return
	}
	key := endpoint.Method + " " + ginPath(endpoint.Path)
	if registered[key] {
		return
	}
	registered[key] = true
	router.Handle(endpoint.Method, ginPath(endpoint.Path), middleware.RouteTag("relay"), middleware.ConfigurableNativeProfile(profile.ID, relayconstant.RelayModeVideoSubmit), middleware.TokenAuth(), middleware.Distribute(), controller.RelayTask)
}

func registerConfigurableNativeFetchRoute(router *gin.Engine, profile *configurable.Profile, registered map[string]bool) {
	endpoint := profile.VideoNativeFetch()
	if endpoint.Method == "" || endpoint.Path == "" {
		return
	}
	key := endpoint.Method + " " + ginPath(endpoint.Path)
	if registered[key] {
		return
	}
	registered[key] = true
	router.Handle(endpoint.Method, ginPath(endpoint.Path), middleware.RouteTag("relay"), middleware.ConfigurableNativeProfile(profile.ID, relayconstant.RelayModeVideoFetchByID), middleware.TokenAuth(), middleware.Distribute(), controller.RelayTaskFetch)
}

func registerConfigurableResourceRoutes(router *gin.Engine, profile *configurable.Profile, registered map[string]bool) {
	for _, resource := range profile.Resources {
		if resource.ID == "" {
			continue
		}
		endpoints := []configurable.EndpointConfig{resource.Public}
		endpoints = append(endpoints, resource.Aliases...)
		for _, endpoint := range endpoints {
			if endpoint.Method == "" || endpoint.Path == "" {
				continue
			}
			registerConfigurableResourceEndpoint(router, registered, endpoint.Method, endpoint.Path)
		}
	}
}

func registerCommonConfigurableAssetRoutes(router *gin.Engine, registered map[string]bool) {
	for _, endpoint := range commonConfigurableAssetEndpoints() {
		registerConfigurableResourceEndpoint(router, registered, endpoint.method, endpoint.path)
	}
}

func registerCommonConfigurableAssetAPIRoutes(apiRouter *gin.RouterGroup) {
	for _, endpoint := range commonConfigurableAssetEndpoints() {
		apiPath := "/" + strings.TrimPrefix(endpoint.path, "/api/")
		apiRouter.Handle(endpoint.method, ginPath(apiPath), middleware.RouteTag("relay"), middleware.ConfigurableResource("", ""), middleware.TokenAuth(), controller.RelayConfigurableResource)
	}
}

func commonConfigurableAssetEndpoints() []configurableAssetEndpoint {
	return []configurableAssetEndpoint{
		{method: "POST", path: "/api/assets/upload"},
		{method: "POST", path: "/api/assets"},
		{method: "GET", path: "/api/assets"},
		{method: "GET", path: "/api/assets/{id}"},
		{method: "DELETE", path: "/api/assets/{id}"},
	}
}

func registerConfigurableResourceEndpoint(router *gin.Engine, registered map[string]bool, method string, path string) {
	resolvedPath := ginPath(path)
	key := method + " " + resolvedPath
	if registered[key] {
		return
	}
	registered[key] = true
	if routeRegistered(router, method, resolvedPath) {
		return
	}
	router.Handle(method, resolvedPath, middleware.RouteTag("relay"), middleware.ConfigurableResource("", ""), middleware.TokenAuth(), controller.RelayConfigurableResource)
}

func routeRegistered(router *gin.Engine, method string, path string) bool {
	for _, route := range router.Routes() {
		if route.Method == method && route.Path == path {
			return true
		}
	}
	return false
}

func ginPath(path string) string {
	result := ""
	inParam := false
	paramName := ""
	for _, r := range path {
		switch r {
		case '{':
			inParam = true
			paramName = ""
		case '}':
			inParam = false
			result += ":" + paramName
		default:
			if inParam {
				paramName += string(r)
			} else {
				result += string(r)
			}
		}
	}
	return result
}
