package sub2api

import (
	"strings"

	"github.com/QuantumNous/new-api/relay/channel/newapi"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

type Adaptor struct {
	newapi.Adaptor
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	if info != nil && info.RelayMode == relayconstant.RelayModeResponses && strings.TrimSpace(request.PreviousResponseID) != "" {
		// Sub2API subscription accounts cannot use previous_response_id on
		// HTTP Responses requests. The input carries the current turn and is
		// still forwarded, so omit only the unsupported state reference.
		request.PreviousResponseID = ""
	}
	return a.Adaptor.ConvertOpenAIResponsesRequest(c, info, request)
}
