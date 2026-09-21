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
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetUserIsolationSharesCRUDWithAnotherToken(t *testing.T) {
	for _, backend := range []string{"youniyouju", "tgxmaas", "volcengine-assets"} {
		t.Run(backend, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if backend == "tgxmaas" && r.Method == http.MethodPost && r.URL.Path == "/v1/private-avatar/assets" {
					body, err := io.ReadAll(r.Body)
					if err != nil || gjson.GetBytes(body, "model").String() != tgxRegressionModel {
						w.WriteHeader(http.StatusBadRequest)
						_, _ = w.Write([]byte(`{"error":{"message":"expected the asset channel's model"}}`))
						return
					}
				}
				switch {
				case r.URL.Query().Get("Action") == "ListAssets" || strings.HasSuffix(r.URL.Path, "/list"):
					_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"owned"},{"Id":"foreign"}],"TotalCount":2}}`))
				case r.URL.Query().Get("Action") == "DeleteAsset" || r.Method == "DELETE":
					w.WriteHeader(http.StatusNoContent)
				default:
					_, _ = w.Write([]byte(`{"Result":{"Id":"owned"}}`))
				}
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, backend, "")
			seedAssetOwnershipForTest(t, 20, 1, "group", "own-group")
			seedAssetOwnershipForTest(t, 20, 2, "asset", "foreign")
			created := seedanceCall(r, "POST", "/api/assets", `{"GroupId":"own-group","URL":"https://example.com/a.png"}`, 1)
			require.Equal(t, http.StatusOK, created.Code, created.Body.String())

			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","alternate":"alternate"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"alternate":1}`))
			// Same user, different token, unrelated model limit and channel group.
			// There is intentionally no channel in this token's group.
			require.NoError(t, model.DB.Create(&model.Token{Id: 3, UserId: 1, Key: "seedancetest3", Name: "second-token", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "alternate", ModelLimitsEnabled: true, ModelLimits: "unrelated-model"}).Error)
			for _, policy := range []map[string]any{
				{"group": "auto", "auto_groups": `["alternate"]`, "group_policy": ""},
				{"group": "alternate", "auto_groups": "", "group_policy": `{"type":"ordered","groups":["alternate","default"]}`},
				{"group": "auto", "auto_groups": "", "group_policy": fmt.Sprintf(`{"type":"routing_strategy","strategy":%q,"excluded_groups":["default"]}`, model.RoutingStrategies()[0])},
				{"group": "alternate", "auto_groups": "", "group_policy": ""},
			} {
				require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 3).Updates(policy).Error)
				got := seedanceCall(r, "GET", "/api/assets/owned", "", 3)
				require.Equal(t, http.StatusOK, got.Code, got.Body.String())
				for _, path := range []string{"/api/assets", "/?Action=CreateAsset&Version=2024-01-01"} {
					created := seedanceCall(r, "POST", path, `{"GroupId":"own-group","URL":"https://example.com/a.png"}`, 3)
					require.Equal(t, http.StatusOK, created.Code, "same-user creation must supply the channel model for every token routing policy: %s", created.Body.String())
				}
			}
			for _, tc := range []struct{ method, path, body string }{
				{"GET", "/api/assets/owned", ""},
				{"PATCH", "/api/assets/owned", `{"Name":"renamed"}`},
				{"GET", "/api/assets", `{"Filter":{"GroupIds":["own-group"]}}`},
				{"POST", "/api/assets", `{"GroupId":"own-group","URL":"https://example.com/a.png"}`},
			} {
				before := calls.Load()
				got := seedanceCall(r, tc.method, tc.path, tc.body, 3)
				require.Equal(t, http.StatusOK, got.Code, got.Body.String())
				require.Equal(t, before+1, calls.Load())
				require.NotContains(t, got.Body.String(), "foreign")
				denied := seedanceCall(r, tc.method, tc.path, tc.body, 2)
				require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
				require.Equal(t, before+1, calls.Load(), "another user's request must not reach upstream")
			}
			deleted := seedanceCall(r, "DELETE", "/api/assets/owned", "", 3)
			require.Equal(t, http.StatusNoContent, deleted.Code, deleted.Body.String())
			before := calls.Load()
			for _, token := range []int{1, 2, 3} {
				got := seedanceCall(r, "GET", "/api/assets/owned", "", token)
				require.Equal(t, http.StatusNotFound, got.Code, got.Body.String())
			}
			require.Equal(t, before, calls.Load(), "deletion revokes access for every token")
		})
	}
}

