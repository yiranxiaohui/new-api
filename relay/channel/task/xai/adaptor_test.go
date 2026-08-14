package xai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestContext(body string) *gin.Context {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")
	return context
}

func newTestAdaptor() *TaskAdaptor {
	adaptor := &TaskAdaptor{}
	adaptor.Init(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:    constant.ChannelTypeXai,
		ChannelBaseUrl: "https://upstream.example.com",
		ApiKey:         "secret-key",
	}})
	return adaptor
}

func TestValidateRequestModesAndDurationBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		body       string
		wantAction string
		wantCode   string
	}{
		{
			name:       "text to video",
			body:       `{"model":"grok-imagine-video","prompt":"a cat","duration":"8"}`,
			wantAction: constant.TaskActionTextGenerate,
		},
		{
			name:       "image to video without prompt",
			body:       `{"model":"grok-imagine-video","image":{"url":"https://example.com/cat.png"}}`,
			wantAction: constant.TaskActionGenerate,
		},
		{
			name:       "reference to video",
			body:       `{"model":"grok-imagine-video-1.5","prompt":"put the cat on stage","reference_images":[{"file_id":"file_123"}]}`,
			wantAction: constant.TaskActionReferenceGenerate,
		},
		{
			name:     "model is required",
			body:     `{"prompt":"a cat"}`,
			wantCode: "missing_model",
		},
		{
			name:     "text mode requires prompt",
			body:     `{"model":"grok-imagine-video"}`,
			wantCode: "invalid_request",
		},
		{
			name:     "image and references are mutually exclusive",
			body:     `{"model":"grok-imagine-video","prompt":"a cat","image":{"url":"https://example.com/a.png"},"reference_images":[{"url":"https://example.com/b.png"}]}`,
			wantCode: "invalid_request",
		},
		{
			name:     "zero duration is rejected",
			body:     `{"model":"grok-imagine-video","prompt":"a cat","duration":0}`,
			wantCode: "invalid_duration",
		},
		{
			name:     "unbounded seconds alias is rejected",
			body:     `{"model":"grok-imagine-video","prompt":"a cat","seconds":"9999999999"}`,
			wantCode: "invalid_duration",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := newTestContext(test.body)
			defer common.CleanupBodyStorage(context)
			info := &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{}}

			taskErr := newTestAdaptor().ValidateRequestAndSetAction(context, info)
			if test.wantCode != "" {
				require.NotNil(t, taskErr)
				assert.Equal(t, test.wantCode, taskErr.Code)
				assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
				return
			}
			require.Nil(t, taskErr)
			assert.Equal(t, test.wantAction, info.Action)
		})
	}
}

func TestBuildRequestUsesOfficialEndpointAndPreservesPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context := newTestContext(`{"model":"public-model","prompt":"a cat","duration":8,"storage_options":{"filename":"cat.mp4","public_url":false}}`)
	defer common.CleanupBodyStorage(context)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "grok-imagine-video-1.5"}}
	adaptor := newTestAdaptor()

	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	raw, err := io.ReadAll(body)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(raw, &payload))
	assert.Equal(t, "grok-imagine-video-1.5", payload["model"])
	storageOptions, ok := payload["storage_options"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, storageOptions["public_url"])

	requestURL, err := adaptor.BuildRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://upstream.example.com/v1/videos/generations", requestURL)

	request := httptest.NewRequest(http.MethodPost, requestURL, nil)
	require.NoError(t, adaptor.BuildRequestHeader(context, request, info))
	assert.Equal(t, "Bearer secret-key", request.Header.Get("Authorization"))
	assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
}

func TestEstimateBillingUsesDurationAndResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name           string
		body           string
		upstreamModel  string
		wantSeconds    float64
		wantResolution float64
	}{
		{"official defaults", `{"model":"grok-imagine-video","prompt":"a cat"}`, "grok-imagine-video", 8, 1},
		{"legacy 720p", `{"model":"grok-imagine-video","prompt":"a cat","duration":6,"resolution":"720p"}`, "grok-imagine-video", 6, 1.4},
		{"1.5 1080p", `{"model":"grok-imagine-video-1.5","prompt":"a cat","seconds":"12","resolution":"1080p"}`, "grok-imagine-video-1.5", 12, 3.125},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := newTestContext(test.body)
			defer common.CleanupBodyStorage(context)
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: test.upstreamModel},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			}
			adaptor := newTestAdaptor()
			require.Nil(t, adaptor.ValidateRequestAndSetAction(context, info))

			ratios := adaptor.EstimateBilling(context, info)
			assert.Equal(t, test.wantSeconds, ratios["seconds"])
			assert.Equal(t, test.wantResolution, ratios["resolution"])
		})
	}
}

func TestDoResponseReturnsPublicRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	response := &http.Response{Body: io.NopCloser(strings.NewReader(`{"request_id":"upstream-secret-id"}`))}

	upstreamID, taskData, taskErr := newTestAdaptor().DoResponse(context, response, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	})

	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-secret-id", upstreamID)
	assert.JSONEq(t, `{"request_id":"upstream-secret-id"}`, string(taskData))
	assert.JSONEq(t, `{"request_id":"task_public"}`, recorder.Body.String())
}

func TestParseTaskResult(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus model.TaskStatus
		wantURL    string
		wantReason string
		wantErr    bool
	}{
		{"pending", `{"status":"pending","progress":25}`, model.TaskStatusQueued, "", "", false},
		{"done", `{"status":"done","video":{"url":"https://cdn.example.com/video.mp4","duration":8}}`, model.TaskStatusSuccess, "https://cdn.example.com/video.mp4", "", false},
		{"failed", `{"status":"failed","error":{"message":"moderation rejected"}}`, model.TaskStatusFailure, "", "moderation rejected", false},
		{"expired", `{"status":"expired"}`, model.TaskStatusFailure, "", "video generation expired", false},
		{"done without url", `{"status":"done","video":{}}`, "", "", "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := newTestAdaptor().ParseTaskResult([]byte(test.body))
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.wantStatus, model.TaskStatus(result.Status))
			assert.Equal(t, test.wantURL, result.Url)
			assert.Equal(t, test.wantReason, result.Reason)
		})
	}
}

func TestConvertToXAIResultHidesUpstreamRequestID(t *testing.T) {
	adaptor := newTestAdaptor()
	task := &model.Task{
		TaskID: "task_public",
		Status: model.TaskStatusSuccess,
		Properties: model.Properties{
			OriginModelName: "public-model",
		},
		PrivateData: model.TaskPrivateData{ResultURL: "https://cdn.example.com/video.mp4"},
		Data:        []byte(`{"request_id":"upstream-secret-id","status":"done","video":{"duration":8},"usage":{"cost_in_usd_ticks":500000000}}`),
	}

	body, err := adaptor.ConvertToOpenAIVideo(task)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, common.Unmarshal(body, &result))
	assert.NotContains(t, result, "request_id")
	assert.Equal(t, "done", result["status"])
	assert.Equal(t, "public-model", result["model"])
	video, ok := result["video"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://cdn.example.com/video.mp4", video["url"])
	assert.NotNil(t, result["usage"])
}
