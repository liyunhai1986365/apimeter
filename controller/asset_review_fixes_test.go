package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetResponseImplicitProjectSurvivesDefaultChange(t *testing.T) {
	var queriedProject string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Query().Get("Action") == "GetAssetGroup" {
			queriedProject = gjson.GetBytes(body, "ProjectName").String()
			if queriedProject != "platform-project" {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"ResponseMetadata":{"Error":{"Code":"NotFound"}}}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{"Result":{"Id":"group-implicit","ProjectName":"platform-project"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	settings := ch.GetSetting()
	settings.Protocol.ProjectName = ""
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	created := assetSecurityAction(r, "CreateAssetGroup", `{"Name":"test"}`, 1)
	require.Equal(t, 200, created.Code, created.Body.String())
	bindings, err := model.FindAssetBindings(1, "group", "group-implicit")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, "platform-project", bindings[0].Project)
	settings.Protocol.ProjectName = "new-default"
	ch.SetSetting(settings)
	require.NoError(t, model.DB.Save(ch).Error)
	got := assetSecurityAction(r, "GetAssetGroup", `{"Id":"group-implicit"}`, 1)
	t.Logf("queried project=%q response=%s", queriedProject, got.Body.String())
	require.Equal(t, "platform-project", queriedProject, "the original upstream project must survive a default change")
	require.Equal(t, 200, got.Code, got.Body.String())
}

func TestAssetLegacyConcurrentClaimsAcrossRoutesMySQL(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")) == "" {
		t.Skip("requires MySQL for cross-channel contention")
	}
	preserveSmartRetryTestConfiguration(t)
	var probes atomic.Int32
	ready := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if probes.Add(1) == 2 {
			close(ready)
		}
		select {
		case <-ready:
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Second):
			w.WriteHeader(504)
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"Id":"same-legacy-asset","ProjectName":"test-project"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"default","alternate":"alternate"}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"alternate":1}`))
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 2).Update("group", "alternate").Error)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.Id, ch.Group = 21, "alternate"
	require.NoError(t, model.DB.Create(ch).Error)
	require.NoError(t, model.DB.Create(&model.Ability{ChannelId: 21, Group: "alternate", Model: tgxRegressionModel, Enabled: true, Priority: common.GetPointer(int64(0))}).Error)
	results := make(chan int, 2)
	for token := 1; token <= 2; token++ {
		go func(token int) { results <- seedanceCall(r, "GET", "/api/assets/same-legacy-asset", "", token).Code }(token)
	}
	first, second := <-results, <-results
	t.Logf("concurrent HTTP statuses=%d,%d", first, second)
	owners := 0
	for token := 1; token <= 2; token++ {
		bindings, err := model.FindAssetBindings(token, "asset", "same-legacy-asset")
		require.NoError(t, err)
		got := seedanceCall(r, "GET", "/api/assets/same-legacy-asset", "", token)
		if len(bindings) > 0 {
			owners++
			require.Equal(t, 200, got.Code)
		} else {
			require.Equal(t, 404, got.Code)
		}
	}
	require.ElementsMatch(t, []int{200, 404}, []int{first, second})
	require.Equal(t, 1, owners)
}

func TestAssetIndependentTgxBackendVideoAlias(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "generic", true: "native"}[native], func(t *testing.T) {
			var submitted []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				submitted, _ = io.ReadAll(r.Body)
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"error":{"message":"stop after capturing request"}}`))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "")
			r.POST("/v1/videos", middleware.TokenAuth(), middleware.Distribute(), RelayTask)
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			setting := ch.GetSetting()
			setting.Protocol.ProfileID = "seedance2-ark-task-assets"
			ch.SetSetting(setting)
			require.NoError(t, model.DB.Save(ch).Error)
			profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
			scope, err := assetAccountScope(ch, profile)
			require.NoError(t, err)
			require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "asset", Project: "test-project", CanonicalID: "asset_local"}, "asset-original"))
			path, body := "/v1/videos", `{"model":"`+tgxRegressionModel+`","duration":4,"metadata":{"content":[{"type":"image_url","image_url":{"url":"asset://asset-original"}}]}}`
			if native {
				path, body = "/api/v3/contents/generations/tasks", `{"model":"`+tgxRegressionModel+`","duration":4,"content":[{"type":"image_url","image_url":{"url":"asset://asset-original"}}]}`
			}
			got := seedanceCall(r, "POST", path, body, 1)
			require.Equal(t, 503, got.Code, got.Body.String())
			t.Logf("outbound request=%s", submitted)
			require.Equal(t, "asset://asset_local", gjson.GetBytes(submitted, "content.0.image_url.url").String())
			require.Equal(t, "test-project", gjson.GetBytes(submitted, "ProjectName").String())
		})
	}
}

