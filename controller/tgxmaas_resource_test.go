package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestTgxMaasResourceProtocol(t *testing.T) {
	cases := []struct{ id, method, path, body, response string }{
		{"asset_groups_create", "POST", "/v1/private-avatar/groups", `{"model":"doubao-seedance-2-5-260628","Name":"group","Description":"","extension":{"zero":0,"enabled":false}}`, `{"ResponseMetadata":{"Action":"CreateAssetGroup"},"Result":{"Id":"ag_local"}}`},
		{"asset_groups_list", "POST", "/v1/private-avatar/groups/list", `{"model":"doubao-seedance-2-5-260628","NextToken":"opaque+/=token","MaxResults":20,"Filter":{"GroupType":"AIGC"}}`, `{"ResponseMetadata":{"Action":"ListAssetGroups"},"Result":{"Items":[{"Id":"ag_local"}],"NextToken":"opaque+/=next"}}`},
		{"asset_groups_get", "GET", "/v1/private-avatar/groups/ag_local", "", `{"Result":{"Id":"ag_local","Name":"group"}}`},
		{"asset_groups_update", "PATCH", "/v1/private-avatar/groups/ag_local", `{"Name":"renamed","Description":""}`, `{"Result":{"Id":"ag_local"}}`},
		{"asset_groups_delete", "DELETE", "/v1/private-avatar/groups/ag_local", "", `{"Result":{}}`},
		{"assets_create", "POST", "/v1/private-avatar/assets", `{"model":"doubao-seedance-2-5-260628","GroupId":"ag_local","URL":"https://example.com/image.png","AssetType":"Image","Name":"asset"}`, `{"Result":{"Id":"asset_local","upstream_asset_id":"asset-official","GroupId":"ag_local","Status":"Processing"}}`},
		{"assets_list", "POST", "/v1/private-avatar/assets/list", `{"Filter":{"GroupIds":["ag_local"],"Statuses":["Active"]},"NextToken":"opaque-token"}`, `{"Result":{"Items":[{"Id":"asset_local","LastInferenceTime":"2026-09-15T00:00:00Z"}],"NextToken":"next"}}`},
		{"assets_get", "GET", "/v1/private-avatar/assets/asset_local", "", `{"Result":{"Id":"asset_local","Status":"Failed","Error":{"Code":"DownloadFailed","Message":"fixture"},"extension":9007199254740993}}`},
		{"assets_update", "PATCH", "/v1/private-avatar/assets/asset_local", `{"Name":"renamed"}`, `{"Result":{"Id":"asset_local"}}`},
		{"assets_delete", "DELETE", "/v1/private-avatar/assets/asset_local", "", `{"Result":{}}`},
		{"liveness_session_create", "POST", "/v1/real-avatar/auth/session", `{"model":"doubao-seedance-2-5-260628","CallbackURL":"https://client.example/callback","Lng":"zh"}`, `{"Result":{"H5Link":"https://provider.example/auth","BytedToken":"opaque-session"}}`},
		{"liveness_group_exchange", "POST", "/v1/real-avatar/groups/from-token", `{"model":"doubao-seedance-2-5-260628","BytedToken":"opaque-session"}`, `{"Result":{"Id":"ag_local_real","GroupType":"LivenessFace"}}`},
	}
	var calls atomic.Int32
	var current atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		tc := cases[current.Load()]
		require.Equal(t, tc.method, r.Method)
		require.Equal(t, tc.path, r.URL.Path)
		require.Equal(t, "Bearer tgx-test-secret", r.Header.Get("Authorization"))
		data, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		if tc.body != "" {
			require.JSONEq(t, tc.body, string(data))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tc.response))
	}))
	defer upstream.Close()
	r := tgxMaasResourceTestRouter(t, upstream.URL, "tgx-test-secret", "doubao-seedance-2-5-260628")
	profile, ok := configurable.GetProfile("seedance-tgxmaas")
	require.True(t, ok)
	for i, tc := range cases {
		current.Store(int32(i))
		t.Run(tc.id, func(t *testing.T) {
			response := seedanceCall(r, tc.method, tc.path, tc.body, 1)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Equal(t, tc.response, response.Body.String(), "response must retain provider IDs, extensions and numeric precision")
			resource, ok := profile.ResourceByID(tc.id)
			require.True(t, ok)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(tc.method, tc.path, nil)
			require.False(t, configurableResourceAllowsReplay(c, resource), "do not replay resource operations on another account")
		})
	}
	require.Equal(t, int32(12), calls.Load())
	// Authentication failures must not reach the supplier.
	request := httptest.NewRequest("POST", "/v1/private-avatar/assets", strings.NewReader(cases[5].body))
	rejected := httptest.NewRecorder()
	r.ServeHTTP(rejected, request)
	require.NotEqual(t, 200, rejected.Code)
	require.Equal(t, int32(12), calls.Load())
	var tasks int64
	require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
	require.Zero(t, tasks, "asset Processing is not a video billing task")
}

