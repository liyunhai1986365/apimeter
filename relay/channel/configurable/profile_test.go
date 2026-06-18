package configurable

import "testing"

func TestListProfilesLoadsEmbeddedProfiles(t *testing.T) {
	profiles := ListProfiles()
	if len(profiles) == 0 {
		_, err := loadProfiles()
		t.Fatalf("expected embedded profiles, load error: %v", err)
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

	seedance, ok := GetProfile("doubao-seedance-2")
	if !ok {
		t.Fatal("expected doubao-seedance-2 profile")
	}
	if seedance.Submit.Path != "/api/v3/contents/generations/tasks" {
		t.Fatalf("unexpected seedance submit path: %s", seedance.Submit.Path)
	}
	if seedance.Fetch.Response.ResultURLPath != "content.video_url" {
		t.Fatalf("unexpected seedance result url path: %s", seedance.Fetch.Response.ResultURLPath)
	}
	if len(seedance.Resources) != 4 {
		t.Fatalf("expected seedance material resources, got %d", len(seedance.Resources))
	}
	createAsset, ok := seedance.ResourceByID("material_assets")
	if !ok {
		t.Fatal("expected seedance material_assets resource")
	}
	if createAsset.Public.Path != "/material/assets" {
		t.Fatalf("unexpected create asset public path: %s", createAsset.Public.Path)
	}
	if len(createAsset.Aliases) != 2 || createAsset.Aliases[0].Path != "/api/assets/upload" || createAsset.Aliases[1].Path != "/api/assets" {
		t.Fatalf("unexpected create asset aliases: %#v", createAsset.Aliases)
	}
	if createAsset.Upstream.Path != "/material/assets" {
		t.Fatalf("unexpected create asset upstream path: %s", createAsset.Upstream.Path)
	}
	if createAsset.Response.Passthrough {
		t.Fatal("expected create asset response to be normalized")
	}
	if len(createAsset.Response.Fields) == 0 {
		t.Fatal("expected create asset response fields")
	}
	detailAsset, ok := seedance.ResourceByID("material_asset_detail")
	if !ok {
		t.Fatal("expected seedance material_asset_detail resource")
	}
	if detailAsset.Public.Path != "/material/assets/{asset_id}" {
		t.Fatalf("unexpected detail asset public path: %s", detailAsset.Public.Path)
	}
	if len(detailAsset.Aliases) != 1 || detailAsset.Aliases[0].Path != "/api/assets/{id}" {
		t.Fatalf("unexpected detail asset aliases: %#v", detailAsset.Aliases)
	}
	if detailAsset.PathParams["asset_id"] != "id" {
		t.Fatalf("unexpected detail asset path params: %#v", detailAsset.PathParams)
	}
	listAsset, ok := seedance.ResourceByID("material_assets_list")
	if !ok {
		t.Fatal("expected seedance material_assets_list resource")
	}
	if listAsset.Public.Path != "/material/assets" || len(listAsset.Aliases) != 1 || listAsset.Aliases[0].Path != "/api/assets" {
		t.Fatalf("unexpected list asset routes: public=%s aliases=%#v", listAsset.Public.Path, listAsset.Aliases)
	}
	deleteAsset, ok := seedance.ResourceByID("material_asset_delete")
	if !ok {
		t.Fatal("expected seedance material_asset_delete resource")
	}
	if deleteAsset.Public.Path != "/material/assets/{asset_id}" || deleteAsset.Upstream.Path != "/material/assets/{asset_id}" {
		t.Fatalf("unexpected delete asset paths: public=%s upstream=%s", deleteAsset.Public.Path, deleteAsset.Upstream.Path)
	}
	if deleteAsset.PathParams["asset_id"] != "id" {
		t.Fatalf("unexpected delete asset path params: %#v", deleteAsset.PathParams)
	}

	seedanceAPIAssets, ok := GetProfile("doubao-seedance-2-api-assets")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets profile")
	}
	uploadAsset, ok := seedanceAPIAssets.ResourceByID("assets_upload")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets assets_upload resource")
	}
	if uploadAsset.Public.Path != "/api/assets/upload" || uploadAsset.Upstream.Path != "/api/assets/upload" {
		t.Fatalf("unexpected api assets upload paths: public=%s upstream=%s", uploadAsset.Public.Path, uploadAsset.Upstream.Path)
	}
	listAPIAssets, ok := seedanceAPIAssets.ResourceByID("assets_list")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets assets_list resource")
	}
	if listAPIAssets.Public.Path != "/api/assets" || listAPIAssets.Upstream.Path != "/api/assets" {
		t.Fatalf("unexpected api assets list paths: public=%s upstream=%s", listAPIAssets.Public.Path, listAPIAssets.Upstream.Path)
	}
	detailAPIAsset, ok := seedanceAPIAssets.ResourceByID("asset_detail")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets asset_detail resource")
	}
	if detailAPIAsset.Public.Path != "/api/assets/{id}" || detailAPIAsset.Upstream.Path != "/api/assets/{id}" {
		t.Fatalf("unexpected api assets detail paths: public=%s upstream=%s", detailAPIAsset.Public.Path, detailAPIAsset.Upstream.Path)
	}
	deleteAPIAsset, ok := seedanceAPIAssets.ResourceByID("asset_delete")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets asset_delete resource")
	}
	if deleteAPIAsset.Public.Path != "/api/assets/{id}" || deleteAPIAsset.Upstream.Path != "/api/assets/{id}" {
		t.Fatalf("unexpected api assets delete paths: public=%s upstream=%s", deleteAPIAsset.Public.Path, deleteAPIAsset.Upstream.Path)
	}
}
