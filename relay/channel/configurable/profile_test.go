package configurable

import "testing"

func TestListProfilesLoadsEmbeddedProfiles(t *testing.T) {
	profiles := ListProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected embedded profiles")
	}

	profile, ok := GetProfile("generic-video-json")
	if !ok {
		t.Fatal("expected generic-video-json profile")
	}
	if profile.ID != "generic-video-json" {
		t.Fatalf("unexpected profile id: %s", profile.ID)
	}
	if profile.Submit.Path != "/v1/videos" {
		t.Fatalf("unexpected submit path: %s", profile.Submit.Path)
	}
	if profile.Submit.Response.TaskIDPath != "id" {
		t.Fatalf("unexpected submit task id path: %s", profile.Submit.Response.TaskIDPath)
	}
	if profile.Fetch.Response.StatusMap["completed"] != "SUCCESS" {
		t.Fatalf("unexpected completed status mapping")
	}
}
