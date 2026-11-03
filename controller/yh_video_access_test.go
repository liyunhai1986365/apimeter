package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestYHDraftPreservesSelectedBillingGroup(t *testing.T) {
	for _, tc := range []struct {
		name, tokenGroup, channelGroups, wantGroup string
		wantRatio                                  float64
	}{
		{"fixed", "default", "vip,default", "default", 1},
		{"auto", "auto", "vip,default", "default", 1},
		{"explicit_vip", "vip", "default,vip", "vip", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var creates atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				require.Equal(t, "/v1/video/generations", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"task_id":"cgt-group-%d","status":"SUBMITTED"}`, creates.Add(1))
			}))
			defer upstream.Close()
			r := yhAssetTestRouter(t, upstream.URL)
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.Group = tc.channelGroups
			require.NoError(t, model.DB.Save(ch).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: tgxRegressionModel, ChannelId: 20, Enabled: true, Priority: common.GetPointer(int64(1))}).Error)
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":3}`))
			if tc.wantGroup == "vip" {
				require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","vip":"vip"}`))
			}
			if tc.tokenGroup == "auto" {
				original := setting.AutoGroups2JsonString()
				t.Cleanup(func() { require.NoError(t, setting.UpdateAutoGroupsByJsonString(original)) })
				require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default"]`))
			}
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"group": tc.tokenGroup, "auto_groups": `["default"]`}).Error)
			require.NoError(t, model.DB.Create(&model.Task{
				TaskID: "task-draft-group", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess,
				Properties:  model.Properties{OriginModelName: tgxRegressionModel},
				PrivateData: model.TaskPrivateData{SeedanceDraft: true, Key: "yh-test-key", UpstreamTaskID: "cgt-draft-group"},
			}).Error)
			for _, request := range []struct{ name, content string }{
				{"ordinary", `[{"type":"text","text":"a scene"}]`},
				{"draft", `[{"type":"draft_task","draft_task":{"id":"task-draft-group"}}]`},
			} {
				t.Run(request.name, func(t *testing.T) {
					body := `{"model":"` + tgxRegressionModel + `","content":` + request.content + `}`
					w := seedanceCall(r, http.MethodPost, "/api/v3/contents/generations/tasks", body, 1)
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					task, exists, err := model.GetByTaskIDOrUpstreamID(1, gjson.GetBytes(w.Body.Bytes(), "id").String())
					require.NoError(t, err)
					require.True(t, exists)
					require.Equal(t, tc.wantGroup, task.Group)
					require.NotNil(t, task.PrivateData.BillingContext)
					require.Equal(t, tc.wantRatio, task.PrivateData.BillingContext.GroupRatio)
					require.Equal(t, int(500*tc.wantRatio), task.Quota)
				})
			}
			require.EqualValues(t, 2, creates.Load())
		})
	}
}

func TestYHDraftRejectsUnavailableSelectedGroup(t *testing.T) {
	for _, unavailable := range []string{"removed", "disabled"} {
		t.Run(unavailable, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"task_id":"cgt-unexpected","status":"SUBMITTED"}`))
			}))
			defer upstream.Close()
			r := yhAssetTestRouter(t, upstream.URL)
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.Group = "vip,default"
			ability := model.DB.Where("channel_id = ? AND "+model.CommonGroupCol()+" = ?", 20, "default")
			if unavailable == "removed" {
				ch.Group = "vip"
				require.NoError(t, ability.Delete(&model.Ability{}).Error)
			} else {
				require.NoError(t, ability.Model(&model.Ability{}).Update("enabled", false).Error)
			}
			require.NoError(t, model.DB.Save(ch).Error)
			require.NoError(t, model.DB.Create(&model.Ability{Group: "vip", Model: tgxRegressionModel, ChannelId: 20, Enabled: true, Priority: common.GetPointer(int64(1))}).Error)
			// Distribution can select another channel in default, but ownership
			// must not move the request to the draft channel's vip group.
			alternative := *ch
			alternative.Id, alternative.Group = 21, "default"
			require.NoError(t, model.DB.Create(&alternative).Error)
			createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":3}`))
			require.NoError(t, model.DB.Create(&model.Task{
				TaskID: "task-draft-group", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess,
				Properties:  model.Properties{OriginModelName: tgxRegressionModel},
				PrivateData: model.TaskPrivateData{SeedanceDraft: true, Key: "yh-test-key", UpstreamTaskID: "cgt-draft-group"},
			}).Error)
			var before, after model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			body := `{"model":"` + tgxRegressionModel + `","content":[{"type":"draft_task","draft_task":{"id":"task-draft-group"}}]}`
			w := seedanceCall(r, http.MethodPost, "/api/v3/contents/generations/tasks", body, 1)
			require.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
			require.Zero(t, calls.Load(), "inaccessible draft groups must be rejected before calling upstream")
			require.NoError(t, model.DB.First(&after, 1).Error)
			require.Equal(t, before.Quota, after.Quota)
			var tasks int64
			require.NoError(t, model.DB.Model(&model.Task{}).Count(&tasks).Error)
			require.EqualValues(t, 1, tasks, "a rejected upgrade must not create another task")
		})
	}
}