func TestAssetUserIsolationVideoIgnoresAssetTokenGroups(t *testing.T) {
	for _, policy := range []string{"fixed", "auto", "ordered", "smart"} {
		t.Run(policy, func(t *testing.T) {
			preserveSmartRetryTestConfiguration(t)
			var assetCalls, ordinaryCalls atomic.Int32
			assets := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assetCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"owned-video-task","status":"queued"}`))
			}))
			defer assets.Close()
			ordinary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ordinaryCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"ordinary-video-task","status":"queued"}`))
			}))
			defer ordinary.Close()
			r := assetSecurityRouter(t, assets.URL, "tgxmaas", "seedance-tgxmaas")
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","primary":"primary","backup":"backup"}`))
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"primary":1,"backup":2}`))
			require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["primary","backup"]`))
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 2).Update("group", "primary").Error)
			second := &model.Token{Id: 3, UserId: 1, Key: "seedancetest3", Name: "second-token", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "primary", ModelLimitsEnabled: true, ModelLimits: tgxRegressionModel, CrossGroupRetry: true}
			switch policy {
			case "auto":
				second.Group, second.AutoGroups = "auto", `["primary"]`
			case "ordered":
				second.GroupPolicy = `{"type":"ordered","groups":["primary","backup"]}`
			case "smart":
				second.Group = "auto"
				second.GroupPolicy = fmt.Sprintf(`{"type":"routing_strategy","strategy":%q,"excluded_groups":["backup"]}`, model.RoutingStrategies()[0])
				require.NoError(t, model.DB.AutoMigrate(&model.RoutingStrategySnapshot{}))
			}
			require.NoError(t, model.DB.Create(second).Error)
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			ch.Group = "backup"
			require.NoError(t, model.DB.Save(ch).Error)
			require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", 20).Update("group", "backup").Error)
			seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
			ch.Id, ch.Group, ch.BaseURL = 21, "primary", &ordinary.URL
			require.NoError(t, model.DB.Create(ch).Error)
			require.NoError(t, model.DB.Create(&model.Ability{ChannelId: 21, Group: "primary", Model: tgxRegressionModel, Enabled: true, Priority: common.GetPointer(int64(10))}).Error)

			plain := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","content":[{"type":"text","text":"hello"}],"duration":4}`, 3)
			require.Equal(t, http.StatusOK, plain.Code, plain.Body.String())
			require.EqualValues(t, 1, ordinaryCalls.Load())
			require.Zero(t, assetCalls.Load(), "ordinary video still follows the token's routing policy")
			payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"image_url","image_url":{"url":"asset://owned"}}],"duration":4}`
			denied := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 2)
			require.Equal(t, http.StatusNotFound, denied.Code, denied.Body.String())
			require.Zero(t, assetCalls.Load())
			got := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 3)
			require.Equal(t, http.StatusOK, got.Code, got.Body.String())
			require.EqualValues(t, 1, assetCalls.Load())
			var task model.Task
			require.NoError(t, model.DB.Where("channel_id = ?", 20).First(&task).Error)
			require.Equal(t, 1, task.UserId)
			require.Equal(t, 3, task.TokenId)
			require.Equal(t, float64(2), task.PrivateData.BillingContext.GroupRatio, "bill the actual asset channel's group")

			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 3).Update("model_limits", "unrelated-model").Error)
			denied = seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 3)
			require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
			require.EqualValues(t, 1, assetCalls.Load(), "asset ownership does not grant permission to a video model")
		})
	}
}
