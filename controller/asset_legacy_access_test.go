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
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetLegacyFirstUseAllBackends(t *testing.T) {
	for _, tc := range []struct{ backend, profile, path, response string }{
		{"volcengine-assets", "", "/api/assets/legacy", `{"Result":{"Id":"legacy"}}`},
		{"youniyouju", "", "/api/assets/legacy", `{"Result":{"Id":"legacy"}}`},
		{"tgxmaas", "seedance-tgxmaas", "/api/assets/legacy", `{"Result":{"Id":"legacy"}}`},
		{"task", "seedance2-ark-task-assets", "/api/assets/legacy", `{"code":0,"data":{"asset_id":"legacy"}}`},
		{"modelsell", "seedance2-modelsell", "/api/assets/legacy", `{"code":0,"data":{"Id":"legacy"}}`},
		{"api-assets", "doubao-seedance-2-api-assets", "/api/assets/legacy", `{"id":"legacy"}`},
		{"material", "doubao-seedance-2", "/api/assets/legacy", `{"id":"legacy"}`},
		{"service-inference", "seedance2-service-inference", "/api/assets/legacy", `{"id":"legacy","task_id":"legacy-task"}`},
		{"max-service-inference", "doubao-seedance-max-service-inference", "/v2/db-sd-max/assets/legacy", `{"success":true,"data":{"Id":"legacy"}}`},
	} {
		for _, inherited := range []bool{false, true} {
			if inherited && tc.profile == "" {
				continue
			}
			t.Run(fmt.Sprintf("%s/inherit=%v", tc.backend, inherited), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					require.NotEmpty(t, r.Header.Get("Authorization"))
					body, _ := io.ReadAll(r.Body)
					require.True(t, strings.Contains(r.URL.Path, "legacy") || strings.Contains(string(body), "legacy"), "lookup must contain the requested ID: %s %s", r.URL, body)
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(tc.response))
				}))
				defer upstream.Close()
				profile := ""
				if inherited {
					profile = tc.profile
				}
				r := assetSecurityRouter(t, upstream.URL, tc.backend, profile)
				got := seedanceCall(r, "GET", tc.path, "", 1)
				require.Equal(t, 200, got.Code, got.Body.String())
				owned, err := model.FindAssetBindings(1, "asset", "legacy")
				require.NoError(t, err)
				require.Len(t, owned, 1)
				require.Equal(t, 20, owned[0].ChannelID)
				require.Equal(t, "test-project", owned[0].Project)
				require.NoError(t, model.DB.Create(&model.Token{Id: 3, UserId: 1, Key: "seedancetest3", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"}).Error)
				before := calls.Load()
				got = seedanceCall(r, "GET", tc.path, "", 3)
				require.Equal(t, 200, got.Code, got.Body.String())
				require.Equal(t, before+1, calls.Load(), "another token of the owner needs no verification")
				got = seedanceCall(r, "GET", tc.path, "", 2)
				require.Equal(t, 404, got.Code, got.Body.String())
				require.Equal(t, before+1, calls.Load(), "other users must be rejected before upstream access")
			})
		}
	}
}

