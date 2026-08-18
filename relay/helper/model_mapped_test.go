package helper

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestModelMappedHelperPreservesOriginModelName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("model_mapping", `{"alias-model":"real-model"}`)

	info := &relaycommon.RelayInfo{
		OriginModelName: "alias-model",
		RelayMode:       relayconstant.RelayModeChatCompletions,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "alias-model",
		},
	}
	req := &dto.GeneralOpenAIRequest{Model: "alias-model"}

	err := ModelMappedHelper(ctx, info, req)
	require.NoError(t, err)
	require.True(t, info.IsModelMapped)
	require.Equal(t, "alias-model", info.OriginModelName)
	require.Equal(t, "real-model", info.UpstreamModelName)
	require.Equal(t, "real-model", req.Model)
}

func TestModelMappedHelperUsesStandardModelNameForResponsesCompact(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("model_mapping", `{"alias-model":"real-model"}`)

	info := &relaycommon.RelayInfo{
		OriginModelName: "alias-model",
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "alias-model",
		},
	}
	req := &dto.OpenAIResponsesCompactionRequest{Model: "alias-model"}

	err := ModelMappedHelper(ctx, info, req)
	require.NoError(t, err)
	require.True(t, info.IsModelMapped)
	require.Equal(t, "alias-model", info.OriginModelName)
	require.Equal(t, "real-model", info.UpstreamModelName)
	require.Equal(t, "real-model", req.Model)
}

func TestReplaceResponseModelVariants(t *testing.T) {
	info := &relaycommon.RelayInfo{
		OriginModelName: "alias-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			IsModelMapped: true,
		},
	}

	rootJSON := `{"id":"resp_1","model":"real-model"}`
	rootResult := ReplaceResponseModelStr(rootJSON, info)
	require.Equal(t, "alias-model", gjson.Get(rootResult, "model").String())

	nestedJSON := `{"type":"response.completed","response":{"id":"resp_1","model":"real-model"}}`
	nestedResult := ReplaceResponseModelStr(nestedJSON, info)
	require.Equal(t, "alias-model", gjson.Get(nestedResult, "response.model").String())

	unrelatedJSON := `{"type":"response.output_text.delta","delta":"hello"}`
	unrelatedResult := ReplaceResponseModelStr(unrelatedJSON, info)
	require.Equal(t, unrelatedJSON, unrelatedResult)
}
