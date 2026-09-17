package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetBackendPreservesKlingVideoResources(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "test-only-persistent-secret")
	for _, smart := range []bool{false, true} {
		name := "normal"
		if smart {
			name = "smart"
		}
		t.Run(name, func(t *testing.T) {
			var videoCalls, assetCalls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				videoCalls.Add(1)
				assert.Equal(t, "/v1/videos/text2video/kling-task", r.URL.Path)
				token, err := jwt.Parse(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), func(token *jwt.Token) (any, error) { return []byte("mock-sk"), nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("mock-ak"))
				if !assert.NoError(t, err) {
					w.WriteHeader(401)
					return
				}
				assert.True(t, token.Valid, "video request must retain video JWT authentication")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"data":{"task_id":"kling-task","task_status":"succeed"}}`))
			}))
			defer upstream.Close()
			assetUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assetCalls.Add(1)
				w.WriteHeader(500)
			}))
			defer assetUpstream.Close()
			r := tgxMaasResourceTestRouter(t, upstream.URL, "mock-ak|mock-sk", "kling-v1")
			r.GET("/kling/v1/videos/text2video/:id", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
			r.GET("/fixed/kling/:id", middleware.ConfigurableResource("kling-video", "text2video_get"), middleware.TokenAuth(), RelayConfigurableResource)
			if smart {
				require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
				strategy := model.RoutingStrategies()[0]
				require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["default"]`, Scores: `{}`, Config: `{}`}))
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + strategy + `"}`}).Error)
			}
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			for _, backend := range []string{"inherit", "disabled", "tgxmaas", configurable.OfficialAssetBackend} {
				t.Run(backend, func(t *testing.T) {
					cfg := &relaydto.AssetLibrarySettings{Backend: backend, AuthMode: "channel_key", BaseURL: assetUpstream.URL}
					var credentials *model.AssetCredentials
					if backend == "tgxmaas" {
						cfg.AuthMode = "api_key"
						credentials = &model.AssetCredentials{APIKey: "asset-key"}
					} else if backend == configurable.OfficialAssetBackend {
						cfg.AuthMode = "aksk"
						credentials = &model.AssetCredentials{AccessKeyID: "asset-ak", SecretAccessKey: "asset-sk"}
					}
					ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "kling-video", ProjectName: "nmyk", AssetLibrary: cfg}})
					require.NoError(t, prepareAssetCredentials(ch, nil, credentials))
					require.NoError(t, model.DB.Save(ch).Error)
					for _, path := range []string{"/kling/v1/videos/text2video/kling-task", "/fixed/kling/kling-task"} {
						w := seedanceCall(r, "GET", path, "", 1)
						require.Equal(t, 200, w.Code, "asset setting broke video query: %s", w.Body.String())
					}
				})
			}
			require.EqualValues(t, 8, videoCalls.Load())
			require.Zero(t, assetCalls.Load(), "video requests must never reach the asset backend")
		})
	}
}
