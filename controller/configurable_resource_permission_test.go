package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFixedModelResourceAllowsRestrictedToken(t *testing.T) {
	for _, smart := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed", true: "smart"}[smart], func(t *testing.T) {

			gin.SetMode(gin.TestMode)
			db := openConfigurableResourceTestDB(t)
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"默认分组"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
			originalModelPrice := ratio_setting.ModelPrice2JSONString()
			require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"kling-3.0-turbo":0.002}`))
			t.Cleanup(func() {
				require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(originalModelPrice))
			})

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/text-to-video/kling-3.0-turbo", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"message":"SUCCEED","data":{"id":"901349125802336344","status":"submitted"}}`))
			}))
			defer upstream.Close()

			require.NoError(t, db.Create(&model.User{Id: 10, Username: "resource-user", Password: "password", Group: "default", Quota: 100000, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
			require.NoError(t, db.Create(&model.Token{Id: 1, UserId: 10, Name: "resource-token", Key: "resourcetokenkey", ModelLimitsEnabled: true, ModelLimits: "kling-3.0-turbo", Status: common.TokenStatusEnabled, CreatedTime: 1, ExpiredTime: -1, RemainQuota: 100000, Group: "default"}).Error)

			channel := &model.Channel{Id: 20, Type: constant.ChannelTypeConfigurable, Key: "upstream-secret", Status: common.ChannelStatusEnabled, Name: "kling-video", Group: "default", Models: "kling-3.0-turbo", BaseURL: &upstream.URL, CreatedTime: common.GetTimestamp()}
			channel.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "kling-video"}})
			require.NoError(t, db.Create(channel).Error)
			createConfigurableResourceAbility(t, db, channel.Id, "kling-3.0-turbo", true, 0)

			if smart {
				oldAuto := setting.AutoGroups2JsonString()
				t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto)) })
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
				require.NoError(t, db.AutoMigrate(&model.RoutingStrategySnapshot{}, &model.RetryRouteEvent{}))
				require.NoError(t, db.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + model.RoutingStrategies()[0] + `"}`}).Error)
			}

			router := gin.New()
			router.POST("/kling/text-to-video/kling-3.0-turbo", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/kling/text-to-video/kling-3.0-turbo", strings.NewReader(`{"prompt":"A train window shot","settings":{"duration":3}}`))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer resourcetokenkey")
			router.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.JSONEq(t, `{"code":0,"message":"SUCCEED","data":{"id":"901349125802336344","status":"submitted"}}`, recorder.Body.String())

			var task model.Task
			require.NoError(t, db.First(&task).Error)
			require.Equal(t, "901349125802336344", task.PrivateData.UpstreamTaskID)
			require.Equal(t, constant.TaskPlatform("999"), task.Platform)
			require.Equal(t, "kling-turbo", task.Action)
			require.Equal(t, string(model.TaskStatusSubmitted), string(task.Status))
			require.Equal(t, "10%", task.Progress)
			require.Equal(t, "kling-3.0-turbo", task.Properties.OriginModelName)
			require.Equal(t, 20, task.ChannelId)
			require.Equal(t, 1, task.TokenId)
			require.NotZero(t, task.CreatedAt)
			require.NotZero(t, task.UpdatedAt)

			var consumeLog model.Log
			require.NoError(t, db.Where("type = ?", model.LogTypeConsume).First(&consumeLog).Error)
			require.Equal(t, "kling-3.0-turbo", consumeLog.ModelName)
			require.Equal(t, 20, consumeLog.ChannelId)
			require.Equal(t, 1, consumeLog.TokenId)
			require.Equal(t, 1000, consumeLog.Quota)

		})
	}
}

func TestFixedModelResourceRejectsSpoofedModelAcrossRoutes(t *testing.T) {
	for _, smart := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed", true: "smart"}[smart], func(t *testing.T) {
			for _, body := range []string{`{"prompt":"test"}`, `{"model":"allowed-other","prompt":"test"}`, `{"model_name":"allowed-other","prompt":"test"}`} {
				t.Run(body, func(t *testing.T) {
					var calls int
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
					defer upstream.Close()
					r := seedanceTestRouter(t, upstream.URL, "test", "kling-3.0-turbo")
					ch, err := model.GetChannelById(20, true)
					require.NoError(t, err)
					ch.Type = constant.ChannelTypeConfigurable
					ch.SetSetting(dto.ChannelSettings{Protocol: &dto.ChannelProtocolSettings{ProfileID: "kling-video"}})
					require.NoError(t, model.DB.Save(ch).Error)
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": "allowed-other"}).Error)
					if smart {
						oldAuto := setting.AutoGroups2JsonString()
						t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAuto)) })
						require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
						require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
						require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": "auto", "group_policy": `{"type":"routing_strategy","strategy":"` + model.RoutingStrategies()[0] + `"}`}).Error)
					}
					r.POST("/kling/text-to-video/kling-3.0-turbo", middleware.ConfigurableResource("", ""), middleware.TokenAuth(), RelayConfigurableResource)
					response := seedanceCall(r, "POST", "/kling/text-to-video/kling-3.0-turbo", body, 1)
					require.Equal(t, 403, response.Code, response.Body.String())
					require.Zero(t, calls)
				})
			}
		})
	}
}
