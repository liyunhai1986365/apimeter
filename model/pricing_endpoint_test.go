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

func TestParseEndpointInfoMapPreservesConfig(t *testing.T) {
	endpoints := parseEndpointInfoMap(`{
		"seedance2-native-video": {
			"path": "/api/v3/contents/generations/tasks",
			"method": "POST",
			"label": "Seedance2 Native Video",
			"config": {
				"protocol_profile_id": "seedance2-service-inference",
				"mode": "native_video",
				"submit_path": "/api/v3/contents/generations/tasks",
				"fetch_path": "/api/v3/contents/generations/tasks/{task_id}",
				"path_params": ["task_id"],
				"parameters": {
					"resolution": {"type": "string", "enum": ["480p", "720p"]},
					"duration": {"type": "integer", "default": 4}
				}
			}
		}
	}`)

	config := endpoints["seedance2-native-video"].Config
	require.Equal(t, "seedance2-service-inference", config["protocol_profile_id"])
	require.Equal(t, "native_video", config["mode"])
	require.Equal(t, "/api/v3/contents/generations/tasks", config["submit_path"])
	require.Equal(t, "/api/v3/contents/generations/tasks/{task_id}", config["fetch_path"])
	require.Equal(t, []interface{}{"task_id"}, config["path_params"])

	parameters, ok := config["parameters"].(map[string]interface{})
	require.True(t, ok)
	resolution, ok := parameters["resolution"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, []interface{}{"480p", "720p"}, resolution["enum"])
}
