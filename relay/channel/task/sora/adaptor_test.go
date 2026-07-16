package sora

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultUsesCompletedVideoURL(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"task_upstream",
		"status":"completed",
		"video_url":"https://cdn.example.com/video.mp4"
	}`))

	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	require.Equal(t, "https://cdn.example.com/video.mp4", result.Url)
}

func TestParseTaskResultLeavesCompletedURLBlankWhenMissing(t *testing.T) {
	adaptor := &TaskAdaptor{}

	result, err := adaptor.ParseTaskResult([]byte(`{
		"id":"task_upstream",
		"status":"completed"
	}`))

	require.NoError(t, err)
	require.Equal(t, model.TaskStatusSuccess, result.Status)
	require.Empty(t, result.Url)
}
