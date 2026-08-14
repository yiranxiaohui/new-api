package router

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXAIVideoGenerationRouteIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()

	require.NotPanics(t, func() {
		SetVideoRouter(engine)
	})

	for _, route := range engine.Routes() {
		if route.Method == http.MethodPost && route.Path == "/v1/videos/generations" {
			return
		}
	}
	assert.Fail(t, "xAI video generation route is not registered")
}
