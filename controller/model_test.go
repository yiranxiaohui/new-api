package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func stringPtr(value string) *string {
	return &value
}

func TestCollectHiddenMappedModelNames(t *testing.T) {
	channels := []*model.Channel{
		{
			Status: common.ChannelStatusEnabled,
			Group:  "default, vip",
			ModelMapping: stringPtr(`{
				"claude-sonnet": "claude-3-7-sonnet",
				"claude-3-7-sonnet": "claude-3-7-sonnet-latest",
				"gpt-4o": "gpt-4o-2024-11-20"
			}`),
		},
		{
			Status:       common.ChannelStatusEnabled,
			Group:        "vip",
			ModelMapping: stringPtr(`{"vip-alias":"vip-target"}`),
		},
		{
			Status:       0,
			Group:        "default",
			ModelMapping: stringPtr(`{"disabled-alias":"disabled-target"}`),
		},
		{
			Status:       common.ChannelStatusEnabled,
			Group:        "default",
			ModelMapping: stringPtr(`{"broken-json":`),
		},
	}

	hiddenDefaultModels := collectHiddenMappedModelNames(channels, []string{"default"})
	require.True(t, hiddenDefaultModels["claude-3-7-sonnet-latest"])
	require.True(t, hiddenDefaultModels["gpt-4o-2024-11-20"])
	require.False(t, hiddenDefaultModels["claude-3-7-sonnet"])
	require.False(t, hiddenDefaultModels["vip-target"])
	require.False(t, hiddenDefaultModels["disabled-target"])

	hiddenVipModels := collectHiddenMappedModelNames(channels, []string{"vip"})
	require.True(t, hiddenVipModels["vip-target"])
	require.True(t, hiddenVipModels["gpt-4o-2024-11-20"])
}
