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
	if profile.Video == nil {
		t.Fatal("expected generic-video-json video protocol")
	}
	if profile.Video.Submit.Path != "/v1/videos" {
		t.Fatalf("unexpected submit path: %s", profile.Video.Submit.Path)
	}
	if profile.Video.Submit.Response.TaskIDPath != "id" {
		t.Fatalf("unexpected submit task id path: %s", profile.Video.Submit.Response.TaskIDPath)
	}
	if profile.Video.Fetch.Response.StatusMap["completed"] != "SUCCESS" {
		t.Fatalf("unexpected completed status mapping")
	}

	seedance, ok := GetProfile("doubao-seedance-2")
	if !ok {
		t.Fatal("expected doubao-seedance-2 profile")
	}
	if seedance.Video == nil {
		t.Fatal("expected seedance video protocol")
	}
	if seedance.Video.Submit.Path != "/api/v3/contents/generations/tasks" {
		t.Fatalf("unexpected seedance submit path: %s", seedance.Video.Submit.Path)
	}
	if seedance.Video.Fetch.Response.ResultURLPath != "content.video_url" {
		t.Fatalf("unexpected seedance result url path: %s", seedance.Video.Fetch.Response.ResultURLPath)
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

	serviceInference, ok := GetProfile("seedance2-service-inference")
	if !ok {
		t.Fatal("expected seedance2-service-inference profile")
	}
	if serviceInference.Video == nil {
		t.Fatal("expected service inference video protocol")
	}
	if serviceInference.Video.Native.Submit.Path != "/api/v3/contents/generations/tasks" {
		t.Fatalf("unexpected service inference native submit path: %s", serviceInference.Video.Native.Submit.Path)
	}
	if serviceInference.Video.Submit.Path != "/v1/video/generate" {
		t.Fatalf("unexpected service inference upstream submit path: %s", serviceInference.Video.Submit.Path)
	}
	if serviceInference.Video.Native.Fetch.Path != "/api/v3/contents/generations/tasks/{task_id}" {
		t.Fatalf("unexpected service inference native fetch path: %s", serviceInference.Video.Native.Fetch.Path)
	}
	if serviceInference.Video.Fetch.Path != "/v1/video/tasks/{task_id}" {
		t.Fatalf("unexpected service inference upstream fetch path: %s", serviceInference.Video.Fetch.Path)
	}
	if len(serviceInference.Resources) != 4 {
		t.Fatalf("expected service inference asset resources, got %d", len(serviceInference.Resources))
	}
	groupCreate, ok := serviceInference.ResourceByID("asset_groups_create")
	if !ok {
		t.Fatal("expected service inference asset_groups_create resource")
	}
	if groupCreate.Public.Path != "/v1/asset-groups" || groupCreate.Upstream.Path != "/v1/asset-groups" {
		t.Fatalf("unexpected asset group create paths: public=%s upstream=%s", groupCreate.Public.Path, groupCreate.Upstream.Path)
	}
	groupDetail, ok := serviceInference.ResourceByID("asset_group_detail")
	if !ok {
		t.Fatal("expected service inference asset_group_detail resource")
	}
	if groupDetail.Public.Path != "/v1/asset-groups/{group_id}" || groupDetail.Upstream.Path != "/v1/asset-groups/{group_id}" {
		t.Fatalf("unexpected asset group detail paths: public=%s upstream=%s", groupDetail.Public.Path, groupDetail.Upstream.Path)
	}
	assetCreate, ok := serviceInference.ResourceByID("assets_create")
	if !ok {
		t.Fatal("expected service inference assets_create resource")
	}
	if assetCreate.Public.Path != "/v1/assets" || assetCreate.Upstream.Path != "/v1/assets" {
		t.Fatalf("unexpected assets create paths: public=%s upstream=%s", assetCreate.Public.Path, assetCreate.Upstream.Path)
	}
	assetGet, ok := serviceInference.ResourceByID("assets_get")
	if !ok {
		t.Fatal("expected service inference assets_get resource")
	}
	if assetGet.Public.Path != "/v1/assets/get" || assetGet.Upstream.Path != "/v1/assets/get" {
		t.Fatalf("unexpected assets get paths: public=%s upstream=%s", assetGet.Public.Path, assetGet.Upstream.Path)
	}
}

func TestVideoProfilesUseVideoProtocolConfig(t *testing.T) {
	for _, profile := range ListProfiles() {
		if profile.MediaType != "video" {
			continue
		}
		if profile.Video == nil {
			t.Fatalf("video profile %s must use video protocol config", profile.ID)
		}
		if profile.Video.Submit.Path == "" {
			t.Fatalf("video profile %s missing video submit path", profile.ID)
		}
		if profile.Video.Fetch.Path == "" {
			t.Fatalf("video profile %s missing video fetch path", profile.ID)
		}
		if profile.Submit.Path != "" || profile.Fetch.Path != "" || profile.Native.Submit.Path != "" || profile.Native.Fetch.Path != "" {
			t.Fatalf("video profile %s should keep video protocol under video block only", profile.ID)
		}
	}
}
