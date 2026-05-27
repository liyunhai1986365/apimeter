package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func TestShouldUseDuomiImageAsyncForImageGenerations(t *testing.T) {
	require.True(t, ShouldUseDuomiImageAsync("https://duomiapi.com", "/v1/images/generations"))
}

func TestDuomiGeminiImageEndpointSelection(t *testing.T) {
	require.True(t, IsDuomiGeminiImageModel("gemini-3-pro-image-preview"))
	require.True(t, IsDuomiGeminiImageModel("gemini-3.1-flash-image-preview"))
	require.True(t, IsDuomiGeminiImageModel("gemini-2.5-pro-image-preview"))
	require.True(t, IsDuomiGeminiImageModel("gemini-2.5-flash-image"))
	require.True(t, IsDuomiGeminiImageModel("gemini-2.5-flash-image-preview"))
	require.False(t, IsDuomiGeminiImageModel("gpt-image-1"))

	requestURL, ok := DuomiGeminiImageRequestPath("https://duomiapi.com", "/v1/images/generations", "gemini-3-pro-image-preview", false)
	require.True(t, ok)
	require.Equal(t, "/api/gemini/nano-banana", requestURL)

	requestURL, ok = DuomiGeminiImageRequestPath("https://duomiapi.com", "/v1/images/generations", "gemini-3.1-flash-image-preview", true)
	require.True(t, ok)
	require.Equal(t, "/api/gemini/nano-banana-edit", requestURL)

	requestURL, ok = DuomiGeminiImageRequestPath("https://duomiapi.com", "/v1/images/generations", "gemini-2.5-flash-image-preview", true)
	require.True(t, ok)
	require.Equal(t, "/api/gemini/nano-banana-edit", requestURL)

	requestURL, ok = DuomiGeminiImageRequestPath("https://duomiapi.com", "/v1/images/generations", "gemini-2.5-flash-image", true)
	require.True(t, ok)
	require.Equal(t, "/api/gemini/nano-banana-edit", requestURL)

	_, ok = DuomiGeminiImageRequestPath("https://example.com", "/v1/images/generations", "gemini-3-pro-image-preview", false)
	require.False(t, ok)
}

func TestWithAsyncQueryAppendsAsyncParameter(t *testing.T) {
	require.Equal(t, "/v1/images/generations?async=true", WithAsyncQuery("/v1/images/generations"))
	require.Equal(t, "/v1/images/generations?async=true&foo=bar", WithAsyncQuery("/v1/images/generations?foo=bar"))
	require.Equal(t, "/v1/images/generations?async=true", WithAsyncQuery("/v1/images/generations?async=true"))
}

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
