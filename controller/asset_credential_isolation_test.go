package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDedicatedAssetsDoNotRequireEnabledVideoKey(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "review-only")
	for _, backend := range []string{"tgxmaas", configurable.OfficialAssetBackend} {
		for _, smart := range []bool{false, true} {
			name := "normal"
			if smart {
				name = "smart"
			}
			t.Run(backend+"/"+name, func(t *testing.T) {
				preserveSmartRetryTestConfiguration(t)
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if backend == "tgxmaas" {
						assert.Equal(t, "Bearer asset-only-key", r.Header.Get("Authorization"))
					} else {
						assert.Contains(t, r.Header.Get("Authorization"), "Credential=asset-only-ak/")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"Result":{"Items":[]}}`))
				}))
				defer upstream.Close()
				r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "video-a\nvideo-b", "model")
				if smart {
					require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
					require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
					strategy := model.RoutingStrategies()[0]
					require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["default"]`, Scores: `{}`, Config: `{}`}))
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + strategy + `"}`}).Error)
				}
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				cfg := ch.GetSetting()
				cfg.Protocol.ProjectName = "mock-project"
				auth := "api_key"
				if backend == configurable.OfficialAssetBackend {
					auth = "aksk"
				}
				cfg.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: backend, AuthMode: auth, BaseURL: upstream.URL}
				ch.SetSetting(cfg)
				ch.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 2, MultiKeyMode: constant.MultiKeyModePolling}
				require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "asset-only-key", AccessKeyID: "asset-only-ak", SecretAccessKey: "asset-only-sk"}))
				require.NoError(t, model.DB.Save(ch).Error)
				response := seedanceCall(r, "POST", "/v1/private-avatar/groups/list", `{"model":"model"}`, 1)
				require.Equal(t, 200, response.Code, response.Body.String())
				require.EqualValues(t, 1, calls.Load())
				saved, err := model.GetChannelById(ch.Id, true)
				require.NoError(t, err)
				require.Zero(t, saved.ChannelInfo.MultiKeyPollingIndex, "assets must not advance video key rotation")
				ch.ChannelInfo.MultiKeyStatusList = map[int]int{0: common.ChannelStatusManuallyDisabled, 1: common.ChannelStatusManuallyDisabled}
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("channel_info", ch.ChannelInfo).Error)
				response = seedanceCall(r, "POST", "/v1/private-avatar/groups/list", `{"model":"model"}`, 1)
				t.Logf("same asset account, disabled only video keys: HTTP=%d upstream_calls=%d body=%s", response.Code, calls.Load(), response.Body.String())
				require.Equal(t, 200, response.Code, "dedicated asset credentials should not require a video key: %s", response.Body.String())
				require.EqualValues(t, 2, calls.Load())

				// Revoking video credentials must still block non-asset resources.
				videoCfg := ch.GetSetting()
				videoCfg.Protocol.ProfileID = "kling-video"
				ch.SetSetting(videoCfg)
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("setting", ch.Setting).Error)
				r.GET("/kling/v1/videos/text2video/:id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
				response = seedanceCall(r, "GET", "/kling/v1/videos/text2video/task?model=model", "", 1)
				if smart {
					require.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "get_channel_failed")
				} else {
					require.Equal(t, http.StatusInternalServerError, response.Code, response.Body.String())
					require.Contains(t, response.Body.String(), "channel:no_available_key")
				}
				require.EqualValues(t, 2, calls.Load())
				ch.SetSetting(cfg)
				require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("setting", ch.Setting).Error)

				for _, boundary := range []string{"disabled_channel", "wrong_group", "disabled_ability", "missing_credentials", "invalid_credentials"} {
					t.Run(boundary, func(t *testing.T) {
						wantStatus := http.StatusServiceUnavailable
						switch boundary {
						case "disabled_channel":
							require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("status", common.ChannelStatusManuallyDisabled).Error)
							defer model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("status", common.ChannelStatusEnabled)
						case "wrong_group":
							require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("group", "other").Error)
							defer model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("group", "default")
						case "disabled_ability":
							require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", ch.Id).Update("enabled", false).Error)
							defer model.DB.Model(&model.Ability{}).Where("channel_id = ?", ch.Id).Update("enabled", true)
						case "missing_credentials", "invalid_credentials":
							wantStatus = http.StatusBadRequest
							secret := ""
							if boundary == "invalid_credentials" {
								secret = "invalid-ciphertext"
							}
							require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("asset_secret", secret).Error)
							defer model.DB.Model(&model.Channel{}).Where("id = ?", ch.Id).Update("asset_secret", ch.AssetSecret)
						}
						response = seedanceCall(r, "POST", "/v1/private-avatar/groups/list", `{"model":"model"}`, 1)
						require.Equal(t, wantStatus, response.Code, response.Body.String())
						require.EqualValues(t, 2, calls.Load(), "authorization failures must not reach the asset upstream")
					})
				}
				// Asset management only isolates users; video model limits do not
				// prevent listing that user's assets on the selected library.
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": "other"}).Error)
				response = seedanceCall(r, "POST", "/v1/private-avatar/groups/list", `{"model":"model"}`, 1)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				require.EqualValues(t, 3, calls.Load())
			})
		}
	}
}

func TestSharedAssetCredentialsStillRequireVideoKeys(t *testing.T) {
	for _, backend := range []string{"inherit", "tgxmaas"} {
		for _, separateURL := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/separate_url_%t", backend, separateURL), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(200)
				}))
				defer upstream.Close()
				r := tgxMaasResourceTestRouter(t, upstream.URL, "video-key", "model")
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				cfg := ch.GetSetting()
				cfg.Protocol.ProjectName = "mock-project"
				cfg.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: backend, AuthMode: "channel_key", BaseURL: upstream.URL}
				if separateURL {
					cfg.Protocol.AssetLibrary.BaseURL = upstream.URL + "/assets"
				}
				ch.SetSetting(cfg)
				ch.ChannelInfo = model.ChannelInfo{IsMultiKey: true, MultiKeySize: 1, MultiKeyMode: constant.MultiKeyModeRandom, MultiKeyStatusList: map[int]int{0: common.ChannelStatusManuallyDisabled}}
				require.NoError(t, model.DB.Save(ch).Error)
				response := seedanceCall(r, "POST", "/v1/private-avatar/groups/list", `{"model":"model"}`, 1)
				require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
				require.Contains(t, response.Body.String(), "dedicated credentials")
				require.Zero(t, calls.Load())
			})
		}
	}
}
