package controller

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Independent HMAC verification at the upstream boundary: catches a signature
// calculated before body, path, query or headers were finalized.
func verifyAssetSignature(t *testing.T, r *http.Request, body []byte) {
	t.Helper()
	hash := func(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
	require.Equal(t, hash(body), r.Header.Get("X-Content-Sha256"))
	auth := r.Header.Get("Authorization")
	require.True(t, strings.HasPrefix(auth, "HMAC-SHA256 Credential=mock-ak/"))
	parts := strings.Split(auth, ", ")
	require.Len(t, parts, 3)
	signed := strings.TrimPrefix(parts[1], "SignedHeaders=")
	canonicalHeaders := ""
	for _, key := range strings.Split(signed, ";") {
		value := r.Header.Get(key)
		if key == "host" {
			value = r.Host
		}
		canonicalHeaders += key + ":" + strings.TrimSpace(value) + "\n"
	}
	canonical := strings.Join([]string{r.Method, r.URL.EscapedPath(), strings.ReplaceAll(r.URL.Query().Encode(), "+", "%20"), canonicalHeaders, signed, hash(body)}, "\n")
	date := r.Header.Get("X-Date")
	require.Len(t, date, 16)
	scope := date[:8] + "/cn-beijing/ark/request"
	message := "HMAC-SHA256\n" + date + "\n" + scope + "\n" + hash([]byte(canonical))
	key := []byte("mock-sk")
	for _, v := range []string{date[:8], "cn-beijing", "ark", "request", message} {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write([]byte(v))
		key = mac.Sum(nil)
	}
	require.Equal(t, "Signature="+hex.EncodeToString(key), parts[2])
}

func TestOfficialAssetLibraryAllActions(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "test-only-persistent-secret")
	var expectedAction, expectedID, expectedProject string
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/", r.URL.Path)
		require.Equal(t, expectedAction, r.URL.Query().Get("Action"))
		require.Equal(t, "2024-01-01", r.URL.Query().Get("Version"))
		require.Len(t, r.URL.Query(), 2)
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, expectedProject, gjson.GetBytes(body, "ProjectName").String())
		require.Equal(t, expectedID, gjson.GetBytes(body, "Id").String())
		require.False(t, gjson.GetBytes(body, "model").Exists())
		require.False(t, gjson.GetBytes(body, "asset_id").Exists())
		require.False(t, gjson.GetBytes(body, "id").Exists())
		require.Equal(t, "0", gjson.GetBytes(body, "extension.zero").Raw)
		require.Equal(t, "false", gjson.GetBytes(body, "extension.enabled").Raw)
		verifyAssetSignature(t, r, body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ResponseMetadata":{"RequestId":"mock-request"},"Result":{"Id":"asset-official","extension":9007199254740993}}`))
	}))
	defer upstream.Close()
	// Any attempt to reuse the VIDEO URL is a failure.
	r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "video-key", "doubao-seedance-2-5-260628")
	registerTgxConversionTestRoutes(t, r)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance2-ark-task-assets", ProjectName: "nmyk", AssetLibrary: &relaydto.AssetLibrarySettings{Backend: configurable.OfficialAssetBackend, BaseURL: upstream.URL, AuthMode: "aksk"}}})
	require.NoError(t, prepareAssetCredentials(ch, nil, &model.AssetCredentials{AccessKeyID: "mock-ak", SecretAccessKey: "mock-sk"}))
	require.NoError(t, model.DB.Save(ch).Error)
	actions := []string{"CreateAssetGroup", "ListAssetGroups", "GetAssetGroup", "UpdateAssetGroup", "DeleteAssetGroup", "CreateAsset", "GetAsset", "ListAssets", "UpdateAsset", "DeleteAsset", "CreateVisualValidateSession", "GetVisualValidateResult"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			expectedAction, expectedProject, expectedID = action, "nmyk", ""
			body := map[string]any{"model": "doubao-seedance-2-5-260628", "extension": map[string]any{"zero": 0, "enabled": false}}
			if strings.HasPrefix(action, "GetAsset") || strings.HasPrefix(action, "Update") || strings.HasPrefix(action, "Delete") {
				expectedID = "asset-official"
				body["Id"] = expectedID
			}
			if action == "CreateAsset" {
				body["GroupId"] = "group-official"
				body["AssetType"] = "Image"
				body["URL"] = "https://example.com/a.png"
			}
			if action == "CreateVisualValidateSession" {
				body["CallbackURL"] = "https://example.com/callback"
			}
			if action == "GetVisualValidateResult" {
				body["BytedToken"] = "mock-token"
			}
			if action == "ListAssets" {
				expectedProject = "explicit"
				body["ProjectName"] = expectedProject
				body["PageNumber"] = 1
				body["PageSize"] = 10
			}
			raw, err := common.Marshal(body)
			require.NoError(t, err)
			response := seedanceCall(r, "POST", "/?Action="+action+"&Version=2024-01-01", string(raw), 1)
			require.Equal(t, 200, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), "9007199254740993")
		})
	}
	for _, tc := range []struct{ method, path, action, id string }{
		{"POST", "/api/assets", "CreateAsset", ""}, {"GET", "/v1/assets/asset-official", "GetAsset", "asset-official"},
		{"POST", "/v1/assets/get", "GetAsset", "asset-official"}, {"POST", "/v1/asset-groups", "CreateAssetGroup", ""},
	} {
		expectedAction, expectedProject, expectedID = tc.action, "nmyk", tc.id
		body := `{"Id":"` + tc.id + `","extension":{"zero":0,"enabled":false}}`
		if tc.path == "/v1/assets/get" {
			body = `{"asset_id":"` + tc.id + `","extension":{"zero":0,"enabled":false}}`
		}
		response := seedanceCall(r, tc.method, tc.path, body, 1)
		require.Equal(t, 200, response.Code, response.Body.String())
	}
	require.Equal(t, 16, calls)
	serialized, err := common.Marshal(ch)
	require.NoError(t, err)
	require.NotContains(t, string(serialized), "mock-sk")
	require.NotContains(t, string(serialized), ch.AssetSecret)
	// Video profile selection is untouched by the independent resource backend.
	require.Equal(t, "seedance2-ark-task-assets", ch.GetSetting().Protocol.ProfileID)
}

