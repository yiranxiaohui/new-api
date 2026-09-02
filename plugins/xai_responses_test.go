package plugins_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	builtinplugins "github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXAIResponsesProtocol(t *testing.T) {
	testVideoResponsesProtocol(t, videoResponsesTestCase{
		pluginKey: "xai",
		model:     "grok-imagine-video-1.5",
		requestBody: map[string]any{
			"model":   "grok-imagine-video-1.5",
			"input":   "a cat surfing",
			"seconds": 6,
			"size":    "1280x720",
		},
		wantAction: "text_to_video",
		wantRequest: map[string]any{
			"model":        "grok-imagine-video-1.5",
			"prompt":       "a cat surfing",
			"duration":     float64(6),
			"aspect_ratio": "16:9",
			"resolution":   "720p",
		},
		wantUsageKeys:  []string{"resolution", "seconds"},
		wantVendorName: "xai",
	})
}

func loadXAIPlugin(t *testing.T) *jsplugin.LoadedPlugin {
	t.Helper()
	source, err := builtinplugins.Source("xai")
	require.NoError(t, err)
	plugin, err := jsplugin.NewRegistry().RegisterFactory(source, jsplugin.Options{Key: "xai"})
	require.NoError(t, err)
	return plugin
}

func callPath(t *testing.T, plugin *jsplugin.LoadedPlugin, root string, path []string, args ...any) map[string]any {
	t.Helper()
	value, err := plugin.Engine.CallPath(t.Context(), root, path, args...)
	require.NoError(t, err)
	encoded, err := common.Marshal(value)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(encoded, &decoded))
	return decoded
}

func TestXAINativeDecodeValidatesModesAndDurationBounds(t *testing.T) {
	plugin := loadXAIPlugin(t)
	tests := []struct {
		name       string
		body       map[string]any
		wantAction string
		wantErr    string
	}{
		{"text to video", map[string]any{"model": "grok-imagine-video", "prompt": "a cat", "duration": 8}, "text_to_video", ""},
		{"image to video", map[string]any{"model": "grok-imagine-video", "image": map[string]any{"url": "https://img.example/a.png"}}, "image_to_video", ""},
		{"reference images", map[string]any{"model": "grok-imagine-video", "prompt": "a cat", "reference_images": []any{map[string]any{"url": "https://img.example/a.png"}}}, "reference_to_video", ""},
		{"missing model", map[string]any{"prompt": "a cat"}, "", "model field is required"},
		{"prompt required without image", map[string]any{"model": "grok-imagine-video"}, "", "prompt is required without an image"},
		{"image and references conflict", map[string]any{"model": "grok-imagine-video", "prompt": "p", "image": map[string]any{"url": "u"}, "reference_images": []any{map[string]any{"url": "u"}}}, "", "cannot be used together"},
		{"duration too large", map[string]any{"model": "grok-imagine-video", "prompt": "p", "duration": 3601}, "", "duration must be between 1 and 3600"},
		{"duration zero", map[string]any{"model": "grok-imagine-video", "prompt": "p", "duration": 0}, "", "duration must be between 1 and 3600"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := map[string]any{"body": map[string]any{"kind": "json", "value": tt.body}}
			if tt.wantErr != "" {
				_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"decodeGeneration"}, ctx)
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			resolved := callPath(t, plugin, "native", []string{"decodeGeneration"}, ctx)
			assert.Equal(t, "submit", resolved["kind"])
			assert.Equal(t, tt.wantAction, resolved["action"])
		})
	}

	_, err := plugin.Engine.CallPath(t.Context(), "native", []string{"decodeGeneration"}, map[string]any{"body": map[string]any{"kind": "form", "fields": map[string]any{}}})
	require.ErrorContains(t, err, "requires application/json")
}

func TestXAIOpenAIVideoDecodeTranslatesSizeAndInputReference(t *testing.T) {
	plugin := loadXAIPlugin(t)
	resolved := callPath(t, plugin, "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "grok-imagine-video",
		"body": map[string]any{"kind": "json", "value": map[string]any{
			"model": "grok-imagine-video", "prompt": "a cat", "seconds": "8", "size": "1792x1024", "input_reference": "file_abc",
		}},
	})
	assert.Equal(t, "image_to_video", resolved["action"])
	body, ok := resolved["requestBody"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(8), body["duration"])
	assert.Equal(t, "16:9", body["aspect_ratio"])
	assert.Equal(t, "1080p", body["resolution"])
	image, ok := body["image"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "file_abc", image["file_id"])
	assert.Nil(t, body["seconds"])
	assert.Nil(t, body["size"])

	_, err := plugin.Engine.CallPath(t.Context(), "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "grok-imagine-video",
		"body":  map[string]any{"kind": "json", "value": map[string]any{"model": "grok-imagine-video", "prompt": "p", "size": "640x640"}},
	})
	require.ErrorContains(t, err, "unsupported OpenAI video size")

	multipart := callPath(t, plugin, "protocols", []string{"openai_video", "decodeRequest"}, map[string]any{
		"model": "grok-imagine-video",
		"body": map[string]any{
			"kind":   "multipart",
			"fields": map[string]any{"model": []any{"grok-imagine-video"}, "prompt": []any{"a cat"}},
			"files":  []any{map[string]any{"ref": "request_file:input_reference", "field": "input_reference", "filename": "a.png", "mimeType": "image/png", "size": 12}},
		},
	})
	body, ok = multipart["requestBody"].(map[string]any)
	require.True(t, ok)
	image, ok = body["image"].(map[string]any)
	require.True(t, ok)
	placeholder, ok := image["url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "request_file:input_reference", placeholder["__fileRef"])
	assert.Equal(t, "dataUrl", placeholder["encoding"])
}

