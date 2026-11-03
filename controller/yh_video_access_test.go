package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaydto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type draftChannelTestTransport func(*http.Request) (*http.Response, error)

func (f draftChannelTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestDraftAfterChannelReplacement(t *testing.T) {
	for _, tc := range []struct {
		name                                 string
		originalYH, selectedYH, removeOrigin bool
		wantStatus                           int
	}{
		{name: "official channel retained", wantStatus: http.StatusOK},
		{name: "official channel removed", removeOrigin: true, wantStatus: http.StatusOK},
		{name: "YH draft cannot escape through official channel", originalYH: true, removeOrigin: true, wantStatus: http.StatusNotFound},
		{name: "YH channel cannot reuse official draft", selectedYH: true, removeOrigin: true, wantStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			baseURL := "https://draft-test.invalid"
			var router http.Handler
			if tc.selectedYH {
				router = yhAssetTestRouter(t, baseURL)
			} else {
				router = seedanceTestRouter(t, baseURL, "same-upstream-account", tgxRegressionModel)
			}
			original, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			original.Id, original.Type = 21, constant.ChannelTypeVolcEngine
			original.SetSetting(relaydto.ChannelSettings{})
			if tc.originalYH {
				original.Type = constant.ChannelTypeConfigurable
				original.SetSetting(relaydto.ChannelSettings{Protocol: &relaydto.ChannelProtocolSettings{ProfileID: "seedance-yh"}})
			}
			require.NoError(t, model.DB.Create(original).Error)
			require.NoError(t, model.DB.Create(&model.Task{
				TaskID: "task-original-draft", UserId: 1, ChannelId: 21, Status: model.TaskStatusSuccess,
				PrivateData: model.TaskPrivateData{Key: original.Key, UpstreamTaskID: "cgt-original-draft", SeedanceDraft: tc.originalYH},
			}).Error)
			if tc.removeOrigin {
				require.NoError(t, model.DB.Delete(&model.Channel{}, 21).Error)
			}
			client := service.GetHttpClient()
			oldTransport := client.Transport
			t.Cleanup(func() { client.Transport = oldTransport })
			calls := 0
			client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				require.NoError(t, err)
				require.Equal(t, "/api/v3/contents/generations/tasks", req.URL.Path)
				require.Equal(t, "cgt-original-draft", gjson.GetBytes(body, "content.0.draft_task.id").String())
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"cgt-new-video"}`)), Request: req}, nil
			})
			var before model.User
			require.NoError(t, model.DB.First(&before, 1).Error)
			body := fmt.Sprintf(`{"model":%q,"content":[{"type":"draft_task","draft_task":{"id":"cgt-original-draft"}}]}`, tgxRegressionModel)
			out := seedanceCall(router, http.MethodPost, "/api/v3/contents/generations/tasks", body, 1)
			require.Equal(t, tc.wantStatus, out.Code, out.Body.String())
			if tc.wantStatus == http.StatusOK {
				require.Equal(t, 1, calls)
			} else {
				require.Zero(t, calls, "unavailable YH drafts must be rejected before calling upstream")
				var after model.User
				require.NoError(t, model.DB.First(&after, 1).Error)
				require.Equal(t, before.Quota, after.Quota)
			}
		})
	}
}

func TestYHDraftRetryToOfficialAfterChannelReplacement(t *testing.T) {
	router := yhAssetTestRouter(t, "https://draft-retry.invalid")
	common.RetryTimes = 1 // Restored by the router fixture.
	oldRules := operation_setting.AutomaticRetryPolicyRules
	t.Cleanup(func() { operation_setting.AutomaticRetryPolicyRules = oldRules })
	operation_setting.AutomaticRetryPolicyRules = []operation_setting.RetryPolicyRule{{Name: "retry 503", Action: operation_setting.RetryPolicyActionRetry, StatusCodes: "503"}}
	original, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	official := *original
	official.Id, official.Type = 21, constant.ChannelTypeVolcEngine
	official.SetSetting(relaydto.ChannelSettings{})
	require.NoError(t, model.DB.Create(&official).Error)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 20).Update("priority", 100).Error)
	createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 1)
	client := service.GetHttpClient()
	oldTransport := client.Transport
	t.Cleanup(func() { client.Transport = oldTransport })
	var calls []string
	client.Transport = draftChannelTestTransport(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		status, payload := http.StatusOK, `{"id":"cgt-official-after-retry"}`
		body, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		if req.URL.Path == "/v1/video/generations" {
			require.True(t, gjson.GetBytes(body, "metadata.draft").Bool())
			status, payload = http.StatusServiceUnavailable, `{"error":{"code":"service_unavailable","message":"mock unavailable"}}`
		} else {
			require.Equal(t, "/api/v3/contents/generations/tasks", req.URL.Path)
			if len(calls) == 2 {
				require.True(t, gjson.GetBytes(body, "draft").Bool())
			} else {
				require.Equal(t, "cgt-official-after-retry", gjson.GetBytes(body, "content.0.draft_task.id").String())
				payload = `{"id":"cgt-upgraded-official"}`
			}
		}
		return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(payload)), Request: req}, nil
	})
	body := `{"model":"` + tgxRegressionModel + `","draft":true,"content":[{"type":"text","text":"a scene"}]}`
	out := seedanceCall(router, http.MethodPost, "/api/v3/contents/generations/tasks", body, 1)
	require.Equal(t, http.StatusOK, out.Code, out.Body.String())
	require.Equal(t, []string{"/v1/video/generations", "/api/v3/contents/generations/tasks"}, calls)
	saved, exists, err := model.GetByTaskIDOrUpstreamID(1, gjson.GetBytes(out.Body.Bytes(), "id").String())
	require.NoError(t, err)
	require.True(t, exists)
	require.Equal(t, 21, saved.ChannelId)
	require.False(t, saved.PrivateData.SeedanceDraft, "official retry must not inherit a YH-specific marker")
	// The upstream account still owns the completed draft after replacing its
	// local official channel. Reusing it must reach that replacement channel.
	require.NoError(t, model.DB.Model(saved).Update("status", model.TaskStatusSuccess).Error)
	require.NoError(t, model.DB.Delete(&model.Channel{}, 21).Error)
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id IN ?", []int{20, 21}).Update("enabled", false).Error)
	official.Id = 22
	require.NoError(t, model.DB.Create(&official).Error)
	createConfigurableResourceAbility(t, model.DB, 22, tgxRegressionModel, true, 1)
	body = `{"model":"` + tgxRegressionModel + `","content":[{"type":"draft_task","draft_task":{"id":"cgt-official-after-retry"}}]}`
	upgraded := seedanceCall(router, http.MethodPost, "/api/v3/contents/generations/tasks", body, 1)
	require.Equal(t, http.StatusOK, upgraded.Code, upgraded.Body.String())
	require.Equal(t, "cgt-upgraded-official", gjson.GetBytes(upgraded.Body.Bytes(), "id").String())
	require.Len(t, calls, 3, "draft upgrade should reach the replacement official channel")
}

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