func TestAssetLibraryCredentialsAndSelection(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "test-only-persistent-secret")
	r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "video-key", "model")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: "tgxmaas", AuthMode: "api_key"}
	settings.Protocol.ProjectName = "nmyk"
	var gotKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"Result":{"Id":"ag_mock"}}`))
	}))
	defer upstream.Close()
	settings.Protocol.AssetLibrary.BaseURL = upstream.URL
	ch.SetSetting(settings)
	require.NoError(t, prepareAssetCredentials(ch, nil, &model.AssetCredentials{APIKey: "asset-only-key"}))
	require.NoError(t, model.DB.Save(ch).Error)
	w := seedanceCall(r, "POST", "/v1/private-avatar/groups", `{"Name":"mock"}`, 1)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "Bearer asset-only-key", gotKey)
	updated := *ch
	updated.AssetSecret = ""
	require.NoError(t, prepareAssetCredentials(&updated, ch, nil))
	require.Empty(t, updated.AssetSecret, "omitted credentials must not write an old snapshot")
	settings.Protocol.AssetLibrary.Backend = configurable.OfficialAssetBackend
	ch.SetSetting(settings)
	require.ErrorContains(t, validateAssetLibrary(ch, nil), "AK/SK")
	settings.Protocol.AssetLibrary.Backend = "tgxmaas"
	settings.Protocol.AssetLibrary.AuthMode = "channel_key"
	ch.SetSetting(settings)
	ch.ChannelInfo.IsMultiKey = true
	require.ErrorContains(t, validateAssetLibrary(ch, nil), "multi-key")
	settings.Protocol.AssetLibrary.Backend = "disabled"
	ch.SetSetting(settings)
	_, _, ok := configurableResourceForChannelEndpoint(ch, "POST", "/v1/private-avatar/groups", "")
	require.False(t, ok)
}

func TestAssetLibraryUnsupportedOperation(t *testing.T) {
	r := tgxMaasResourceTestRouter(t, "http://127.0.0.1:1", "video-key", "model")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	setting := ch.GetSetting()
	setting.Protocol.AssetLibrary = &relaydto.AssetLibrarySettings{Backend: "task", AuthMode: "channel_key"}
	ch.SetSetting(setting)
	require.NoError(t, model.DB.Save(ch).Error)
	w := seedanceCall(r, "POST", "/v1/private-avatar/groups", `{"Name":"unsupported"}`, 1)
	require.Equal(t, http.StatusNotImplemented, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "unsupported_asset_operation")
}

func TestAssetLibraryChannelSaveAPI(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "test-persistent-secret")
	openConfigurableResourceTestDB(t)
	setting := `{"protocol":{"profile_id":"seedance2-ark-task-assets","project_name":"nmyk","asset_library":{"backend":"volcengine-assets","auth_mode":"aksk"}}}`
	baseURL := "https://video.example"
	ch := model.Channel{Type: 999, Name: "asset-save-test", Key: "video-key", BaseURL: &baseURL, Models: "seedance", Group: "default", Setting: &setting}
	payload := AddChannelRequest{Mode: "single", Channel: &ch, AssetCredentials: &model.AssetCredentials{AccessKeyID: "write-only-ak", SecretAccessKey: "write-only-sk"}}
	call := func(method string, body any, handler func(*gin.Context)) *httptest.ResponseRecorder {
		raw, err := common.Marshal(body)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(method, "/api/channel", strings.NewReader(string(raw)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("role", common.RoleRootUser)
		c.Set("id", 1)
		handler(c)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.True(t, gjson.Get(w.Body.String(), "success").Bool(), w.Body.String())
		require.NotContains(t, w.Body.String(), "write-only")
		require.NotContains(t, w.Body.String(), "asset_credentials")
		return w
	}
	call("POST", payload, AddChannel)
	var saved model.Channel
	require.NoError(t, model.DB.Where("name = ?", ch.Name).First(&saved).Error)
	require.NotEmpty(t, saved.AssetSecret)
	first := saved.AssetSecret
	decoded, err := saved.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, "write-only-sk", decoded.SecretAccessKey)
	update := map[string]any{"id": saved.Id, "type": 999, "name": "asset-save-updated", "models": "seedance", "group": "default", "base_url": baseURL, "setting": setting}
	call("PUT", update, UpdateChannel)
	require.NoError(t, model.DB.First(&saved, saved.Id).Error)
	require.Equal(t, first, saved.AssetSecret)
	update["asset_credentials"] = model.AssetCredentials{AccessKeyID: "replacement-ak", SecretAccessKey: "replacement-sk"}
	response := call("PUT", update, UpdateChannel)
	require.NotContains(t, response.Body.String(), "replacement-sk")
	require.NoError(t, model.DB.First(&saved, saved.Id).Error)
	decoded, err = saved.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, "replacement-sk", decoded.SecretAccessKey)
}

func TestAssetMetadataUpdatePreservesConcurrentCredentialRotation(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "review-only")
	openConfigurableResourceTestDB(t)
	url := "https://video.example"
	ch := &model.Channel{Type: 999, Key: "video-key", Name: "test", Models: "model", Group: "default", BaseURL: &url}
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas", ProjectName: "nmyk", AssetLibrary: &relaydto.AssetLibrarySettings{Backend: "tgxmaas", AuthMode: "api_key"}}})
	require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "old-asset-key"}))
	require.NoError(t, ch.Insert())
	snapshot, err := model.GetChannelById(ch.Id, true)
	require.NoError(t, err)
	// Request A has read the channel. Request B rotates the credential and commits.
	require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "rotated-asset-key"}))
	require.NoError(t, ch.Update())
	metadataUpdate := &model.Channel{Id: ch.Id, Name: "renamed"}
	require.NoError(t, prepareAssetCredentials(metadataUpdate, snapshot, nil))
	require.NoError(t, metadataUpdate.Update())
	actual, err := metadataUpdate.GetAssetCredentials()
	require.NoError(t, err)
	require.Equal(t, "rotated-asset-key", actual.APIKey)
}

func TestIndependentAssetFailurePreservesVideoHealth(t *testing.T) {
	for _, mode := range []string{"api_key", "aksk", "channel_key"} {
		t.Run(mode, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			t.Setenv("CRYPTO_SECRET", "test-persistent-secret")
			// Keep notifications disabled; routing key eligibility is checked below.
			previous := common.AutomaticDisableChannelEnabled
			common.AutomaticDisableChannelEnabled = false
			t.Cleanup(func() { common.AutomaticDisableChannelEnabled = previous })
			var calls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if mode == "api_key" {
					require.Equal(t, "Bearer asset-key", r.Header.Get("Authorization"))
				}
				if mode == "aksk" {
					require.Contains(t, r.Header.Get("Authorization"), "Credential=mock-ak/")
				}
				if mode == "channel_key" {
					require.Equal(t, "Bearer video-key", r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(401)
				_, _ = w.Write([]byte(`{"error":{"message":"invalid asset credential","code":"invalid_api_key"}}`))
			}))
			defer upstream.Close()
			r := tgxMaasResourceTestRouter(t, "http://video.example", "video-key", "model")
			require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
			strategy := model.RoutingStrategies()[0]
			require.NoError(t, model.UpsertRoutingStrategySnapshot(&model.RoutingStrategySnapshot{Strategy: strategy, UserGroup: "default", Groups: `["default"]`, Scores: `{}`, Config: `{}`}))
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			cfg := &relaydto.AssetLibrarySettings{Backend: "tgxmaas", AuthMode: mode, BaseURL: upstream.URL}
			if mode == "aksk" {
				cfg.Backend = configurable.OfficialAssetBackend
			}
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas", ProjectName: "nmyk", AssetLibrary: cfg}})
			require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "asset-key", AccessKeyID: "mock-ak", SecretAccessKey: "mock-sk"}))
			require.NoError(t, model.DB.Save(ch).Error)
			r.POST("/health-check", middleware.TokenAuth(), func(c *gin.Context) {
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "auto")
				common.SetContextKey(c, constant.ContextKeyTokenGroupPolicy, `{"type":"routing_strategy","strategy":"`+strategy+`"}`)
				c.Request.URL.Path = "/v1/private-avatar/groups"
				RelayConfigurableResource(c)
				require.True(t, service.ChannelRetryKeyAllowed(c, ch, "model", "video-key"), "asset failures must not mark video credentials unhealthy")
			})
			w := seedanceCall(r, "POST", "/health-check", `{"model":"model","Name":"mock"}`, 1)
			require.Equal(t, 401, w.Code, w.Body.String())
			require.Equal(t, 1, calls, "do not replay asset requests")
			latest, err := model.GetChannelById(ch.Id, true)
			require.NoError(t, err)
			require.Equal(t, common.ChannelStatusEnabled, latest.Status)
		})
	}
}

func TestAssetLibraryHealthScope(t *testing.T) {
	base := "https://video.example"
	ch := &model.Channel{BaseURL: &base}
	for _, tc := range []struct {
		backend, auth, url string
		independent        bool
	}{
		{"inherit", "", "", false}, {"tgxmaas", "channel_key", base + "/", false},
		{"tgxmaas", "channel_key", "https://assets.example", true},
		{"tgxmaas", "api_key", base, true}, {configurable.OfficialAssetBackend, "aksk", "", true},
	} {
		ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{AssetLibrary: &relaydto.AssetLibrarySettings{Backend: tc.backend, AuthMode: tc.auth, BaseURL: tc.url}}})
		require.Equal(t, tc.independent, assetLibraryHasIndependentHealth(ch))
	}
}

func TestIndependentAssetResponsesDoNotAffectVideoMetrics(t *testing.T) {
	for _, status := range []int{200, 401, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			t.Setenv("CRYPTO_SECRET", "test-persistent-secret")
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 200 {
					_, _ = w.Write([]byte(`{"Result":{"Items":[]}}`))
				} else {
					_, _ = w.Write([]byte(`{"error":{"message":"asset service failure"}}`))
				}
			}))
			defer upstream.Close()
			modelName := t.Name()
			r := tgxMaasResourceTestRouter(t, "http://video.example", "video-key", modelName)
			require.NoError(t, model.DB.AutoMigrate(&model.PerfMetric{}))
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas", ProjectName: "nmyk", AssetLibrary: &relaydto.AssetLibrarySettings{Backend: "tgxmaas", AuthMode: "api_key", BaseURL: upstream.URL}}})
			require.NoError(t, ch.SetAssetCredentials(&model.AssetCredentials{APIKey: "asset-key"}))
			require.NoError(t, model.DB.Save(ch).Error)
			body, err := common.Marshal(map[string]string{"model": modelName})
			require.NoError(t, err)
			w := seedanceCall(r, "POST", "/v1/private-avatar/groups/list", string(body), 1)
			require.Equal(t, status, w.Code, w.Body.String())
			stats, err := perfmetrics.Query(perfmetrics.QueryParams{Model: modelName})
			require.NoError(t, err)
			require.Empty(t, stats.Groups, "independent asset latency and failures must not change video routing scores")
		})
	}
}
