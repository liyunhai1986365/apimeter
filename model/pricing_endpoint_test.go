package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseEndpointInfoMapPreservesDocsURLAndLabel(t *testing.T) {
	endpoints := parseEndpointInfoMap(`{
		"openai-image-edit": {
			"path": "/v1/images/edits",
			"method": "post",
			"label": "OpenAI Image Edit",
			"docs_url": "https://docs.example.com/openai-image-edit"
		},
		"legacy-image": "/v1/images/generations"
	}`)

	require.Equal(t, "/v1/images/edits", endpoints["openai-image-edit"].Path)
	require.Equal(t, "POST", endpoints["openai-image-edit"].Method)
	require.Equal(t, "OpenAI Image Edit", endpoints["openai-image-edit"].Label)
	require.Equal(t, "https://docs.example.com/openai-image-edit", endpoints["openai-image-edit"].DocsURL)

	require.Equal(t, "/v1/images/generations", endpoints["legacy-image"].Path)
	require.Equal(t, "POST", endpoints["legacy-image"].Method)
}
