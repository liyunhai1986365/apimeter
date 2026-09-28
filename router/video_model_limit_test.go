package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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
)

func TestVideoTaskFetchModelLimitsProductionRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db := openChannelRouteAuthTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Token{}, &model.Log{}))
	oldCache := common.MemoryCacheEnabled
	oldGroups := setting.UserUsableGroups2JSONString()
	oldRatios := ratio_setting.GroupRatio2JSONString()
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldCache
		_ = setting.UpdateUserUsableGroupsByJSONString(oldGroups)
		_ = ratio_setting.UpdateGroupRatioByJSONString(oldRatios)
	})
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	service.InitHttpClient()
	const allowedModel = "doubao-seedance-2-5-260628"
	for _, user := range []model.User{
		{Id: 1, Username: "review-owner", AffCode: "rv1", Role: common.RoleCommonUser, Group: "default", Status: common.UserStatusEnabled, Quota: 1000000},
		{Id: 2, Username: "review-foreign", AffCode: "rv2", Role: common.RoleCommonUser, Group: "default", Status: common.UserStatusEnabled, Quota: 1000000},
	} {
		require.NoError(t, db.Create(&user).Error)
	}
	for _, token := range []model.Token{
		{Id: 1, UserId: 1, Key: "reviewowner", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: allowedModel},
		{Id: 2, UserId: 2, Key: "reviewforeign", Group: "default", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: allowedModel},
	} {
		require.NoError(t, db.Create(&token).Error)
	}
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		require.Equal(t, "GET", r.Method)
		require.True(t, strings.HasSuffix(r.URL.Path, "/contents/generations/tasks/cgt-review"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"cgt-review","status":"running"}`))
	}))
	defer upstream.Close()
	ch := &model.Channel{Id: 20, Type: constant.ChannelTypeConfigurable, Status: common.ChannelStatusEnabled, Name: "review", BaseURL: &upstream.URL, Key: "mock", Group: "default", Models: allowedModel}
	ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "seedance-tgxmaas"}})
	require.NoError(t, db.Create(ch).Error)
	require.NoError(t, db.Create(&model.Ability{ChannelId: 20, Group: "default", Model: allowedModel, Enabled: true}).Error)
	task := &model.Task{TaskID: "task_review", UserId: 1, ChannelId: 20, Platform: "999", Status: model.TaskStatusInProgress, Properties: model.Properties{OriginModelName: allowedModel}, PrivateData: model.TaskPrivateData{UpstreamTaskID: "cgt-review"}}
	require.NoError(t, db.Create(task).Error)
	r := gin.New()
	r.Use(gin.Recovery())
	SetVideoRouter(r)
	for _, path := range []string{"/api/v3/contents/generations/tasks/cgt-review", "/v1/videos/task_review", "/v1/video/generations/task_review"} {
		for _, tc := range []struct {
			name, key, limits string
			enabled           bool
			status            int
			dispatch          bool
		}{
			{"authorized", "reviewowner", allowedModel, true, 200, true},
			{"forbidden", "reviewowner", "unrelated", true, 403, false},
			{"empty", "reviewowner", "", true, 403, false},
			{"foreign", "reviewforeign", allowedModel, true, 400, false},
			{"forged-channel", "reviewowner-20", "unrelated", true, 403, false},
			{"unrestricted", "reviewowner", "", false, 200, true},
		} {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": tc.enabled, "model_limits": tc.limits}).Error)
				before := upstreamCalls.Load()
				req := httptest.NewRequest(http.MethodGet, path+"?model=unrelated", strings.NewReader(`{"model":"unrelated"}`))
				req.Header.Set("Authorization", "Bearer "+tc.key)
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				require.Equal(t, tc.status, w.Code, w.Body.String())
				if tc.dispatch {
					require.Equal(t, before+1, upstreamCalls.Load())
				} else {
					require.Equal(t, before, upstreamCalls.Load())
				}
			})
		}
	}
}
