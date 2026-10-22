package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostProtocolRegistryDrivesProtocolRoutesOnce(t *testing.T) {
	engine := gin.New()
	SetTaskPluginProtocolRouter(engine)

	expected := []string{
		"POST /v1/responses",
		"GET /v1/responses/:response_id",
		"POST /v1/videos",
		"GET /v1/videos/:task_id",
		"GET /v1/videos/:task_id/content",
		"HEAD /v1/videos/:task_id/content",
		"POST /v1/images/generations",
		"POST /v1/images/edits",
	}
	actual := make([]string, 0, len(engine.Routes()))
	for _, route := range engine.Routes() {
		actual = append(actual, fmt.Sprintf("%s %s", route.Method, route.Path))
	}
	sort.Strings(expected)
	sort.Strings(actual)
	assert.Equal(t, expected, actual)
}

func TestTaskRoutesHonorUserConcurrency(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	t.Cleanup(func() { model.DB, model.LOG_DB = previousDB, previousLogDB })
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Task{}, &model.Channel{}))
	require.NoError(t, i18n.Init())
	previousLimit := setting.UserMaxConcurrency
	previousRateLimit := setting.ModelRequestRateLimitEnabled
	setting.ModelRequestRateLimitEnabled = false
	t.Cleanup(func() {
		setting.UserMaxConcurrency = previousLimit
		setting.ModelRequestRateLimitEnabled = previousRateLimit
	})

	source := routerPluginSource("concurrency", "1.0.0", `[
		{method: "POST", path: "/concurrency/submit", type: "submit"},
		{method: "POST", path: "/concurrency/dynamic", type: "dynamic"},
		{method: "GET", path: "/concurrency/tasks/:task_id", type: "query"}
	]`)
	source = strings.Replace(source, `return {kind: "submit", model: "model", requestBody: ctx.body.value};`,
		`if (ctx.body.value.query) return {kind: "query", taskIds: ["missing"]};
		return {kind: "submit", model: "model", requestBody: ctx.body.value};`, 1)
	plugin, err := jsplugin.CompilePlugin(source, jsplugin.Options{})
	require.NoError(t, err)
	registry := jsplugin.NewRegistry()
	require.NoError(t, registry.ReplaceOverrides([]*jsplugin.LoadedPlugin{plugin}))
	engine := gin.New()
	SetRelayRouter(engine)
	SetVideoRouter(engine)
	SetTaskPluginProtocolRouter(engine)
	builder := newPluginGenerationBuilder(engine.Routes(), nil, productionPluginRouteHandlers)
	require.NoError(t, registry.SetGenerationPreparer(builder.prepare))
	engine.NoRoute((&pluginRouteDispatcher{registry: registry}).dispatch)

	for index, tc := range []struct {
		name     string
		global   int
		override int
	}{
		{name: "global", global: 1},
		{name: "user override", global: 0, override: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setting.UserMaxConcurrency = tc.global
			user := model.User{
				Username: fmt.Sprintf("concurrency%d", index), Status: common.UserStatusEnabled,
				Group: "default", Quota: 100, MaxConcurrency: &tc.override,
				AffCode: fmt.Sprintf("conc%d", index),
			}
			require.NoError(t, model.DB.Create(&user).Error)
			key := fmt.Sprintf("concurrencykey%d", index)
			require.NoError(t, model.DB.Create(&model.Token{
				UserId: user.Id, Key: key, Status: common.TokenStatusEnabled,
				ExpiredTime: -1, UnlimitedQuota: true,
			}).Error)
			allowed, release := service.TryAcquireUserConcurrency(user.Id, 1)
			require.True(t, allowed)
			require.NotNil(t, release)
			t.Cleanup(release)

			for _, path := range []string{
				"/v1/responses", "/v1/images/generations", "/v1/images/edits",
				"/v1/videos", "/v1/video/generations",
				"/concurrency/submit", "/concurrency/dynamic",
			} {
				t.Run(path, func(t *testing.T) {
					request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"model":"model"}`))
					request.Header.Set("Authorization", "Bearer "+key)
					request.Header.Set("Content-Type", "application/json")
					recorder := httptest.NewRecorder()
					engine.ServeHTTP(recorder, request)
					assert.Equal(t, http.StatusTooManyRequests, recorder.Code, recorder.Body.String())
					assert.Contains(t, recorder.Body.String(), "concurrency limit")
				})
			}

			// Retrieval, including a dynamic route decoded as a query, remains
			// available while the user's submission capacity is exhausted.
			for _, query := range []struct{ method, path, body string }{
				{http.MethodGet, "/v1/responses/missing", ""},
				{http.MethodGet, "/concurrency/tasks/missing", ""},
				{http.MethodPost, "/concurrency/dynamic", `{"query":true}`},
			} {
				request := httptest.NewRequest(query.method, query.path, strings.NewReader(query.body))
				request.Header.Set("Authorization", "Bearer "+key)
				request.Header.Set("Content-Type", "application/json")
				recorder := httptest.NewRecorder()
				engine.ServeHTTP(recorder, request)
				assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
			}

			release()
			request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"model"}`))
			request.Header.Set("Authorization", "Bearer "+key)
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, request)
			// The request now reaches distribution (there are no channels).
			assert.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
			allowed, releaseAfter := service.TryAcquireUserConcurrency(user.Id, 1)
			require.True(t, allowed, "a downstream failure must release the request's slot")
			require.NotNil(t, releaseAfter)
			releaseAfter()
		})
	}
}