func tgxMaasResourceTestRouter(t *testing.T, upstream, key, modelName string) *gin.Engine {
	t.Helper()
	r := seedanceTestRouter(t, upstream, key, modelName)
	require.NoError(t, model.DB.AutoMigrate(&model.RetryRouteEvent{}, &model.ConfigurableResourceState{}))
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.Type = constant.ChannelTypeConfigurable
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
	require.NoError(t, model.DB.Save(ch).Error)
	profile, ok := configurable.GetProfile("seedance-tgxmaas")
	require.True(t, ok)
	require.Len(t, profile.Resources, 12)
	for _, resource := range profile.Resources {
		path := strings.ReplaceAll(strings.ReplaceAll(resource.Public.Path, "{group_id}", ":group_id"), "{asset_id}", ":asset_id")
		r.Handle(resource.Public.Method, path, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
	}

	return r
}

// Opt-in only. Creates one group and one image asset, then deletes only those IDs.
func TestTgxMaasAssetsLive(t *testing.T) {
	configPath := os.Getenv("TGXMAAS_ASSETS_LIVE_CONFIG")
	if configPath == "" {
		t.Skip("requires explicit TGXMAAS_ASSETS_LIVE_CONFIG")
	}
	var cfg struct {
		AssetBackend         string `json:"asset_backend"`
		AssetBaseURL         string `json:"asset_base_url"`
		AssetAccessKeyID     string `json:"asset_access_key_id"`
		AssetSecretAccessKey string `json:"asset_secret_access_key"`
		URL                  string `json:"url"`
		Key                  string `json:"key"`
		Model                string `json:"model"`
		AssetURL             string `json:"asset_url"`
		GenerateVideo        bool   `json:"generate_video"`
		OfficialActions      bool   `json:"official_actions"`
		ProjectName          string `json:"project_name"`
		ChannelProjectName   string `json:"channel_project_name"`
		PagePagination       bool   `json:"page_pagination"`
		VerifyOriginalID     bool   `json:"verify_original_id"`
	}
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(data, &cfg))
	require.NotEmpty(t, cfg.Model)
	require.NotEmpty(t, cfg.AssetURL)
	evidence := os.Getenv("TGXMAAS_ASSETS_LIVE_EVIDENCE")
	require.NotEmpty(t, evidence)
	require.NoError(t, os.MkdirAll(evidence, 0700))
	r := tgxMaasResourceTestRouter(t, cfg.URL, cfg.Key, cfg.Model)
	if cfg.ChannelProjectName != "" {
		ch, err := model.GetChannelById(20, true)
		require.NoError(t, err)
		setting := ch.GetSetting()
		setting.Protocol.ProjectName = cfg.ChannelProjectName
		ch.SetSetting(setting)
		require.NoError(t, model.DB.Save(ch).Error)
	}
	if cfg.AssetBackend != "" {
		require.Equal(t, configurable.OfficialAssetBackend, cfg.AssetBackend)
		require.True(t, cfg.OfficialActions, "official live test requires official_actions")
		require.NotEmpty(t, cfg.ChannelProjectName, "configure the authorized project on the channel")
		// This fixture uses an isolated test database, never production credentials.
		t.Setenv("CRYPTO_SECRET", "asset-live-fixture-only")
		ch, err := model.GetChannelById(20, true)
		require.NoError(t, err)
		setting := ch.GetSetting()
		setting.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: cfg.AssetBackend, BaseURL: cfg.AssetBaseURL, AuthMode: "aksk", Region: "cn-beijing"}
		ch.SetSetting(setting)
		require.NoError(t, prepareAssetCredentials(ch, nil, &model.AssetCredentials{AccessKeyID: cfg.AssetAccessKeyID, SecretAccessKey: cfg.AssetSecretAccessKey}))
		require.NoError(t, model.DB.Save(ch).Error)
	}
	if cfg.OfficialActions {
		registerTgxConversionTestRoutes(t, r)
	}
	call := func(label, method, path string, body any) *httptest.ResponseRecorder {
		if strings.HasPrefix(path, "/v1/private-avatar/") {
			mapped := map[string]any{}
			if original, ok := body.(map[string]any); ok {
				for key, value := range original {
					mapped[key] = value
				}
			}
			if cfg.ProjectName != "" {
				mapped["ProjectName"] = cfg.ProjectName
			}
			if cfg.PagePagination && strings.HasSuffix(path, "/list") {
				delete(mapped, "MaxResults")
				delete(mapped, "NextToken")
				mapped["PageNumber"], mapped["PageSize"] = 1, 10
			}
			body = mapped
		}
		if cfg.OfficialActions && strings.HasPrefix(path, "/v1/private-avatar/") {
			parts := strings.Split(strings.TrimPrefix(path, "/v1/private-avatar/"), "/")
			kind, plural := "Asset", "Assets"
			if parts[0] == "groups" {
				kind, plural = "AssetGroup", "AssetGroups"
			}
			action := ""
			switch method {
			case "POST":
				action = "Create" + kind
				if len(parts) == 2 && parts[1] == "list" {
					action = "List" + plural
				}
			case "GET":
				action = "Get" + kind
			case "PATCH":
				action = "Update" + kind
			case "DELETE":
				action = "Delete" + kind
			}
			mapped := map[string]any{}
			if original, ok := body.(map[string]any); ok {
				for key, value := range original {
					mapped[key] = value
				}
			}
			if len(parts) == 2 && parts[1] != "list" {
				mapped["Id"] = parts[1]
			}
			body = mapped
			method, path = "POST", "/?Action="+action+"&Version=2024-01-01"
		}
		raw := ""
		if body != nil {
			b, e := common.Marshal(body)
			require.NoError(t, e)
			raw = string(b)
		}
		if cfg.ChannelProjectName != "" && cfg.ProjectName == "" {
			require.False(t, gjson.Get(raw, "ProjectName").Exists(), "client must omit ProjectName to verify the channel default")
		}
		response := seedanceCall(r, method, path, raw, 1)
		require.NoError(t, os.WriteFile(filepath.Join(evidence, label+".json"), response.Body.Bytes(), 0600))
		t.Logf("%s HTTP %d", label, response.Code)
		return response
	}
	success := func(w *httptest.ResponseRecorder) {
		require.Equal(t, 200, w.Code, "see response evidence; no automatic creation retry")
		require.False(t, gjson.Get(w.Body.String(), "ResponseMetadata.Error").Exists(), "see response evidence")
	}
	name := fmt.Sprintf("codex-protocol-test-%d", time.Now().Unix())
	created := call("group-create", "POST", "/v1/private-avatar/groups", map[string]any{"model": cfg.Model, "Name": name, "Description": "Temporary protocol verification; delete after test"})
	success(created)
	groupID := gjson.Get(created.Body.String(), "Result.Id").String()
	require.NotEmpty(t, groupID)
	t.Logf("Created test group %s", groupID)
	groupDeleted := false
	videoMayBeRunning := false
	defer func() {
		if !groupDeleted && !videoMayBeRunning {
			w := call("group-cleanup", "DELETE", "/v1/private-avatar/groups/"+groupID, nil)
			if w.Code != 200 {
				t.Errorf("test group cleanup failed for %s (HTTP %d)", groupID, w.Code)
			}
		}
	}()
	fetched := call("group-get", "GET", "/v1/private-avatar/groups/"+groupID, nil)
	success(fetched)
	require.Equal(t, groupID, gjson.Get(fetched.Body.String(), "Result.Id").String())
	success(call("group-update", "PATCH", "/v1/private-avatar/groups/"+groupID, map[string]any{"Name": name + "-updated", "Description": "Updated by protocol test"}))
	fetched = call("group-get-updated", "GET", "/v1/private-avatar/groups/"+groupID, nil)
	success(fetched)
	require.Equal(t, name+"-updated", gjson.Get(fetched.Body.String(), "Result.Name").String())
	list := call("group-list", "POST", "/v1/private-avatar/groups/list", map[string]any{"model": cfg.Model, "Filter": map[string]any{"GroupIds": []string{groupID}, "GroupType": "AIGC"}, "MaxResults": 10})
	success(list)
	require.Contains(t, list.Body.String(), groupID)
	created = call("asset-create", "POST", "/v1/private-avatar/assets", map[string]any{"model": cfg.Model, "GroupId": groupID, "URL": cfg.AssetURL, "AssetType": "Image", "Name": name})
	success(created)
	assetID := gjson.Get(created.Body.String(), "Result.Id").String()
	require.NotEmpty(t, assetID)
	t.Logf("Created test asset %s", assetID)
	assetDeleted := false
	defer func() {
		if !assetDeleted && !videoMayBeRunning {
			w := call("asset-cleanup", "DELETE", "/v1/private-avatar/assets/"+assetID, nil)
			if w.Code != 200 {
				t.Errorf("test asset cleanup failed for %s (HTTP %d)", assetID, w.Code)
			}
		}
	}()
	active := false
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		fetched = call("asset-get", "GET", "/v1/private-avatar/assets/"+assetID, nil)
		success(fetched)
		require.Equal(t, assetID, gjson.Get(fetched.Body.String(), "Result.Id").String())
		status := gjson.Get(fetched.Body.String(), "Result.Status").String()
		t.Logf("Asset status %s", status)
		if status == "Active" {
			active = true
			break
		}
		require.NotEqual(t, "Failed", status, "see asset-get evidence")
		time.Sleep(3 * time.Second)
	}
	require.True(t, active, "asset did not become Active before deadline")
	if cfg.VerifyOriginalID {
		originalID := gjson.Get(fetched.Body.String(), "Result.upstream_asset_id").String()
		if originalID == "" && strings.HasPrefix(assetID, "asset-") {
			originalID = assetID
		}
		require.NotEmpty(t, originalID, "supplier did not return an original asset ID")
		original := call("asset-get-original-id", "GET", "/v1/private-avatar/assets/"+originalID, nil)
		success(original)
		require.Equal(t, "Active", gjson.Get(original.Body.String(), "Result.Status").String())
	}
	success(call("asset-update", "PATCH", "/v1/private-avatar/assets/"+assetID, map[string]any{"Name": name + "-updated"}))
	fetched = call("asset-get-updated", "GET", "/v1/private-avatar/assets/"+assetID, nil)
	success(fetched)
	require.Equal(t, name+"-updated", gjson.Get(fetched.Body.String(), "Result.Name").String())
	list = call("asset-list", "POST", "/v1/private-avatar/assets/list", map[string]any{"model": cfg.Model, "Filter": map[string]any{"GroupIds": []string{groupID}, "GroupType": "AIGC", "Statuses": []string{"Active"}}, "MaxResults": 10})
	success(list)
	require.Contains(t, list.Body.String(), assetID)

	if cfg.GenerateVideo {
		payload := map[string]any{"model": cfg.Model, "content": []map[string]any{
			{"type": "text", "text": "Keep the fruit arrangement from the reference image, static camera, subtle natural lighting changes."},
			{"type": "image_url", "image_url": map[string]string{"url": "asset://" + assetID}, "role": "first_frame"},
		}, "duration": 4, "resolution": "480p", "generate_audio": false}
		raw, err := common.Marshal(payload)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(evidence, "video-request.json"), raw, 0600))
		videoMayBeRunning = true
		videoCreated := call("video-create", "POST", "/api/v3/contents/generations/tasks", payload)
		if videoCreated.Code >= 400 && videoCreated.Code < 500 {
			videoMayBeRunning = false
		}
		success(videoCreated)
		videoID := gjson.Get(videoCreated.Body.String(), "id").String()
		require.True(t, strings.HasPrefix(videoID, "cgt-"), "video accepted; retain test asset if task identity is unavailable")
		t.Logf("Created exactly one asset-reference video %s", videoID)
		videoDeadline := time.Now().Add(12 * time.Minute)
		for time.Now().Before(videoDeadline) {
			videoFetched := call("video-get", "GET", "/api/v3/contents/generations/tasks/"+videoID, nil)
			if videoFetched.Code == 429 || (videoFetched.Code == 503 && gjson.Get(videoFetched.Body.String(), "code").String() == "task_status_pending") {
				time.Sleep(3 * time.Second)
				continue
			}
			success(videoFetched)
			require.Equal(t, videoID, gjson.Get(videoFetched.Body.String(), "id").String())
			status := gjson.Get(videoFetched.Body.String(), "status").String()
			t.Logf("Video status %s", status)
			if status == "failed" || status == "expired" || status == "cancelled" {
				videoMayBeRunning = false
				t.Fatalf("asset-reference video ended as %s; see video-get evidence", status)
			}
			if status == "succeeded" {
				videoMayBeRunning = false
				url := gjson.Get(videoFetched.Body.String(), "content.video_url").String()
				require.NotEmpty(t, url)
				client := &http.Client{Timeout: 60 * time.Second}
				response, err := client.Get(url)
				require.NoError(t, err)
				content, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
				_ = response.Body.Close()
				require.NoError(t, err)
				require.Equal(t, 200, response.StatusCode)
				require.True(t, bytes.Contains(content[:min(len(content), 64)], []byte("ftyp")))
				require.NoError(t, os.WriteFile(filepath.Join(evidence, "video.bin"), content, 0600))
				require.True(t, gjson.Get(videoFetched.Body.String(), "usage").IsObject())
				var before, after model.User
				require.NoError(t, model.DB.First(&before, 1).Error)
				success(call("video-repeat", "GET", "/api/v3/contents/generations/tasks/"+videoID, nil))
				require.NoError(t, model.DB.First(&after, 1).Error)
				require.Equal(t, before.Quota, after.Quota)
				t.Logf("Asset-reference video passed; bytes=%d usage=%s", len(content), gjson.Get(videoFetched.Body.String(), "usage").Raw)
				break
			}
			time.Sleep(5 * time.Second)
		}
		require.False(t, videoMayBeRunning, "video still pending; retain this test group and asset, do not resubmit")
	}
	deleted := call("asset-delete", "DELETE", "/v1/private-avatar/assets/"+assetID, nil)
	success(deleted)
	assetDeleted = true
	deleted = call("group-delete", "DELETE", "/v1/private-avatar/groups/"+groupID, nil)
	success(deleted)
	groupDeleted = true
	t.Log("All 10 ordinary asset/group operations passed; created test resources deleted; liveness not executed")
}
