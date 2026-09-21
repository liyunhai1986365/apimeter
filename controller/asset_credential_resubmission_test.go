package controller

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"gorm.io/gorm"
)

func TestAssetCredentialResubmissionPreservesOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, backend, auth string
		original, rotated   model.AssetCredentials
	}{
		{"youniyouju_api_key", "youniyouju", "api_key", model.AssetCredentials{APIKey: "asset-key"}, model.AssetCredentials{APIKey: "rotated-key"}},
		{"tgxmaas_api_key", "tgxmaas", "api_key", model.AssetCredentials{APIKey: "asset-key"}, model.AssetCredentials{APIKey: "rotated-key"}},
		{"ak_rotation", "volcengine-assets", "aksk", model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "asset-sk"}, model.AssetCredentials{AccessKeyID: "rotated-ak", SecretAccessKey: "asset-sk"}},
		{"sk_rotation", "volcengine-assets", "aksk", model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "asset-sk"}, model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "rotated-sk"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			wantCredentials := tc.original
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if tc.auth == "api_key" {
					require.Equal(t, "Bearer "+wantCredentials.APIKey, r.Header.Get("Authorization"))
				} else {
					require.Contains(t, r.Header.Get("Authorization"), "Credential="+wantCredentials.AccessKeyID+"/")
				}
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Query().Get("Action") == "GetAssetGroup" {
					_, _ = w.Write([]byte(`{"Result":{"Id":"group-owned"}}`))
				} else {
					_, _ = w.Write([]byte(`{"Result":{"Id":"asset-owned","Status":"Active"}}`))
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, tc.backend, "")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			settings := ch.GetSetting()
			settings.Protocol.AssetLibrary.AuthMode = tc.auth
			ch.SetSetting(settings)
			require.NoError(t, ch.SetAssetCredentials(&tc.original))
			require.NoError(t, model.DB.Save(ch).Error)
			seedAssetOwnershipForTest(t, ch.Id, 1, "asset", "asset-owned")
			seedAssetOwnershipForTest(t, ch.Id, 1, "group", "group-owned")
			originalSecret := ch.AssetSecret
			profile, ok := configurable.AssetProfile(settings.Protocol)
			require.True(t, ok)
			resource, ok := profile.ResourceByID("assets_get")
			require.True(t, ok)
			originalCacheKey := configurableResourceStateKey(ch, resource, "test-project")

			// Both a credential-only update and a complete settings save must
			// retain existing ownership without a new fingerprint or migration.
			for _, includeSettings := range []bool{false, true} {
				patch := map[string]any{"id": ch.Id, "asset_credentials": tc.original}
				if includeSettings {
					patch["setting"] = ch.Setting
					patch["type"] = ch.Type
					patch["base_url"] = ch.BaseURL
				}
				response := assetCredentialUpdateForTest(t, patch)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.True(t, gjson.Get(response.Body.String(), "success").Bool(), response.Body.String())
				ch, err = model.GetChannelById(ch.Id, true)
				require.NoError(t, err)
				require.Equal(t, originalSecret, ch.AssetSecret)
				require.Equal(t, originalCacheKey, configurableResourceStateKey(ch, resource, "test-project"))
				for _, path := range []string{"/api/assets/asset-owned", "/api/asset-groups/group-owned"} {
					owned := seedanceCall(r, http.MethodGet, path, "", 1)
					require.Equal(t, http.StatusOK, owned.Code, owned.Body.String())
					denied := seedanceCall(r, http.MethodGet, path, "", 2)
					require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
				}
			}
			require.Equal(t, int32(4), calls.Load())

			rotated := assetCredentialUpdateForTest(t, map[string]any{"id": ch.Id, "asset_credentials": tc.rotated})
			require.Equal(t, http.StatusOK, rotated.Code, rotated.Body.String())
			require.True(t, gjson.Get(rotated.Body.String(), "success").Bool(), rotated.Body.String())
			ch, err = model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			require.NotEqual(t, originalSecret, ch.AssetSecret)
			decoded, err := ch.GetAssetCredentials()
			require.NoError(t, err)
			require.Equal(t, tc.rotated, *decoded)
			wantCredentials = tc.rotated
			require.Equal(t, originalCacheKey, configurableResourceStateKey(ch, resource, "test-project"))
			for _, path := range []string{"/api/assets/asset-owned", "/api/asset-groups/group-owned"} {
				owned := seedanceCall(r, http.MethodGet, path, "", 1)
				require.Equal(t, http.StatusOK, owned.Code, owned.Body.String())
				denied := seedanceCall(r, http.MethodGet, path, "", 2)
				require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
			}
			require.Equal(t, int32(6), calls.Load(), "rotation must preserve the owner's access using the new credential")
		})
	}
}

