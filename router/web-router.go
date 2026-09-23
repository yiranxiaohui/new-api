package router

import (
	"embed"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-contrib/static"
	"github.com/gin-gonic/gin"
)

// WebAssets holds the embedded dashboard frontend assets.
type WebAssets struct {
	BuildFS   embed.FS
	IndexPage []byte
}

func SetWebRouter(router *gin.Engine, assets WebAssets, pluginDispatcher gin.HandlerFunc) {
	frontendFS := common.EmbedFolder(assets.BuildFS, "web/dist")

	// Serve index.html through the dynamic injector so <title> and favicon
	// reflect the current SystemName / Logo (avoids the "New API" → custom
	// name flicker on first paint). Registered as explicit routes so the
	// static handler in the NoRoute chain never ships the embedded index.html.
	indexHandler := func(c *gin.Context) { serveIndex(c, assets) }
	router.GET("/", middleware.RouteTag("web"), middleware.GlobalWebRateLimit(), indexHandler)
	router.GET("/index.html", middleware.RouteTag("web"), middleware.GlobalWebRateLimit(), indexHandler)

	router.NoRoute(
		pluginDispatcher,
		middleware.RouteTag("web"),
		gzip.Gzip(gzip.DefaultCompression),
		middleware.AccessTokenAudit(),
		middleware.GlobalWebRateLimit(),
		middleware.Cache(),
		static.Serve("/", frontendFS),
		func(c *gin.Context) {
			// Missing build assets (for example chunks from a previous release
			// requested by a stale tab) must 404 instead of returning index.html,
			// otherwise the browser executes HTML as JavaScript.
			uri := c.Request.RequestURI
			if strings.HasPrefix(uri, "/v1") || strings.HasPrefix(uri, "/api") || strings.HasPrefix(uri, "/assets") || strings.HasPrefix(uri, "/static/") {
				controller.RelayNotFound(c)
				return
			}
			serveIndex(c, assets)
		},
	)
}
