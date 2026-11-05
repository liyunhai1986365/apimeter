package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetVideoPreservesAuthorizedBillingGroup(t *testing.T) {
	for _, backend := range []string{"yh", "tgxmaas", configurable.OfficialAssetBackend} {
		for _, mode := range []string{"fixed", "auto", "explicit_vip", "ordered_fallback", "removed", "disabled"} {
			t.Run(backend+"/"+mode, func(t *testing.T) {
				var router *gin.Engine
				if backend == "yh" {
					router = yhAssetTestRouter(t, "https://asset-group.invalid")
				} else {
					router = assetSecurityRouter(t, "https://asset-group.invalid", backend, "")
				}
				seedAssetOwnershipForTest(t, 20, 1, "asset", "asset-owned")
				ch, err := model.GetChannelById(20, true)
				require.NoError(t, err)
				ch.Group = "vip,default"
				reject := mode == "removed" || mode == "disabled"
				ability := model.DB.Where("channel_id = ? AND "+model.CommonGroupCol()+" = ?", 20, "default")
				if mode == "removed" || mode == "ordered_fallback" {
					ch.Group = "vip"
					require.NoError(t, ability.Delete(&model.Ability{}).Error)
				} else if mode == "disabled" {
					require.NoError(t, ability.Model(&model.Ability{}).Update("enabled", false).Error)
				}
				require.NoError(t, model.DB.Save(ch).Error)
				require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: tgxRegressionModel, ChannelId: 20, Enabled: true}).Error)
				if reject {
					// Distribution still finds a default channel. Asset ownership
					// must not override that authorization by moving to vip.
					alternate := *ch
					alternate.Id, alternate.Group = 21, "default"
					require.NoError(t, model.DB.Create(&alternate).Error)
					createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
				}
				require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":3}`))
				group, ratio := "default", 1.0
				if mode == "explicit_vip" || mode == "ordered_fallback" {
					group, ratio = "vip", 3
					require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","vip":"vip"}`))
				}
				tokenGroup := group
				if mode == "ordered_fallback" {
					// A policy-authorized fallback selected by distribution remains
					// valid; only the later asset-driven group switch is forbidden.
					tokenGroup = "default"
					require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Update("group_policy", `{"type":"ordered","groups":["default","vip"]}`).Error)
				}
				if mode == "auto" {
					tokenGroup = "auto"
					original := setting.AutoGroups2JsonString()
					t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(original)) })
					require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
				}
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": tokenGroup, "auto_groups": `["default"]`}).Error)
				client := service.GetHttpClient()
				old := client.Transport
				t.Cleanup(func() { client.Transport = old })
				calls := 0
				client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
					calls++
					response := `{"id":"cgt-asset-group"}`
					if backend == "yh" {
						response = `{"task_id":"cgt-asset-group","status":"SUBMITTED"}`
					}
					return yhHTTPResponse(req, 200, []byte(response), nil), nil
				})
				var before, after model.User
				require.NoError(t, model.DB.First(&before, 1).Error)
				body := `{"model":"` + tgxRegressionModel + `","content":[{"type":"text","text":"test"},{"type":"image_url","image_url":{"url":"asset://asset-owned"},"role":"reference_image"}]}`
				out := seedanceCall(router, http.MethodPost, "/api/v3/contents/generations/tasks", body, 1)
				if reject {
					require.Equal(t, http.StatusNotFound, out.Code, out.Body.String())
					require.Zero(t, calls)
					require.NoError(t, model.DB.First(&after, 1).Error)
					require.Equal(t, before.Quota, after.Quota)
					var count int64
					require.NoError(t, model.DB.Model(&model.Task{}).Count(&count).Error)
					require.Zero(t, count)
					return
				}
				require.Equal(t, http.StatusOK, out.Code, out.Body.String())
				require.Equal(t, 1, calls)
				task, exists, err := model.GetByTaskIDOrUpstreamID(1, gjson.GetBytes(out.Body.Bytes(), "id").String())
				require.NoError(t, err)
				require.True(t, exists)
				require.Equal(t, group, task.Group)
				require.Equal(t, ratio, task.PrivateData.BillingContext.GroupRatio)
				require.Equal(t, int(500*ratio), task.Quota)
			})
		}
	}
}
