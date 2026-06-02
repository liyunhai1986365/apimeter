package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageWithRequestIdRemovesExistingRequestIds(t *testing.T) {
	message := "status_code=400, ***.***.content.0: Invalid `signature` in `thinking` block (request id: upstream-a) (request id: upstream-b)"

	got := MessageWithRequestId(message, "system-request")

	require.Equal(t, "status_code=400, ***.***.content.0: Invalid `signature` in `thinking` block (request id: system-request)", got)
}

func TestMessageWithRequestIdPreservesBodyTextAroundRequestIdLikeContent(t *testing.T) {
	message := "bad response: value (request id: upstream-a) is invalid"

	got := MessageWithRequestId(message, "system-request")

	require.Equal(t, "bad response: value is invalid (request id: system-request)", got)
}
