package sora

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestAdaptor(channelType int) *TaskAdaptor {
	a := &TaskAdaptor{}
	a.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    channelType,
			ChannelBaseUrl: "https://upstream.example.com",
		},
	})
	return a
}

func TestBuildRequestURLByChannelType(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		action      string
		wantURL     string
		wantErr     bool
	}{
		{"sora generate", constant.ChannelTypeSora, constant.TaskActionGenerate, "https://upstream.example.com/v1/videos", false},
		{"openai generate", constant.ChannelTypeOpenAI, constant.TaskActionGenerate, "https://upstream.example.com/v1/videos", false},
		{"newapi video generate", constant.ChannelTypeNewAPIVideo, constant.TaskActionGenerate, "https://upstream.example.com/v1/video/generations", false},
		{"sora remix", constant.ChannelTypeSora, constant.TaskActionRemix, "https://upstream.example.com/v1/videos/video_123/remix", false},
		{"newapi video remix rejected", constant.ChannelTypeNewAPIVideo, constant.TaskActionRemix, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAdaptor(tt.channelType)
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: tt.channelType},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: tt.action, OriginTaskID: "video_123"},
			}
			url, err := a.BuildRequestURL(info)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, url)
		})
	}
}

func TestEstimateBillingNewAPIVideoNoRatios(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		channelType int
		wantNil     bool
	}{
		{"newapi video returns nil even with seconds/size", constant.ChannelTypeNewAPIVideo, true},
		{"sora returns ratios", constant.ChannelTypeSora, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newTestAdaptor(tt.channelType)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("task_request", relaycommon.TaskSubmitReq{
				Model: "veo-3.1-quality", Prompt: "p", Seconds: "8", Size: "1792x1024",
			})
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: tt.channelType},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionGenerate},
			}
			ratios := a.EstimateBilling(c, info)
			if tt.wantNil {
				assert.Nil(t, ratios)
			} else {
				require.NotNil(t, ratios)
				assert.Equal(t, 8.0, ratios["seconds"])
			}
		})
	}
}

func TestValidateRejectsRemixForNewAPIVideo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := newTestAdaptor(constant.ChannelTypeNewAPIVideo)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/video_123/remix",
		strings.NewReader(`{"prompt":"p"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeNewAPIVideo},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{Action: constant.TaskActionRemix},
	}
	taskErr := a.ValidateRequestAndSetAction(c, info)
	require.NotNil(t, taskErr)
	assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
}
