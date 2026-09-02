package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	_ "github.com/QuantumNous/new-api/plugins"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXAIVideoGenerationRouteIsHostOwned(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	_ = SetPluginRouter(engine)

	registered := false
	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/v1/videos/generations" {
			registered = true
		}
	}
	require.True(t, registered, "xAI video generation route must be registered on the host router")

	generation := jsplugin.DefaultRegistry.Generation()
	require.NotNil(t, generation)
	_, found := generation.Get(xaiVideoPluginKey)
	assert.True(t, found, "xai factory plugin must stay in the routing generation")
	_, declared := generation.LookupDeclaredRoute(http.MethodPost, "/v1/videos/generations")
	assert.False(t, declared, "the xAI route is host-owned, not plugin-declared")
	assert.Empty(t, jsplugin.DefaultRegistry.RoutingErrors()[xaiVideoPluginKey])

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(`{"model":"grok-imagine-video","prompt":"a cat"}`))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	assert.Equal(t, http.StatusUnauthorized, recorder.Code, "unauthenticated requests must stop at TokenAuth")
}
