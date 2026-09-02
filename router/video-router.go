package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"

	"github.com/gin-gonic/gin"
)

func SetVideoRouter(router *gin.Engine) {
	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	// xAI-native video submit. The path shape /v1/videos/:segment collides with
	// the host-owned OpenAI video retrieve operation, so the plugin router
	// rejects it as a plugin-declared route; the host binds it to the factory
	// xai plugin's native decode/render members instead.
	router.POST(
		"/v1/videos/generations",
		middleware.RouteTag("relay"),
		middleware.TokenAuth(),
		middleware.PinHostOwnedPluginRoute(xaiVideoPluginKey, xaiVideoGenerationRoute),
		middleware.SystemPerformanceCheck(),
		middleware.ModelRequestRateLimit(),
		middleware.PrepareTaskPluginRoute(),
		middleware.Distribute(),
		middleware.UserConcurrencyLimit(),
		controller.RelayTask,
	)

	videoV1Router := router.Group("/v1")
	videoV1Router.Use(middleware.RouteTag("relay"))
	videoV1Router.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1Router.GET("/video/generations/:task_id", controller.RelayTaskFetch)
		videoV1Router.POST("/videos/:video_id/remix", middleware.UserConcurrencyLimit(), controller.RelayTask)
	}
}

const xaiVideoPluginKey = "xai"

var xaiVideoGenerationRoute = pluginruntime.Route{
	Method: http.MethodPost,
	Path:   "/v1/videos/generations",
	Type:   pluginruntime.RouteTypeSubmit,
	Decode: "decodeGeneration",
	Render: "generationCreated",
}
