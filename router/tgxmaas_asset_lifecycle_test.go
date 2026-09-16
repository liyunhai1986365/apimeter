package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

// Exercise the actual API/dashboard/relay/video registration order, authenticated
// requests and channel 14 selection, rather than manually registering aliases.
func TestTgxMaasAssetAliasesLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db := openChannelRouteAuthTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.RetryRouteEvent{}, &model.ConfigurableResourceState{}))
	oldCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	oldGroups := setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldCache
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	service.InitHttpClient()
	const modelName = "doubao-seedance-2-5-260628"
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "asset-user", Group: "default", Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Quota: 1000000}).Error)
	require.NoError(t, db.Create(&model.Token{Id: 1, UserId: 1, Key: "assettesttoken", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	groupCreated, assetCreated := false, false
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		require.Equal(t, "Bearer supplier-key", r.Header.Get("Authorization"))
		raw, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "POST /v1/private-avatar/groups":
			require.Equal(t, "sample group", gjson.GetBytes(raw, "Name").String())
			groupCreated = true
			_, _ = w.Write([]byte(`{"Result":{"Id":"ag_local","Name":"sample group"}}`))
		case "POST /v1/private-avatar/assets":
			require.True(t, groupCreated)
			require.Equal(t, "ag_local", gjson.GetBytes(raw, "GroupId").String())
			require.Equal(t, "Image", gjson.GetBytes(raw, "AssetType").String())
			require.Equal(t, modelName, gjson.GetBytes(raw, "model").String())
			assetCreated = true
			_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local","upstream_asset_id":"asset-202609-test","GroupId":"ag_local","Status":"Processing"}}`))
		case "GET /v1/private-avatar/assets/asset_local":
			require.True(t, assetCreated)
			require.Empty(t, raw)
			_, _ = w.Write([]byte(`{"Result":{"Id":"asset_local","upstream_asset_id":"asset-202609-test","GroupId":"ag_local","Status":"Active"}}`))
		case "GET /v1/private-avatar/groups/ag_local":
			require.True(t, groupCreated)
			_, _ = w.Write([]byte(`{"Result":{"Id":"ag_local","Name":"sample group"}}`))
		default:
			t.Errorf("unexpected upstream request %s %s", r.Method, r.URL.String())
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	ch := model.Channel{Id: 14, Type: constant.ChannelTypeConfigurable, Status: common.ChannelStatusEnabled, Name: "TgxMaas", Group: "default", Models: modelName, Key: "supplier-key", BaseURL: &upstream.URL}
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
	require.NoError(t, db.Create(&ch).Error)
	require.NoError(t, db.Create(&model.Ability{Group: "default", Model: modelName, ChannelId: 14, Enabled: true}).Error)
	r := gin.New()
	SetApiRouter(r)
	SetDashboardRouter(r)
	SetRelayRouter(r)
	SetVideoRouter(r)
	call := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer assettesttoken")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	for _, groupPath := range []string{"/api/asset-groups", "/v1/asset-groups"} {
		for _, assetPath := range []string{"/api/assets", "/v1/assets"} {
			t.Run(groupPath+assetPath, func(t *testing.T) {
				groupCreated, assetCreated = false, false
				w := call("POST", groupPath, `{"name":"sample group"}`)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, "ag_local", gjson.GetBytes(w.Body.Bytes(), "Result.Id").String())
				w = call("POST", assetPath, `{"group_id":"ag_local","url":"https://example.com/a.png","asset_type":"Image"}`)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, "asset-202609-test", gjson.GetBytes(w.Body.Bytes(), "Result.upstream_asset_id").String())
				for _, fetch := range []struct{ method, path, body string }{
					{"GET", "/api/assets/asset_local", ""},
					{"GET", "/v1/assets/asset_local", ""},
					{"POST", "/v1/assets/get", `{"asset_id":"asset_local"}`},
					{"POST", "/v1/assets/get", `{"Id":"asset_local"}`},
					{"GET", "/v1/assets/get?asset_id=asset_local", ""},
				} {
					w = call(fetch.method, fetch.path, fetch.body)
					require.Equal(t, 200, w.Code, w.Body.String())
					require.Equal(t, "asset_local", gjson.GetBytes(w.Body.Bytes(), "Result.Id").String())
					require.Equal(t, "asset-202609-test", gjson.GetBytes(w.Body.Bytes(), "Result.upstream_asset_id").String())
					require.Equal(t, "Active", gjson.GetBytes(w.Body.Bytes(), "Result.Status").String())
					require.Equal(t, "ag_local", gjson.GetBytes(w.Body.Bytes(), "Result.GroupId").String())
				}
				w = call("GET", groupPath+"/ag_local", "")
				require.Equal(t, 200, w.Code, w.Body.String())
				require.Equal(t, "ag_local", gjson.GetBytes(w.Body.Bytes(), "Result.Id").String())
			})
		}
	}
	before := calls
	for _, body := range []string{`{}`, `{"asset_id":"../escape"}`, `{"asset_id":"asset_local","Id":"different"}`, `{"asset_id":123}`} {
		w := call("POST", "/v1/assets/get", body)
		require.Equal(t, 400, w.Code, w.Body.String())
	}
	require.Equal(t, before, calls)
	require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": modelName}).Error)
	w := call("POST", "/v1/assets/get", `{"asset_id":"asset_local"}`)
	require.Equal(t, 403, w.Code, w.Body.String())
	require.Equal(t, before, calls)
	w = call("POST", "/v1/assets/get", `{"asset_id":"asset_local","model":"`+modelName+`"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
}