func TestAssetLegacyVerificationFailureDoesNotClaimOrMutate(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"not-found", `{"error":{"code":"NotFound"}}`, 404},
		{"rate-limit", `{"error":{"code":"TooManyRequests"}}`, 429},
		{"provider-error", `{"ResponseMetadata":{"Error":{"Code":"Denied"}}}`, 200},
		{"empty-object", `{}`, 200},
		{"wrong-project", `{"Result":{"Id":"legacy","ProjectName":"other-project"}}`, 200},
		{"foreign-alias", `{"Result":{"Id":"foreign","upstream_asset_id":"legacy"}}`, 200},
		{"revoked-alias", `{"Result":{"Id":"revoked","upstream_asset_id":"legacy"}}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var reads, mutations atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("Action") != "GetAsset" {
					mutations.Add(1)
				}
				reads.Add(1)
				w.Header().Set("X-Request-Id", "lookup-request")
				w.Header().Set("Retry-After", "10")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
			seedAssetOwnershipForTest(t, 20, 2, "asset", "foreign")
			seedAssetOwnershipForTest(t, 20, 1, "asset", "revoked")
			revoked, err := model.FindAssetBindings(1, "asset", "revoked")
			require.NoError(t, err)
			require.NoError(t, model.InvalidateAssetBindings(revoked[0], false))
			got := seedanceCall(r, "PATCH", "/api/assets/legacy", `{"Name":"changed"}`, 1)
			if tc.status != 200 || tc.name == "provider-error" {
				require.Equal(t, tc.status, got.Code, got.Body.String())
				require.JSONEq(t, tc.body, got.Body.String())
				require.Equal(t, "lookup-request", got.Header().Get("X-Request-Id"))
				require.Equal(t, "10", got.Header().Get("Retry-After"))
			} else {
				require.Equal(t, 404, got.Code, got.Body.String())
			}
			require.EqualValues(t, 1, reads.Load())
			require.Zero(t, mutations.Load())
			exists, err := model.HasAssetBinding("asset", "legacy")
			require.NoError(t, err)
			require.False(t, exists, "failed claims must not leave partial aliases")
		})
	}
}

func TestAssetLegacyDeleteVerifiesAndRevokesAliases(t *testing.T) {
	var actions []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("Action")
		actions = append(actions, action)
		if action == "DeleteAsset" {
			w.WriteHeader(204)
			return
		}
		_, _ = w.Write([]byte(`{"Result":{"Id":"canonical","upstream_asset_id":"legacy","GroupId":"old-group"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	got := seedanceCall(r, "DELETE", "/api/assets/legacy", "", 1)
	require.Equal(t, 204, got.Code, got.Body.String())
	require.Equal(t, []string{"GetAsset", "DeleteAsset"}, actions)
	for _, id := range []string{"legacy", "canonical"} {
		for _, user := range []int{1, 2} {
			got = seedanceCall(r, "GET", "/api/assets/"+id, "", user)
			require.Equal(t, 404, got.Code, got.Body.String())
		}
	}
	require.Len(t, actions, 2)
}

func TestAssetLegacyListDoesNotClaimAndGroupClaimIsExplicit(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("Action") {
		case "GetAssetGroup":
			_, _ = w.Write([]byte(`{"Result":{"Id":"legacy-group"}}`))
		case "CreateAsset":
			_, _ = w.Write([]byte(`{"Result":{"Id":"new-asset"}}`))
		default:
			_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"legacy"},{"Id":"new-asset"}],"TotalCount":2}}`))
		}
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	got := seedanceCall(r, "GET", "/api/assets", "", 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	require.NotContains(t, got.Body.String(), "legacy")
	exists, err := model.HasAssetBinding("asset", "legacy")
	require.NoError(t, err)
	require.False(t, exists)
	got = seedanceCall(r, "POST", "/api/assets", `{"GroupId":"legacy-group","URL":"https://example.com/a.png"}`, 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	groups, err := model.FindAssetBindings(1, "group", "legacy-group")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	got = seedanceCall(r, "GET", "/api/assets", `{"Filter":{"GroupIds":["legacy-group"]}}`, 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	require.Contains(t, got.Body.String(), "new-asset")
	require.NotContains(t, got.Body.String(), "legacy\"")
}

func TestAssetLegacyVideoFirstUse(t *testing.T) {
	var reads, videos atomic.Int32
	var submitted []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			reads.Add(1)
			require.Equal(t, "/v1/private-avatar/assets/asset-legacy", r.URL.Path)
			_, _ = w.Write([]byte(`{"Result":{"Id":"canonical","upstream_asset_id":"asset-legacy","ProjectName":"test-project"}}`))
			return
		}
		videos.Add(1)
		submitted, _ = io.ReadAll(r.Body)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"message":"mock stop after submission"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "seedance-tgxmaas")
	payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"image_url","image_url":{"url":"asset://asset-legacy"}}],"duration":4}`
	got := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
	require.Equal(t, 503, got.Code, got.Body.String())
	require.EqualValues(t, 1, reads.Load())
	require.EqualValues(t, 1, videos.Load())
	require.Equal(t, "asset://canonical", gjson.GetBytes(submitted, "content.0.image_url.url").String())
	require.Equal(t, "test-project", gjson.GetBytes(submitted, "ProjectName").String())
	owned, err := model.FindAssetBindings(1, "asset", "asset-legacy")
	require.NoError(t, err)
	require.Len(t, owned, 1)
	got = seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 2)
	require.Equal(t, 404, got.Code, got.Body.String())
	require.EqualValues(t, 1, reads.Load())
	require.EqualValues(t, 1, videos.Load())
}

func TestAssetLegacyTaskHandle(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "/v1/assets/get", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		require.Equal(t, "old-upload", gjson.GetBytes(body, "task_id").String())
		require.Empty(t, gjson.GetBytes(body, "asset_id").String())
		_, _ = w.Write([]byte(`{"id":"finished-asset","task_id":"old-upload"}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "service-inference", "seedance2-service-inference")
	got := seedanceCall(r, "POST", "/v1/assets/get", `{"task_id":"old-upload"}`, 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	for kind, id := range map[string]string{"task": "old-upload", "asset": "finished-asset"} {
		bindings, err := model.FindAssetBindings(1, kind, id)
		require.NoError(t, err)
		require.Len(t, bindings, 1)
		require.Equal(t, "finished-asset", bindings[0].CanonicalID)
	}
	before := calls.Load()
	got = seedanceCall(r, "POST", "/v1/assets/get", `{"task_id":"old-upload"}`, 2)
	require.Equal(t, 404, got.Code, got.Body.String())
	require.Equal(t, before, calls.Load())
}

func TestAssetLegacyVideoUsesKnownChannelRegardlessOfReferenceOrder(t *testing.T) {
	var reads, wrongCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			reads.Add(1)
			_, _ = w.Write([]byte(`{"Result":{"Id":"asset-legacy"}}`))
			return
		}
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"message":"mock stop after submission"}}`))
	}))
	defer upstream.Close()
	wrong := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrongCalls.Add(1)
		w.WriteHeader(404)
	}))
	defer wrong.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "seedance-tgxmaas")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "asset-owned")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	ch.Id, ch.Key, ch.BaseURL, ch.Priority = 21, "other-account", &wrong.URL, common.GetPointer(int64(100))
	require.NoError(t, model.DB.Create(ch).Error)
	createConfigurableResourceAbility(t, model.DB, 21, tgxRegressionModel, true, 100)
	payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"image_url","image_url":{"url":"asset://asset-legacy"}},{"type":"image_url","image_url":{"url":"asset://asset-owned"}}],"duration":4}`
	got := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
	require.Equal(t, 503, got.Code, got.Body.String())
	require.EqualValues(t, 1, reads.Load())
	require.Zero(t, wrongCalls.Load())
	bindings, err := model.FindAssetBindings(1, "asset", "asset-legacy")
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, 20, bindings[0].ChannelID)
}