func TestXAIBillingRatiosFollowResolutionAndModel(t *testing.T) {
	plugin := loadXAIPlugin(t)
	tests := []struct {
		model      string
		resolution string
		want       float64
	}{
		{"grok-imagine-video", "480p", 1},
		{"grok-imagine-video", "720p", 1.4},
		{"grok-imagine-video-1.5", "720p", 1.75},
		{"grok-imagine-video-1.5", "1080p", 3.125},
	}
	for _, tt := range tests {
		t.Run(tt.model+"/"+tt.resolution, func(t *testing.T) {
			value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
				"usagePurpose":  "billing_ratios",
				"model":         tt.model,
				"upstreamModel": tt.model,
				"requestBody":   map[string]any{"model": tt.model, "prompt": "p", "resolution": tt.resolution},
			})
			require.NoError(t, err)
			ratios, ok := value.(map[string]any)
			require.True(t, ok)
			assert.InDelta(t, tt.want, ratios["resolution-"+tt.resolution], 1e-9)
			assert.Nil(t, ratios["resolution"], "the enum usage fact must not carry the numeric ratio")
			assert.InDelta(t, 8, ratios["seconds"], 1e-9, "default duration applies when omitted")
		})
	}
}

func TestXAIParseTaskResultAndSubmitResponse(t *testing.T) {
	plugin := loadXAIPlugin(t)

	submit, err := plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{"request_id": "req_1"}})
	require.NoError(t, err)
	assert.Equal(t, "req_1", submit.(map[string]any)["taskId"])
	_, err = plugin.Engine.Call(t.Context(), "parseSubmitResponse", map[string]any{}, map[string]any{"statusCode": 200, "body": map[string]any{}})
	require.ErrorContains(t, err, "request_id is empty")

	done, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"status": "done", "video": map[string]any{"url": "https://cdn.x.ai/v.mp4"}, "progress": 100})
	require.NoError(t, err)
	assert.Equal(t, "SUCCESS", done.(map[string]any)["status"])
	assert.Equal(t, "https://cdn.x.ai/v.mp4", done.(map[string]any)["url"])

	_, err = plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"status": "done"})
	require.ErrorContains(t, err, "missing video.url")

	expired, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"status": "expired"})
	require.NoError(t, err)
	assert.Equal(t, "FAILURE", expired.(map[string]any)["status"])
	assert.Equal(t, "video generation expired", expired.(map[string]any)["reason"])

	failed, err := plugin.Engine.Call(t.Context(), "parseTaskResult", map[string]any{}, map[string]any{"status": "failed", "error": map[string]any{"message": "moderation"}})
	require.NoError(t, err)
	assert.Equal(t, "moderation", failed.(map[string]any)["reason"])
}

func TestXAIBillingRatiosPassHostUsageValidation(t *testing.T) {
	plugin := loadXAIPlugin(t)
	value, err := plugin.Engine.Call(t.Context(), "extractUsage", map[string]any{
		"usagePurpose":  "billing_ratios",
		"model":         "grok-imagine-video-1.5",
		"upstreamModel": "grok-imagine-video-1.5",
		"requestBody":   map[string]any{"model": "grok-imagine-video-1.5", "prompt": "p", "duration": 6, "resolution": "1080p"},
	})
	require.NoError(t, err)
	facts, ok := value.(map[string]any)
	require.True(t, ok)
	for key, raw := range facts {
		if schema, declared := plugin.Meta.UsageSchema[key]; declared {
			require.Empty(t, schema.Enum, "ratio key %q must not collide with an enum usage fact", key)
		}
		_, numeric := raw.(float64)
		if !numeric {
			_, numeric = raw.(int64)
		}
		assert.True(t, numeric, "ratio %q must be numeric", key)
	}
	assert.InDelta(t, 3.125, facts["resolution-1080p"], 1e-9)
	assert.InDelta(t, 6, facts["seconds"], 1e-9)
}
