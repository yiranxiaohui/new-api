package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIVideoResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "newapi-video",
		model:     "veo-3.1-fast",
		requestBody: map[string]any{
			"model":   "veo-3.1-fast",
			"input":   "a cute cat",
			"seconds": 8,
			"size":    "1280x720",
		},
		wantAction: "text_to_video",
		wantRequest: map[string]any{
			"model":   "veo-3.1-fast",
			"prompt":  "a cute cat",
			"seconds": float64(8),
			"size":    "1280x720",
		},
		wantUsageKeys:  []string{"seconds"},
		wantVendorName: "newapi-video",
	})
}

func loadNewAPIVideoPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("newapi-video")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "newapi-video"})
	require.NoError(t, err)
	return plugin
}

func callObject(t *testing.T, plugin *jsplugin.LoadedPlugin, hook string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.Call(t.Context(), hook, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func TestNewAPIVideoSubmitUsesNewAPIProtocolAndPerCallBilling(t *testing.T) {
	plugin := loadNewAPIVideoPlugin(t)
	ctx := map[string]any{
		"baseUrl":       "https://upstream.example.com",
		"apiKey":        "secret",
		"model":         "veo3.1",
		"upstreamModel": "veo3.1-fast",
		"action":        "text_to_video",
		"requestBody":   map[string]any{"model": "veo3.1", "prompt": "a cute cat", "seconds": "8", "size": "1280x720"},
	}

	submit := callObject(t, plugin, "buildSubmitRequest", ctx)
	assert.Equal(t, "https://upstream.example.com/v1/video/generations", submit["url"])
	assert.Equal(t, "POST", submit["method"])
	body, ok := submit["body"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "veo3.1-fast", body["model"])
	assert.Equal(t, "a cute cat", body["prompt"])
	assert.Equal(t, "8", body["seconds"])
	assert.Equal(t, "1280x720", body["size"])
	headers, ok := submit["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "application/json", headers["Content-Type"])

	ratios := callObject(t, plugin, "extractUsage", map[string]any{"usagePurpose": "billing_ratios", "requestBody": ctx["requestBody"]})
	assert.Empty(t, ratios, "New API Video upstreams are billed per call; no multipliers")

	remix := map[string]any{}
	for key, value := range ctx {
		remix[key] = value
	}
	remix["action"] = "remix"
	_, err := plugin.Engine.Call(t.Context(), "buildSubmitRequest", remix)
	require.ErrorContains(t, err, "remix is not supported")
}

func TestNewAPIVideoParseTaskResultExtractsVideoURL(t *testing.T) {
	plugin := loadNewAPIVideoPlugin(t)
	tests := []struct {
		name    string
		body    map[string]any
		wantURL string
	}{
		{"top-level url", map[string]any{"status": "completed", "url": "https://cdn.example/a.mp4"}, "https://cdn.example/a.mp4"},
		{"video_url", map[string]any{"status": "completed", "video_url": "https://cdn.example/b.mp4"}, "https://cdn.example/b.mp4"},
		{"videos array", map[string]any{"status": "completed", "videos": []any{map[string]any{"url": "https://cdn.example/c.mp4"}}}, "https://cdn.example/c.mp4"},
		{"data.url", map[string]any{"status": "completed", "data": map[string]any{"url": "https://cdn.example/d.mp4"}}, "https://cdn.example/d.mp4"},
		{"official sora shape has no url", map[string]any{"status": "completed", "id": "video_1", "object": "video"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callObject(t, plugin, "parseTaskResult", map[string]any{}, tt.body)
			assert.Equal(t, "SUCCESS", result["status"])
			if tt.wantURL == "" {
				assert.Nil(t, result["url"])
				return
			}
			assert.Equal(t, tt.wantURL, result["url"])
		})
	}

	failed := callObject(t, plugin, "parseTaskResult", map[string]any{}, map[string]any{"status": "failed", "error": map[string]any{"message": "nsfw"}})
	assert.Equal(t, "FAILURE", failed["status"])
	assert.Equal(t, "nsfw", failed["reason"])
}

func TestNewAPIVideoContentRequestPrefersStoredURLButProxiesContentEndpoints(t *testing.T) {
	plugin := loadNewAPIVideoPlugin(t)
	base := map[string]any{
		"artifactKey":    "video",
		"baseUrl":        "https://upstream.example.com",
		"apiKey":         "secret",
		"upstreamTaskId": "video_up_1",
		"clientRequest":  map[string]any{"method": "GET"},
		"status":         "SUCCESS",
	}
	withData := func(data map[string]any) map[string]any {
		ctx := map[string]any{}
		for key, value := range base {
			ctx[key] = value
		}
		ctx["data"] = data
		return ctx
	}

	direct := callObject(t, plugin, "buildContentRequest", withData(map[string]any{"url": "https://cdn.example/a.mp4"}))
	assert.Equal(t, "https://cdn.example/a.mp4", direct["url"])
	assert.Equal(t, true, direct["credentialless"])

	looped := callObject(t, plugin, "buildContentRequest", withData(map[string]any{"url": "http://127.0.0.1:3000/v1/videos/video_up_1/content"}))
	assert.Equal(t, "https://upstream.example.com/v1/videos/video_up_1/content", looped["url"])
	headers, ok := looped["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Bearer secret", headers["Authorization"])

	missing := callObject(t, plugin, "buildContentRequest", withData(map[string]any{"id": "video_up_1"}))
	assert.Equal(t, "https://upstream.example.com/v1/videos/video_up_1/content", missing["url"])
}

func TestNewAPIVideoRejectsMultipartUploads(t *testing.T) {
	plugin := loadNewAPIVideoPlugin(t)
	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "veo3.1",
		"body":  map[string]any{"kind": "multipart", "fields": map[string]any{"prompt": []any{"p"}}, "files": []any{}},
	})
	require.ErrorContains(t, err, "only accepts JSON")
}
