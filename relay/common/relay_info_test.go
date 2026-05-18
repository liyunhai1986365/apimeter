package common

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestRelayInfoGetFinalRequestRelayFormatPrefersExplicitFinal(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:             types.RelayFormatOpenAI,
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatOpenAIResponses), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToConversionChain(t *testing.T) {
	info := &RelayInfo{
		RelayFormat:            types.RelayFormatOpenAI,
		RequestConversionChain: []types.RelayFormat{types.RelayFormatOpenAI, types.RelayFormatClaude},
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatFallsBackToRelayFormat(t *testing.T) {
	info := &RelayInfo{
		RelayFormat: types.RelayFormatGemini,
	}

	require.Equal(t, types.RelayFormat(types.RelayFormatGemini), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoGetFinalRequestRelayFormatNilReceiver(t *testing.T) {
	var info *RelayInfo
	require.Equal(t, types.RelayFormat(""), info.GetFinalRequestRelayFormat())
}

func TestRelayInfoResponseModelUsesOriginWhenFallbackAttempted(t *testing.T) {
	info := &RelayInfo{
		OriginModelName:        "gpt-5.5",
		ModelFallbackAttempted: true,
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "qwen-3.6",
		},
	}

	require.Equal(t, "gpt-5.5", info.ResponseModel("qwen-3.6"))
}

func TestRelayInfoResponseModelUsesUpstreamWithoutFallback(t *testing.T) {
	info := &RelayInfo{
		OriginModelName: "gpt-5.5",
		ChannelMeta: &ChannelMeta{
			UpstreamModelName: "qwen-3.6",
		},
	}

	require.Equal(t, "qwen-3.6", info.ResponseModel("qwen-3.6"))
}
