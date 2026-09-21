package controller

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Explicit opt-in: each Token mode creates one temporary group and image.
// Only resources created by this run are deleted. No video or human
// verification is requested. Credentials and raw responses stay outside git.
func TestYouniyoujuAssetsLive(t *testing.T) {
	configPath := os.Getenv("YOUNIYOUJU_ASSETS_LIVE_CONFIG")
	if configPath == "" {
		t.Skip("requires explicit YOUNIYOUJU_ASSETS_LIVE_CONFIG")
	}
	var cfg struct {
		URL      string `json:"url"`
		Key      string `json:"key"`
		AssetURL string `json:"asset_url"`
	}
	raw, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(raw, &cfg))
	require.NotEmpty(t, cfg.URL)
	require.NotEmpty(t, cfg.Key)
	require.NotEmpty(t, cfg.AssetURL)
	evidence := os.Getenv("YOUNIYOUJU_ASSETS_LIVE_EVIDENCE")
	require.NotEmpty(t, evidence, "raw responses require a private evidence directory")
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	for _, auth := range []string{"channel_key", "api_key"} {
		t.Run(auth, func(t *testing.T) {
			evidence := filepath.Join(evidence, auth)
			require.NoError(t, os.MkdirAll(evidence, 0700))
			require.NoError(t, os.Chmod(evidence, 0700))
			t.Setenv("CRYPTO_SECRET", "youniyouju-live-isolated-fixture")
			r := tgxMaasResourceTestRouter(t, cfg.URL, "unused-fixture-key", tgxRegressionModel)
			registerTgxConversionTestRoutes(t, r)
			r.POST(configurable.YouniyoujuAssetPath, middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayArkAssetAction)
			client := service.GetHttpClient()
			originalTimeout := client.Timeout
			client.Timeout = 30 * time.Second
			t.Cleanup(func() { client.Timeout = originalTimeout })
			// Exercise real channel administration handlers. Silence SQL logging before
			// inserting credentials, including on any unexpected database failure.
			model.DB = model.DB.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
			model.LOG_DB = model.DB
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 20).Update("status", common.ChannelStatusManuallyDisabled).Error)
			admin := func(label, method string, payload any, handler gin.HandlerFunc, id int) *httptest.ResponseRecorder {
				t.Helper()
				body, err := common.Marshal(payload)
				require.NoError(t, err)
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(method, "/api/channel", bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				c.Set("role", common.RoleRootUser)
				c.Set("id", 1)
				if id > 0 {
					c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
				}
				handler(c)
				require.True(t, !strings.Contains(w.Body.String(), cfg.Key), "channel response must not expose the supplier credential")
				require.NoError(t, os.WriteFile(filepath.Join(evidence, label+".json"), w.Body.Bytes(), 0600))
				require.Equal(t, http.StatusOK, w.Code)
				require.True(t, gjson.GetBytes(w.Body.Bytes(), "success").Bool(), "channel operation failed; inspect private evidence")
				t.Logf("%s HTTP %d success=true", label, w.Code)
				return w
			}
			channelName := fmt.Sprintf("apimeter-live-channel-%s-%d", auth, time.Now().UnixNano())
			ch := &model.Channel{Type: constant.ChannelTypeConfigurable, Status: common.ChannelStatusEnabled, Name: channelName, Key: cfg.Key, BaseURL: &cfg.URL, Models: tgxRegressionModel, Group: "default"}
			settings := dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
				ProfileID: "doubao-seedance-2",
				AssetLibrary: &relaydto.AssetLibrarySettings{
					Backend: configurable.YouniyoujuAssetBackend, AuthMode: auth,
				},
			}}
			var credentials *model.AssetCredentials
			if auth == "api_key" {
				settings.Protocol.AssetLibrary.BaseURL = cfg.URL + "/"
				ch.Key = "unused-video-key"
				credentials = &model.AssetCredentials{APIKey: cfg.Key}
			}
			ch.SetSetting(settings)
			admin("channel-create", "POST", AddChannelRequest{Mode: "single", Channel: ch, AssetCredentials: credentials}, AddChannel, 0)
			require.NoError(t, model.DB.Where("name = ?", channelName).First(ch).Error)
			public := admin("channel-read", "GET", nil, GetChannel, ch.Id)
			publicSetting := gjson.GetBytes(public.Body.Bytes(), "data.setting").String()
			require.Equal(t, configurable.YouniyoujuAssetBackend, gjson.Get(publicSetting, "protocol.asset_library.backend").String())
			require.Equal(t, auth, gjson.Get(publicSetting, "protocol.asset_library.auth_mode").String())
			storedSecret := ch.AssetSecret
			if auth == "api_key" {
				require.NotEmpty(t, storedSecret)
				decoded, err := ch.GetAssetCredentials()
				require.NoError(t, err)
				require.True(t, decoded.APIKey == cfg.Key, "dedicated credential must survive channel creation")
			}
			update := map[string]any{"id": ch.Id, "type": ch.Type, "name": channelName + "-edited", "models": ch.Models, "group": "default", "base_url": cfg.URL, "setting": *ch.Setting}
			admin("channel-edit-preserve-key", "PUT", update, UpdateChannel, ch.Id)
			ch, err = model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			require.True(t, storedSecret == ch.AssetSecret, "editing without a new asset key must preserve its ciphertext")
			if auth == "channel_key" {
				require.True(t, ch.Key == cfg.Key, "editing without a new channel key must preserve the existing key")
			}

			sequence := 0
			call := func(label, method, path string, payload map[string]any) *httptest.ResponseRecorder {
				t.Helper()
				if payload == nil {
					payload = map[string]any{}
				}
				payload["model"] = tgxRegressionModel // local routing only, stripped upstream
				body, err := common.Marshal(payload)
				require.NoError(t, err)
				w := seedanceCall(r, method, path, string(body), 1)
				sequence++
				// Never print the raw body: account data and signed URLs can be present.
				filename := filepath.Join(evidence, fmt.Sprintf("%02d-%s.json", sequence, label))
				if err := os.WriteFile(filename, w.Body.Bytes(), 0600); err != nil {
					t.Errorf("could not save private response evidence: %v", err)
				}
				t.Logf("%s HTTP %d status=%s error_code=%s", label, w.Code,
					gjson.GetBytes(w.Body.Bytes(), "Result.Status").String(),
					gjson.GetBytes(w.Body.Bytes(), "ResponseMetadata.Error.Code").String())
				return w
			}
			action := func(label, action string, payload map[string]any) *httptest.ResponseRecorder {
				return call(label, http.MethodPost, configurable.YouniyoujuAssetPath+"?Action="+action+"&Version=2024-01-01", payload)
			}
			ok := func(w *httptest.ResponseRecorder) bool {
				body := w.Body.Bytes()
				return w.Code == http.StatusOK && gjson.ValidBytes(body) &&
					!gjson.GetBytes(body, "ResponseMetadata.Error").Exists() &&
					!gjson.GetBytes(body, "error").Exists() && gjson.GetBytes(body, "Result").Exists()
			}
			success := func(w *httptest.ResponseRecorder) {
				t.Helper()
				require.True(t, ok(w), "upstream operation failed; inspect private response evidence")
			}
			// The gateway intentionally hides revoked bindings. Verify physical deletion
			// against the supplier as well, without filtering through local ownership.
			verifyDeleted := func(label, operation, id string, filter map[string]any) {
				t.Helper()
				for page := 1; page <= 100; page++ {
					body, err := common.Marshal(map[string]any{"Filter": filter, "PageNumber": page, "PageSize": 100})
					require.NoError(t, err)
					req, err := http.NewRequest("POST", cfg.URL+configurable.YouniyoujuAssetPath+"?Action="+operation+"&Version=2024-01-01", bytes.NewReader(body))
					require.NoError(t, err)
					req.Header.Set("Content-Type", "application/json")
					req.Header.Set("Authorization", "Bearer "+cfg.Key)
					resp, err := client.Do(req)
					require.NoError(t, err)
					data, err := io.ReadAll(io.LimitReader(resp.Body, (16<<20)+1))
					_ = resp.Body.Close()
					require.NoError(t, err)
					require.LessOrEqual(t, len(data), 16<<20)
					require.NoError(t, os.WriteFile(filepath.Join(evidence, fmt.Sprintf("supplier-%s-%d.json", label, page)), data, 0600))
					require.Equal(t, http.StatusOK, resp.StatusCode, "supplier cleanup verification failed; inspect private evidence")
					require.True(t, assetResponseSuccessful(data))
					require.NotContains(t, gjson.GetBytes(data, "Result.Items.#.Id").String(), id)
					total := gjson.GetBytes(data, "Result.TotalCount")
					require.True(t, total.Exists(), "supplier cleanup verification requires a total")
					if int64(page*100) >= total.Int() {
						t.Logf("supplier-%s HTTP %d deletion_confirmed=true", label, resp.StatusCode)
						return
					}
				}
				t.Fatal("supplier cleanup verification exceeded 100 pages")
			}
			for _, listAction := range []string{"ListAssetGroups", "ListAssets"} {
				success(action("initial-"+listAction, listAction, map[string]any{
					"Filter": map[string]any{"GroupType": "AIGC"}, "PageNumber": 1, "PageSize": 1,
				}))
			}

			name := fmt.Sprintf("apimeter-live-%d", time.Now().UnixNano())
			created := action("group-create", "CreateAssetGroup", map[string]any{
				"Name": name, "Description": "Temporary asset API test", "GroupType": "AIGC",
			})
			groupID := gjson.GetBytes(created.Body.Bytes(), "Result.Id").String()
			groupDeleted := false
			defer func() {
				if groupID != "" && !groupDeleted {
					if !ok(action("group-cleanup", "DeleteAssetGroup", map[string]any{"Id": groupID})) {
						t.Error("temporary group cleanup failed; ID is in private group-create evidence")
					}
				}
			}()
			success(created)
			require.NotEmpty(t, groupID, "inspect creation evidence before retrying")
			fetched := action("group-get", "GetAssetGroup", map[string]any{"Id": groupID})
			success(fetched)
			require.Equal(t, groupID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
			success(call("group-update-rest", "PATCH", "/api/asset-groups/"+groupID, map[string]any{"Name": name + "-updated"}))
			fetched = action("group-get-updated", "GetAssetGroup", map[string]any{"Id": groupID})
			success(fetched)
			require.Equal(t, name+"-updated", gjson.GetBytes(fetched.Body.Bytes(), "Result.Name").String())
			groupFilter := map[string]any{"Filter": map[string]any{"GroupIds": []string{groupID}, "GroupType": "AIGC"}, "PageNumber": 1, "PageSize": 20}
			listed := action("group-list", "ListAssetGroups", groupFilter)
			success(listed)
			require.Contains(t, gjson.GetBytes(listed.Body.Bytes(), "Result.Items.#.Id").String(), groupID)

			created = action("asset-create", "CreateAsset", map[string]any{
				"GroupId": groupID, "URL": cfg.AssetURL, "AssetType": "Image", "Name": name,
			})
			assetID := gjson.GetBytes(created.Body.Bytes(), "Result.Id").String()
			assetDeleted := false
			defer func() {
				if assetID != "" && !assetDeleted {
					if !ok(action("asset-cleanup", "DeleteAsset", map[string]any{"Id": assetID})) {
						t.Error("temporary asset cleanup failed; ID is in private asset-create evidence")
					}
				}
			}()
			success(created)
			require.NotEmpty(t, assetID, "inspect creation evidence before retrying")
			active := false
			for deadline := time.Now().Add(3 * time.Minute); time.Now().Before(deadline); {
				fetched = call("asset-get-rest", "GET", "/api/assets/"+assetID, nil)
				success(fetched)
				require.Equal(t, assetID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
				status := gjson.GetBytes(fetched.Body.Bytes(), "Result.Status").String()
				if status == "Active" {
					active = true
					break
				}
				if status == "Failed" {
					t.Error("asset processing failed; inspect private asset-get evidence")
					break
				}
				time.Sleep(3 * time.Second)
			}
			if !active {
				t.Error("asset did not reach Active")
			}
			success(action("asset-update", "UpdateAsset", map[string]any{"Id": assetID, "Name": name + "-updated"}))
			fetched = action("asset-get-updated", "GetAsset", map[string]any{"Id": assetID})
			success(fetched)
			require.Equal(t, name+"-updated", gjson.GetBytes(fetched.Body.Bytes(), "Result.Name").String())
			assetFilter := map[string]any{"Filter": map[string]any{"GroupIds": []string{groupID}, "GroupType": "AIGC"}, "PageNumber": 1, "PageSize": 20}
			listed = action("asset-list", "ListAssets", assetFilter)
			success(listed)
			require.Contains(t, gjson.GetBytes(listed.Body.Bytes(), "Result.Items.#.Id").String(), assetID)

			// Changing only video credentials/address must preserve the independent
			// asset account. Changing asset credentials would revoke the binding.
			if auth == "api_key" {
				update["base_url"], update["key"] = "http://127.0.0.1:1", "unused-video-key-edited"
				admin("channel-edit-independent-video", "PUT", update, UpdateChannel, ch.Id)
				fetched = call("asset-independent-token-url", "GET", "/api/assets/"+assetID, nil)
				success(fetched)
				require.Equal(t, assetID, gjson.GetBytes(fetched.Body.Bytes(), "Result.Id").String())
			}

			success(call("asset-delete-rest", "DELETE", "/api/assets/"+assetID, nil))
			assetDeleted = true
			verifyDeleted("asset-deleted", "ListAssets", assetID, map[string]any{"GroupIds": []string{groupID}, "GroupType": "AIGC"})
			listed = action("asset-list-after-delete", "ListAssets", assetFilter)
			success(listed)
			require.NotContains(t, gjson.GetBytes(listed.Body.Bytes(), "Result.Items.#.Id").String(), assetID)
			success(action("group-delete", "DeleteAssetGroup", map[string]any{"Id": groupID}))
			groupDeleted = true
			verifyDeleted("group-deleted", "ListAssetGroups", groupID, map[string]any{"GroupType": "AIGC"})
			// The supplier returns an HTML 404 when Filter.GroupIds contains a deleted
			// group. Verify absence using the paginated account list instead; a 404 alone
			// would not prove deletion because it could also be a routing failure.
			for page := 1; ; page++ {
				require.LessOrEqual(t, page, 100, "cleanup verification exceeded 100 pages")
				listed = action("group-list-after-delete", "ListAssetGroups", map[string]any{
					"Filter": map[string]any{"GroupType": "AIGC"}, "PageNumber": page, "PageSize": 100,
				})
				success(listed)
				require.NotContains(t, gjson.GetBytes(listed.Body.Bytes(), "Result.Items.#.Id").String(), groupID)
				total := gjson.GetBytes(listed.Body.Bytes(), "Result.TotalCount")
				require.True(t, total.Exists(), "cannot confirm cleanup without pagination total")
				if int64(page*100) >= total.Int() {
					break
				}
			}
			var tasks int64
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
			require.Zero(t, tasks)
			if !t.Failed() {
				t.Logf("All 10 CRUD operations passed through gateway; asset Active; auth=%s; channel create/read/edit passed; supplier cleanup verified; liveness not run", auth)
			}
		})
	}
}
