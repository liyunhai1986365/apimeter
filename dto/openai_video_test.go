package dto

import "testing"

func TestOpenAIVideoSetMetadataURLAlsoSetsVideoURL(t *testing.T) {
	video := NewOpenAIVideo()
	video.SetMetadata("url", "https://example.com/video.mp4")

	if video.VideoURL != "https://example.com/video.mp4" {
		t.Fatalf("unexpected video url: %s", video.VideoURL)
	}
	if got, _ := video.Metadata["url"].(string); got != "https://example.com/video.mp4" {
		t.Fatalf("unexpected metadata url: %s", got)
	}
}
