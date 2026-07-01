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
	if uploadAsset.Public.Path != "/api/assets/upload" || uploadAsset.Upstream.Path != "/material/assets" {
		t.Fatalf("unexpected api assets upload paths: public=%s upstream=%s", uploadAsset.Public.Path, uploadAsset.Upstream.Path)
	}
	listAPIAssets, ok := seedanceAPIAssets.ResourceByID("assets_list")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets assets_list resource")
	}
	if listAPIAssets.Public.Path != "/api/assets" || listAPIAssets.Upstream.Path != "/material/assets" {
		t.Fatalf("unexpected api assets list paths: public=%s upstream=%s", listAPIAssets.Public.Path, listAPIAssets.Upstream.Path)
	}
	detailAPIAsset, ok := seedanceAPIAssets.ResourceByID("asset_detail")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets asset_detail resource")
	}
	if detailAPIAsset.Public.Path != "/api/assets/{id}" || detailAPIAsset.Upstream.Path != "/material/assets/{asset_id}" {
		t.Fatalf("unexpected api assets detail paths: public=%s upstream=%s", detailAPIAsset.Public.Path, detailAPIAsset.Upstream.Path)
	}
	if detailAPIAsset.PathParams["asset_id"] != "id" {
		t.Fatalf("unexpected api assets detail path params: %#v", detailAPIAsset.PathParams)
	}
	deleteAPIAsset, ok := seedanceAPIAssets.ResourceByID("asset_delete")
	if !ok {
		t.Fatal("expected doubao-seedance-2-api-assets asset_delete resource")
	}
	if deleteAPIAsset.Public.Path != "/api/assets/{id}" || deleteAPIAsset.Upstream.Path != "/material/assets/{asset_id}" {
		t.Fatalf("unexpected api assets delete paths: public=%s upstream=%s", deleteAPIAsset.Public.Path, deleteAPIAsset.Upstream.Path)
	}
	if deleteAPIAsset.PathParams["asset_id"] != "id" {
		t.Fatalf("unexpected api assets delete path params: %#v", deleteAPIAsset.PathParams)
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
	if len(serviceInference.Resources) != 5 {
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
	assetUpload, ok := serviceInference.ResourceByID("assets_upload")
	if !ok {
		t.Fatal("expected service inference assets_upload resource")
	}
	if assetUpload.Public.Path != "/api/assets/upload" || assetUpload.Upstream.Path != "/v1/assets" {
		t.Fatalf("unexpected assets upload paths: public=%s upstream=%s", assetUpload.Public.Path, assetUpload.Upstream.Path)
	}
	if len(assetUpload.PreRequests) != 1 || assetUpload.PreRequests[0].ID != "asset_group" || assetUpload.PreRequests[0].Upstream.Path != "/v1/asset-groups" {
		t.Fatalf("unexpected service inference assets upload pre requests: %#v", assetUpload.PreRequests)
	}
	if assetUpload.Response.Passthrough || len(assetUpload.Response.Fields) == 0 {
		t.Fatalf("expected service inference assets upload response mapping: %#v", assetUpload.Response)
	}
	assetCreate, ok := serviceInference.ResourceByID("assets_create")
	if !ok {
		t.Fatal("expected service inference assets_create resource")
	}
	if assetCreate.Public.Path != "/v1/assets" || assetCreate.Upstream.Path != "/v1/assets" {
		t.Fatalf("unexpected assets create paths: public=%s upstream=%s", assetCreate.Public.Path, assetCreate.Upstream.Path)
	}
	if len(assetCreate.Aliases) != 1 || assetCreate.Aliases[0].Path != "/api/assets" {
		t.Fatalf("unexpected service inference assets create aliases: %#v", assetCreate.Aliases)
	}
	if len(assetCreate.PreRequests) != 1 || assetCreate.PreRequests[0].ID != "asset_group" || assetCreate.PreRequests[0].Upstream.Path != "/v1/asset-groups" {
		t.Fatalf("unexpected service inference assets create pre requests: %#v", assetCreate.PreRequests)
	}
	if assetCreate.PreRequests[0].SkipIfPresent != "body.group_id" {
		t.Fatalf("unexpected service inference assets create pre request skip condition: %#v", assetCreate.PreRequests[0])
	}
	if assetCreate.PreRequests[0].ManagedState == nil || assetCreate.PreRequests[0].ManagedState.Key != "asset_group_id" || assetCreate.PreRequests[0].ManagedState.Validate.Path != "/v1/asset-groups/{group_id}" {
		t.Fatalf("unexpected service inference assets create managed state: %#v", assetCreate.PreRequests[0].ManagedState)
	}
	assetGet, ok := serviceInference.ResourceByID("assets_get")
	if !ok {
		t.Fatal("expected service inference assets_get resource")
	}
	if assetGet.Public.Path != "/v1/assets/get" || assetGet.Upstream.Path != "/v1/assets/get" {
		t.Fatalf("unexpected assets get paths: public=%s upstream=%s", assetGet.Public.Path, assetGet.Upstream.Path)
	}
	if len(assetGet.Aliases) != 1 || assetGet.Aliases[0].Method != "GET" || assetGet.Aliases[0].Path != "/api/assets/{id}" {
		t.Fatalf("unexpected service inference assets get aliases: %#v", assetGet.Aliases)
	}
	if assetGet.Response.Passthrough || len(assetGet.Response.Fields) == 0 {
		t.Fatalf("expected service inference assets get response mapping: %#v", assetGet.Response)
	}

	arkTaskAssets, ok := GetProfile("seedance2-ark-task-assets")
	if !ok {
		t.Fatal("expected seedance2-ark-task-assets profile")
	}
	if arkTaskAssets.Video == nil {
		t.Fatal("expected ark task assets video protocol")
	}
	if arkTaskAssets.Video.Native.Submit.Path != "/api/v3/contents/generations/tasks" {
		t.Fatalf("unexpected ark task assets native submit path: %s", arkTaskAssets.Video.Native.Submit.Path)
	}
	if arkTaskAssets.Video.Native.Fetch.Path != "/api/v3/contents/generations/tasks/{task_id}" {
		t.Fatalf("unexpected ark task assets native fetch path: %s", arkTaskAssets.Video.Native.Fetch.Path)
	}
	arkUpload, ok := arkTaskAssets.ResourceByID("assets_upload")
	if !ok {
		t.Fatal("expected ark task assets assets_upload resource")
	}
	if arkUpload.Public.Path != "/api/assets/upload" || arkUpload.Upstream.Path != "/v1/task/submit" {
		t.Fatalf("unexpected ark task assets upload paths: public=%s upstream=%s", arkUpload.Public.Path, arkUpload.Upstream.Path)
	}
	if len(arkUpload.Request.Fields) == 0 || !arkUpload.Response.Passthrough {
		t.Fatalf("expected ark task assets upload request mapping with passthrough response: %#v", arkUpload)
	}
	arkQuery, ok := arkTaskAssets.ResourceByID("asset_query")
	if !ok {
		t.Fatal("expected ark task assets asset_query resource")
	}
	if arkQuery.Public.Method != "GET" || arkQuery.Public.Path != "/api/assets/{id}" || arkQuery.Upstream.Method != "POST" || arkQuery.Upstream.Path != "/v1/task/submit" {
		t.Fatalf("unexpected ark task assets query paths: public=%s %s upstream=%s %s", arkQuery.Public.Method, arkQuery.Public.Path, arkQuery.Upstream.Method, arkQuery.Upstream.Path)
	}
	if len(arkQuery.Request.Fields) == 0 || !arkQuery.Response.Passthrough {
		t.Fatalf("expected ark task assets query request mapping with passthrough response: %#v", arkQuery)
	}

	kling, ok := GetProfile("kling-video")
	if !ok {
		t.Fatal("expected kling-video profile")
	}
	if kling.Video == nil {
		t.Fatal("expected kling video protocol")
	}
	if kling.Video.Submit.Path != "/v1/videos/text2video" {
		t.Fatalf("unexpected kling default submit path: %s", kling.Video.Submit.Path)
	}
	if kling.Video.Fetch.Path != "/v1/videos/text2video/{task_id}" {
		t.Fatalf("unexpected kling default fetch path: %s", kling.Video.Fetch.Path)
	}
	assertPathVariants(t, kling.Video.Submit, []string{
		"/image-to-video/kling-3.0-turbo",
		"/v1/videos/image2video",
		"/v1/videos/omni-video",
		"/v1/videos/motion-control",
		"/v1/videos/multi-image2video",
		"/v1/videos/video-extend",
		"/v1/videos/advanced-lip-sync",
		"/v1/videos/avatar/image2video",
		"/v1/audio/text-to-audio",
		"/v1/audio/video-to-audio",
	})
	assertPathVariants(t, kling.Video.Fetch, []string{
		"/tasks",
		"/v1/videos/image2video/{task_id}",
		"/v1/videos/omni-video/{task_id}",
		"/v1/videos/motion-control/{task_id}",
		"/v1/videos/multi-image2video/{task_id}",
		"/v1/videos/video-extend/{task_id}",
		"/v1/videos/advanced-lip-sync/{task_id}",
		"/v1/videos/avatar/image2video/{task_id}",
		"/v1/audio/text-to-audio/{task_id}",
		"/v1/audio/video-to-audio/{task_id}",
	})
	assertKlingResources(t, kling, []string{
		"turbo_text2video_create",
		"turbo_image2video_create",
		"turbo_tasks_query",
		"turbo_tasks_cursor",
		"text2video_create",
		"text2video_get",
		"text2video_list",
		"image2video_create",
		"image2video_get",
		"image2video_list",
		"omni_video_create",
		"omni_video_get",
		"omni_video_list",
		"motion_control_create",
		"motion_control_get",
		"motion_control_list",
		"multi_image2video_create",
		"multi_image2video_get",
		"multi_image2video_list",
		"multi_elements_init_selection",
		"multi_elements_add_selection",
		"multi_elements_delete_selection",
		"multi_elements_clear_selection",
		"multi_elements_preview_selection",
		"multi_elements_task",
		"multi_elements_get",
		"multi_elements_list",
		"video_extend_create",
		"video_extend_get",
		"video_extend_list",
		"identify_face",
		"lip_sync_create",
		"lip_sync_get",
		"lip_sync_list",
		"avatar_image2video_create",
		"avatar_image2video_get",
		"avatar_image2video_list",
		"tts",
		"text_to_audio_create",
		"text_to_audio_get",
		"text_to_audio_list",
		"video_to_audio_create",
		"video_to_audio_get",
		"video_to_audio_list",
		"advanced_custom_elements_create",
		"advanced_custom_elements_get",
		"advanced_presets_elements",
		"delete_advanced_elements",
		"custom_voices_create",
		"custom_voices_get",
		"custom_voices_list",
		"presets_voices_list",
		"delete_voices",
		"image_recognize",
	})
	assertKlingResourceModels(t, kling, map[string]string{
		"turbo_text2video_create":         "kling-3.0-turbo",
		"turbo_image2video_create":        "kling-3.0-turbo",
		"video_extend_create":             "kling_extend",
		"video_extend_get":                "kling_extend",
		"video_extend_list":               "kling_extend",
		"identify_face":                   "kling_image_recognize",
		"avatar_image2video_create":       "kling_avatar_image2video",
		"avatar_image2video_get":          "kling_avatar_image2video",
		"avatar_image2video_list":         "kling_avatar_image2video",
		"text_to_audio_create":            "kling_audio_text_to_audio",
		"text_to_audio_get":               "kling_audio_text_to_audio",
		"text_to_audio_list":              "kling_audio_text_to_audio",
		"video_to_audio_create":           "kling_audio_video_to_audio",
		"video_to_audio_get":              "kling_audio_video_to_audio",
		"video_to_audio_list":             "kling_audio_video_to_audio",
		"advanced_custom_elements_create": "kling_multi_elements_submit",
		"advanced_custom_elements_get":    "kling_multi_elements_submit",
		"advanced_presets_elements":       "kling_multi_elements_submit",
		"delete_advanced_elements":        "kling_multi_elements_submit",
		"image_recognize":                 "kling_image_recognize",
	})
	if kling.Video.Fetch.Response.ResultURLPath != "data.task_result.videos.0.url" {
		t.Fatalf("unexpected kling result url path: %s", kling.Video.Fetch.Response.ResultURLPath)
	}
	var statusField *FieldMapping
	for i := range assetGet.Response.Fields {
		if assetGet.Response.Fields[i].To == "data.Status" {
			statusField = &assetGet.Response.Fields[i]
			break
		}
	}
	if statusField == nil || statusField.ValueMap["completed"] != "Active" {
		t.Fatalf("unexpected service inference assets get status map: %#v", assetGet.Response.Fields)
	}
}

func assertPathVariants(t *testing.T, endpoint EndpointConfig, expected []string) {
	t.Helper()
	seen := map[string]bool{}
	for _, variant := range endpoint.PathVariants {
		seen[variant.Path] = true
	}
	for _, path := range expected {
		if !seen[path] {
			t.Fatalf("expected path variant %s in %#v", path, endpoint.PathVariants)
		}
	}
}

func assertKlingResources(t *testing.T, profile *Profile, expected []string) {
	t.Helper()
	for _, id := range expected {
		resource, ok := profile.ResourceByID(id)
		if !ok {
			t.Fatalf("expected kling resource %s", id)
		}
		if resource.Public.Method == "" || resource.Public.Path == "" {
			t.Fatalf("expected kling resource %s to expose a public endpoint: %#v", id, resource)
		}
		if resource.Upstream.Method == "" || resource.Upstream.Path == "" {
			t.Fatalf("expected kling resource %s to map an upstream endpoint: %#v", id, resource)
		}
		if !resource.Response.Passthrough {
			t.Fatalf("expected kling resource %s to passthrough response", id)
		}
	}
}

func assertKlingResourceModels(t *testing.T, profile *Profile, expected map[string]string) {
	t.Helper()
	for id, modelName := range expected {
		resource, ok := profile.ResourceByID(id)
		if !ok {
			t.Fatalf("expected kling resource %s", id)
		}
		if resource.Model != modelName {
			t.Fatalf("expected kling resource %s model %s, got %s", id, modelName, resource.Model)
		}
		if !resource.Billing.Enabled {
			t.Fatalf("expected kling resource %s billing to be enabled", id)
		}
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