func TestAssetCredentialResubmissionRejectsConcurrentRotation(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "credential-resubmission-test")
	for _, tc := range []struct {
		name, backend, auth string
		original, rotated   model.AssetCredentials
	}{
		{"api_key", "youniyouju", "api_key", model.AssetCredentials{APIKey: "asset-key"}, model.AssetCredentials{APIKey: "rotated-key"}},
		{"aksk", "volcengine-assets", "aksk", model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "asset-sk"}, model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "rotated-sk"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			openConfigurableResourceTestDB(t)
			ch := &model.Channel{Type: constant.ChannelTypeConfigurable, Name: "original-name"}
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
				ProfileID: "doubao-seedance-2", ProjectName: "test-project",
				AssetLibrary: &relaydto.AssetLibrarySettings{Backend: tc.backend, AuthMode: tc.auth},
			}})
			require.NoError(t, ch.SetAssetCredentials(&tc.original))
			require.NoError(t, model.DB.Create(ch).Error)
			rotated := *ch
			require.NoError(t, rotated.SetAssetCredentials(&tc.rotated))

			// Rotate after UpdateChannel validates its snapshot but before the
			// transaction re-reads it; the partial body deliberately omits settings.
			reads := 0
			const callback = "test:rotate_before_asset_credential_resubmission"
			require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != "channels" {
					return
				}
				reads++
				if reads == 2 {
					require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).
						Update("asset_secret", rotated.AssetSecret).Error)
				}
			}))
			t.Cleanup(func() { _ = model.DB.Callback().Query().Remove(callback) })
			response := assetCredentialUpdateForTest(t, map[string]any{
				"id": ch.Id, "name": "stale-edit", "asset_credentials": tc.original,
			})
			require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
			require.False(t, gjson.Get(response.Body.String(), "success").Bool())
			require.GreaterOrEqual(t, reads, 2)
			stored, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			require.Equal(t, rotated.AssetSecret, stored.AssetSecret)
			require.Equal(t, "original-name", stored.Name)
		})
	}
}

func TestAssetCredentialResubmissionRepairsUnreadableSecret(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "credential-resubmission-test")
	for _, secret := range []string{"", "invalid-ciphertext"} {
		t.Run("secret="+secret, func(t *testing.T) {
			openConfigurableResourceTestDB(t)
			ch := &model.Channel{Type: constant.ChannelTypeConfigurable, AssetSecret: secret}
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{
				ProfileID:    "doubao-seedance-2",
				AssetLibrary: &relaydto.AssetLibrarySettings{Backend: "youniyouju", AuthMode: "api_key"},
			}})
			require.NoError(t, model.DB.Create(ch).Error)
			credentials := model.AssetCredentials{APIKey: "replacement-key"}
			response := assetCredentialUpdateForTest(t, map[string]any{"id": ch.Id, "asset_credentials": credentials})
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			require.True(t, gjson.Get(response.Body.String(), "success").Bool(), response.Body.String())
			stored, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			decoded, err := stored.GetAssetCredentials()
			require.NoError(t, err)
			require.Equal(t, credentials, *decoded)
		})
	}
}

