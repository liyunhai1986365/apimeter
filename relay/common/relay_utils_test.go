package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestGetFullRequestURLKeepsDuomiImageGenerationsUnchanged(t *testing.T) {
	got := GetFullRequestURL("https://duomiapi.com", "/v1/images/generations", constant.ChannelTypeOpenAI)
	require.Equal(t, "https://duomiapi.com/v1/images/generations", got)
}

func TestGetFullRequestURLKeepsNonDuomiImageGenerationsUnchanged(t *testing.T) {
	got := GetFullRequestURL("https://example.com", "/v1/images/generations", constant.ChannelTypeOpenAI)
	require.Equal(t, "https://example.com/v1/images/generations", got)
}

func TestGetFullRequestURLKeepsDuomiOtherPathsUnchanged(t *testing.T) {
	got := GetFullRequestURL("https://duomiapi.com", "/v1/tasks/abc", constant.ChannelTypeOpenAI)
	require.Equal(t, "https://duomiapi.com/v1/tasks/abc", got)
}
