package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestXAIChannelAdvertisesVideoEndpoint(t *testing.T) {
	endpointTypes := GetEndpointTypesByChannelType(constant.ChannelTypeXai, "grok-imagine-video")

	assert.Contains(t, endpointTypes, constant.EndpointTypeOpenAIVideo)
}
