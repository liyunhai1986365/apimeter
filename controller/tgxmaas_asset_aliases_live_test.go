package controller

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Explicit opt-in: creates two groups and two image assets, then removes only
// those resources. No video generation or existing-resource mutations.
func TestTgxMaasAssetAliasesLive(t *testing.T) {
	config := os.Getenv("TGXMAAS_ALIASES_LIVE_CONFIG")
	if config == "" {
		t.Skip("requires TGXMAAS_ALIASES_LIVE_CONFIG")
	}
	var cfg struct {
		URL, Key, Model string
		AssetURL        string `json:"asset_url"`
	}
	raw, err := os.ReadFile(config)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(raw, &cfg))
	require.NotEmpty(t, cfg.URL)
	require.NotEmpty(t, cfg.Key)
	require.NotEmpty(t, cfg.Model)
	require.NotEmpty(t, cfg.AssetURL)
	evidence := os.Getenv("TGXMAAS_ALIASES_LIVE_EVIDENCE")
	require.NotEmpty(t, evidence)
	require.NoError(t, os.MkdirAll(evidence, 0700))
	r := tgxMaasResourceTestRouter(t, cfg.URL, cfg.Key, cfg.Model)
	registerTgxConversionTestRoutes(t, r)
	seq := 0
	call := func(label, method, path string, body any) *httptest.ResponseRecorder {
		payload := ""
		if body != nil {
			data, e := common.Marshal(body)
			require.NoError(t, e)
			payload = string(data)
		}
		response := seedanceCall(r, method, path, payload, 1)
		seq++
		require.NoError(t, os.WriteFile(filepath.Join(evidence, fmt.Sprintf("%03d-%s.json", seq, label)), response.Body.Bytes(), 0600))
		t.Logf("%s %s %s HTTP %d", label, method, path, response.Code)
		return response
	}
	success := func(t *testing.T, w *httptest.ResponseRecorder) {
		require.Equal(t, 200, w.Code, "see private response evidence")
		require.False(t, gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error").Exists(), "see private response evidence")
	}
	for i, prefix := range []string{"/api", "/v1"} {
		t.Run(strings.TrimPrefix(prefix, "/"), func(t *testing.T) {
			name := fmt.Sprintf("codex-alias-check-%d-%d", time.Now().Unix(), i)
			group := call("group-create", "POST", prefix+"/asset-groups", map[string]any{"model": cfg.Model, "name": name})
			success(t, group)
			groupID := gjson.GetBytes(group.Body.Bytes(), "Result.Id").String()
			require.True(t, strings.HasPrefix(groupID, "ag_"))
			defer func() {
				w := call("group-cleanup", "DELETE", prefix+"/asset-groups/"+groupID, nil)
				if w.Code != 200 || gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error").Exists() {
					t.Errorf("group cleanup failed: %s", groupID)
				}
			}()
			fetched := call("group-get", "GET", prefix+"/asset-groups/"+groupID, nil)
			success(t, fetched)
			require.Equal(t, groupID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
			asset := call("asset-create", "POST", prefix+"/assets", map[string]any{"model": cfg.Model, "name": name, "group_id": groupID, "url": cfg.AssetURL, "asset_type": "Image"})
			success(t, asset)
			assetID := gjson.GetBytes(asset.Body.Bytes(), "Result.Id").String()
			require.True(t, strings.HasPrefix(assetID, "asset_"))
			defer func() {
				w := call("asset-cleanup", "DELETE", prefix+"/assets/"+assetID, nil)
				if w.Code != 200 || gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error").Exists() {
					t.Errorf("asset cleanup failed: %s", assetID)
				}
			}()
			require.Equal(t, groupID, gjson.GetBytes(asset.Body.Bytes(), "Result.GroupId").String())
			officialID := gjson.GetBytes(asset.Body.Bytes(), "Result.upstream_asset_id").String()
			require.True(t, strings.HasPrefix(officialID, "asset-"))
			t.Logf("created group=%s asset=%s official=%s", groupID, assetID, officialID)
			active := false
			deadline := time.Now().Add(4 * time.Minute)
			for time.Now().Before(deadline) {
				fetched = call("asset-post-get", "POST", "/v1/assets/get", map[string]string{"asset_id": assetID, "model": cfg.Model})
				success(t, fetched)
				require.Equal(t, assetID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
				require.Equal(t, groupID, gjson.GetBytes(fetched.Body.Bytes(), "Result.GroupId").String())
				require.Equal(t, officialID, gjson.GetBytes(fetched.Body.Bytes(), "Result.upstream_asset_id").String())
				status := gjson.GetBytes(fetched.Body.Bytes(), "Result.Status").String()
				t.Logf("asset status=%s", status)
				if status == "Active" {
					active = true
					break
				}
				require.NotEqual(t, "Failed", status, "see private asset evidence")
				time.Sleep(3 * time.Second)
			}
			require.True(t, active, "asset processing deadline exceeded")
			for _, path := range []string{"/api/assets/" + assetID, "/v1/assets/" + assetID, "/v1/assets/get?asset_id=" + assetID} {
				fetched = call("asset-get", "GET", path, nil)
				success(t, fetched)
				require.Equal(t, assetID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
				require.Equal(t, "Active", gjson.GetBytes(fetched.Body.Bytes(), "Result.Status").String())
			}
			// Re-read the real group after processing, before cleanup.
			fetched = call("group-final-get", "GET", prefix+"/asset-groups/"+groupID, nil)
			success(t, fetched)
			require.Equal(t, groupID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
		})
	}
}