func TestAssetResponseAutoGroupDeletionRevokesKnownChild(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("Action") {
		case "CreateAsset":
			_, _ = w.Write([]byte(`{"Result":{"Id":"auto-child","GroupId":"auto-group","ProjectName":"test-project"}}`))
		case "GetAssetGroup":
			_, _ = w.Write([]byte(`{"Result":{"Id":"auto-group","ProjectName":"test-project"}}`))
		case "DeleteAssetGroup":
			_, _ = w.Write([]byte(`{"Result":{}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	created := assetSecurityAction(r, "CreateAsset", `{"URL":"https://example.com/ref.png","AssetType":"Image"}`, 1)
	require.Equal(t, 200, created.Code, created.Body.String())
	before, err := model.FindAssetBindings(1, "asset", "auto-child")
	require.NoError(t, err)
	require.Len(t, before, 1)
	require.Equal(t, "auto-group", before[0].GroupID)
	claimed := assetSecurityAction(r, "GetAssetGroup", `{"Id":"auto-group"}`, 1)
	require.Equal(t, 200, claimed.Code, claimed.Body.String())
	deleted := assetSecurityAction(r, "DeleteAssetGroup", `{"Id":"auto-group"}`, 1)
	require.Equal(t, 200, deleted.Code, deleted.Body.String())
	remaining, err := model.FindAssetBindings(1, "asset", "auto-child")
	require.NoError(t, err)
	require.Empty(t, remaining, "a successful group deletion must revoke known children")
}

func TestAssetResponseHydratesTaskAndGroupAliasesWithoutOverwriting(t *testing.T) {
	conflict := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("Action") == "DeleteAssetGroup" {
			_, _ = w.Write([]byte(`{"Result":{}}`))
			return
		}
		if conflict {
			_, _ = w.Write([]byte(`{"Result":{"Id":"finished","upstream_asset_id":"new-alias","GroupId":"other-group","ProjectName":"test-project"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"Id":"finished","GroupId":"group-original","ProjectName":"test-project"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	group := model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "group", CanonicalID: "group-local", Project: "test-project"}
	require.NoError(t, model.SaveAssetBinding(group, "group-original"))
	require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "task"}, "async-handle"))
	got := seedanceCall(r, "GET", "/api/assets/async-handle", "", 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	for kind, id := range map[string]string{"task": "async-handle", "asset": "finished"} {
		bindings, err := model.FindAssetBindings(1, kind, id)
		require.NoError(t, err)
		require.Len(t, bindings, 1)
		require.Equal(t, "async-handle", bindings[0].CanonicalID)
		require.Equal(t, "test-project", bindings[0].Project)
		require.Equal(t, "group-local", bindings[0].GroupID)
	}
	conflict = true
	got = seedanceCall(r, "GET", "/api/assets/finished", "", 1)
	require.Equal(t, 502, got.Code, got.Body.String())
	exists, err := model.HasAssetBinding("asset", "new-alias")
	require.NoError(t, err)
	require.False(t, exists)
	got = assetSecurityAction(r, "DeleteAssetGroup", `{"Id":"group-original"}`, 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	for kind, id := range map[string]string{"task": "async-handle", "asset": "finished"} {
		bindings, err := model.FindAssetBindings(1, kind, id)
		require.NoError(t, err)
		require.Empty(t, bindings)
	}
}
