package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelBackedVideoContentURL(t *testing.T) {
	baseURL := "http://172.16.204.177:8000/"

	tests := []struct {
		name        string
		channelType int
		task        *model.Task
		wantURL     string
		wantOK      bool
	}{
		{
			name:        "xai upstream content URL uses configured channel",
			channelType: constant.ChannelTypeXai,
			task: &model.Task{
				TaskID: "task_public",
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "video_upstream",
					ResultURL:      "http://127.0.0.1:8000/v1/videos/video_upstream/content",
				},
			},
			wantURL: "http://172.16.204.177:8000/v1/videos/video_upstream/content",
			wantOK:  true,
		},
		{
			name:        "xai public proxy URL uses configured channel",
			channelType: constant.ChannelTypeXai,
			task: &model.Task{
				TaskID: "task_public",
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "video_upstream",
					ResultURL:      "https://new-api.example/v1/videos/task_public/content",
				},
			},
			wantURL: "http://172.16.204.177:8000/v1/videos/video_upstream/content",
			wantOK:  true,
		},
		{
			name:        "xai public CDN remains a protected direct fetch",
			channelType: constant.ChannelTypeXai,
			task: &model.Task{
				TaskID: "task_public",
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "video_upstream",
					ResultURL:      "https://vidgen.x.ai/videos/result.mp4",
				},
			},
			wantOK: false,
		},
		{
			name:        "openai always uses configured channel",
			channelType: constant.ChannelTypeOpenAI,
			task: &model.Task{
				TaskID: "task_public",
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "video/with space",
				},
			},
			wantURL: "http://172.16.204.177:8000/v1/videos/video%2Fwith%20space/content",
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotOK := channelBackedVideoContentURL(tt.channelType, baseURL, tt.task)
			require.Equal(t, tt.wantOK, gotOK)
			assert.Equal(t, tt.wantURL, gotURL)
		})
	}
}
