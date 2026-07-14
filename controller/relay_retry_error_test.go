package controller

import (
	"errors"
	"net/http"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestChannelSelectionFailurePreservesLastRelayError(t *testing.T) {
	lastError := types.NewOpenAIError(
		errors.New("upstream image conversion failed"),
		types.ErrorCodeConvertRequestFailed,
		http.StatusInternalServerError,
	)
	channelError := types.NewError(
		errors.New("no available channel on retry"),
		types.ErrorCodeGetChannelFailed,
	)
	info := &relaycommon.RelayInfo{LastError: lastError}

	require.Same(t, lastError, relayErrorAfterChannelSelectionFailure(info, channelError))
}

func TestChannelSelectionFailureUsesSelectionErrorWithoutPreviousFailure(t *testing.T) {
	channelError := types.NewError(
		errors.New("no available channel"),
		types.ErrorCodeGetChannelFailed,
	)

	require.Same(t, channelError, relayErrorAfterChannelSelectionFailure(&relaycommon.RelayInfo{}, channelError))
}