func TestAssetCredentialRotationPreservesLegacyBindings(t *testing.T) {
	for _, mode := range []string{"channel_key", "api_key"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer replacement-key", r.Header.Get("Authorization"))
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Query().Get("Action") {
				case "ListAssets":
					_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"legacy-owned"},{"Id":"foreign"},{"Id":"revoked"}]}}`))
				case "DeleteAsset":
					_, _ = w.Write([]byte(`{"Result":{}}`))
				default:
					_, _ = w.Write([]byte(`{"Result":{"Id":"legacy-owned","Status":"Active"}}`))
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			settings := ch.GetSetting()
			settings.Protocol.AssetLibrary.AuthMode = mode
			ch.SetSetting(settings)
			credential := ch.Key
			if mode == "api_key" {
				require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "original-key"}))
				credential = ch.AssetSecret
			}
			require.NoError(t, model.DB.Save(ch).Error)
			// Persist the exact old format without invoking assetAccountScope or
			// initializing the new endpoint identity before the key is replaced.
			legacyScope := fmt.Sprintf("%x", sha256.Sum256([]byte("youniyouju\x00"+upstream.URL+"\x00"+mode+"\x00"+credential)))
			owned := model.AssetBinding{ChannelID: ch.Id, UserID: 1, Backend: "youniyouju", Scope: legacyScope, Kind: "asset", CanonicalID: "legacy-owned"}
			require.NoError(t, model.SaveAssetBinding(owned, "legacy-owned"))
			foreign := owned
			foreign.UserID, foreign.CanonicalID = 2, "foreign"
			require.NoError(t, model.SaveAssetBinding(foreign, "foreign"))
			revoked := owned
			revoked.CanonicalID = "revoked"
			require.NoError(t, model.SaveAssetBinding(revoked, "revoked"))
			require.NoError(t, model.InvalidateAssetBindings(revoked, false))
			var before []model.ConfigurableResourceState
			require.NoError(t, model.DB.Order("id").Find(&before).Error)
			require.Len(t, before, 3, "the fixture must contain only legacy ownership rows")

			patch := map[string]any{"id": ch.Id}
			if mode == "channel_key" {
				patch["key"] = "replacement-key"
			} else {
				patch["asset_credentials"] = model.AssetCredentials{APIKey: "replacement-key"}
			}
			updated := assetCredentialUpdateForTest(t, patch)
			require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
			require.True(t, gjson.Get(updated.Body.String(), "success").Bool(), updated.Body.String())
			var after []model.ConfigurableResourceState
			require.NoError(t, model.DB.Where("profile_id = ?", "asset-access-v1").Order("id").Find(&after).Error)
			require.Equal(t, before, after, "credential rotation must not rewrite ownership or revive deleted resources")
			got := seedanceCall(r, http.MethodGet, "/api/assets/legacy-owned", "", 1)
			require.Equal(t, http.StatusOK, got.Code, got.Body.String())
			listed := seedanceCall(r, http.MethodGet, "/api/assets", "", 1)
			require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
			require.Equal(t, "legacy-owned", gjson.Get(listed.Body.String(), "Result.Items.0.Id").String())
			require.Len(t, gjson.Get(listed.Body.String(), "Result.Items").Array(), 1)
			for _, id := range []string{"foreign", "revoked"} {
				denied := seedanceCall(r, http.MethodGet, "/api/assets/"+id, "", 1)
				require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
			}
			deleted := seedanceCall(r, http.MethodDelete, "/api/assets/legacy-owned", "", 1)
			require.Equal(t, http.StatusOK, deleted.Code, deleted.Body.String())
			denied := seedanceCall(r, http.MethodGet, "/api/assets/legacy-owned", "", 1)
			require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
		})
	}
}

func assetCredentialUpdateForTest(t *testing.T, patch map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := common.Marshal(patch)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/channel", strings.NewReader(string(raw)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("id", 1)
	c.Set("role", common.RoleRootUser)
	UpdateChannel(c)
	require.NotContains(t, w.Body.String(), "asset_credentials")
	return w
}
