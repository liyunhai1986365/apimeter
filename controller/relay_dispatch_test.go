package controller

import (
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayDispatchFormatUsesConvertedRelayInfoFormat(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAIImage,
		RelayMode:   relayconstant.RelayModeImagesGenerations,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIImage), relayDispatchFormat(types.RelayFormatGemini, info))
}
