package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/configurable"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAssetReviewInheritedGroupCacheCredentialRotation(t *testing.T) {
	var createdGroups, validatedGroups int
	var suppliedGroups []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/v1/asset-groups":
			createdGroups++
			_, _ = fmt.Fprintf(w, `{"id":"group-%d"}`, createdGroups)
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/v1/asset-groups/"):
			validatedGroups++
			// The same library remains available through the replacement key.
			_, _ = w.Write([]byte(`{"id":"group-1"}`))
		case r.Method == "POST" && r.URL.Path == "/v1/assets":
			body, _ := io.ReadAll(r.Body)
			suppliedGroups = append(suppliedGroups, gjson.GetBytes(body, "group_id").String())
			_, _ = fmt.Fprintf(w, `{"id":"asset-%d"}`, len(suppliedGroups))
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "service-inference", "seedance2-service-inference")
	for i := 0; i < 3; i++ {
		if i == 2 {
			require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 20).Update("key", "rotated-account").Error)
		}
		got := seedanceCall(r, "POST", "/api/assets/upload", `{"url":"https://example.com/a.png","asset_type":"Image"}`, 1)
		require.Equal(t, 200, got.Code, got.Body.String())
	}
	require.Equal(t, []string{"group-1", "group-1", "group-1"}, suppliedGroups)
	require.Equal(t, 1, createdGroups)
	require.Equal(t, 2, validatedGroups, "credential rotation must keep the existing library's cached group")
}

func TestAssetReviewVideoUsesOriginalProject(t *testing.T) {
	var submitted []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		submitted, _ = io.ReadAll(r.Body)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"message":"mock stop after submission"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "seedance-tgxmaas")
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "asset", Project: "original-project", CanonicalID: "asset_local"}, "asset-original"))
	payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"image_url","image_url":{"url":"asset://asset-original"}}],"duration":4}`
	got := seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
	require.Equal(t, 503, got.Code, got.Body.String())
	require.Equal(t, "original-project", gjson.GetBytes(submitted, "ProjectName").String())
	require.Equal(t, "asset://asset_local", gjson.GetBytes(submitted, "content.0.image_url.url").String())
	got = seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", strings.TrimSuffix(payload, "}")+`,"ProjectName":"different-project"}`, 1)
	require.Equal(t, 404, got.Code, got.Body.String())
}

func TestAssetReviewVideoRetainsProtocolFilter(t *testing.T) {
	assetSecurityRouter(t, "https://unused.example", "tgxmaas", "seedance-tgxmaas")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v1/services/aigc/video-generation/video-synthesis", strings.NewReader(`{"image":"asset://owned"}`))
	common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
	c.Set(middleware.ContextKeyConfigurableNativeProfileID, "dashscope-wan3-video")
	c.Set(middleware.ContextKeyConfigurableNativeProfileIDs, []string{"dashscope-wan3-video"})
	info := &relaycommon.RelayInfo{UserId: 1, OriginModelName: tgxRegressionModel, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	require.ErrorIs(t, lockAssetVideoChannel(c, info), model.ErrAssetNotOwned, "asset binding cannot override native endpoint compatibility")
}

func TestAssetReviewCursorContinuesAfterEmptyPage(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"Result":{"Items":[],"NextToken":"continue"}}`))
		} else {
			_, _ = w.Write([]byte(`{"Result":{"Items":[{"Id":"owned"}],"NextToken":""}}`))
		}
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "youniyouju", "")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "owned")
	got := assetSecurityAction(r, "ListAssets", `{"MaxResults":10}`, 1)
	require.Equal(t, 200, got.Code, got.Body.String())
	require.Contains(t, got.Body.String(), "owned")
	require.Equal(t, 2, calls)
}

func TestAssetReviewOrdinaryVideoTextDoesNotBindChannel(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/videos", strings.NewReader(`{"prompt":"asset://literal","metadata":{"negative_prompt":"asset://literal","description":"asset://literal","title":"asset://literal"}}`))
	info := &relaycommon.RelayInfo{UserId: 1, TaskRelayInfo: &relaycommon.TaskRelayInfo{}}
	require.NoError(t, lockAssetVideoChannel(c, info))
	require.Nil(t, info.LockedChannel)
}

func TestAssetReviewCurrentAccountDoesNotReuseLegacyAlias(t *testing.T) {
	var path string
	var body []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"message":"stop after inspecting request"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "seedance-tgxmaas")
	seedAssetOwnershipForTest(t, 20, 1, "asset", "asset-current")
	// This mapping was written for a previous upstream account; inherited
	// legacy mappings had no key/address fingerprint.
	require.NoError(t, model.SaveTgxMaasAssetHandle(20, 1, "test-project", "asset-current", "asset_foreign"))
	got := seedanceCall(r, "GET", "/api/assets/asset-current", "", 1)
	require.Equal(t, 503, got.Code, got.Body.String())
	require.Equal(t, "/v1/private-avatar/assets/asset-current", path)
	payload := `{"model":"` + tgxRegressionModel + `","content":[{"type":"image_url","image_url":{"url":"asset://asset-current"}}],"duration":4}`
	got = seedanceCall(r, "POST", "/api/v3/contents/generations/tasks", payload, 1)
	require.Equal(t, 503, got.Code, got.Body.String())
	require.Equal(t, "asset://asset-current", gjson.GetBytes(body, "content.0.image_url.url").String())
}

func TestAssetReviewProjectNormalization(t *testing.T) {
	for _, backend := range []string{"tgxmaas", "youniyouju"} {
		t.Run(backend, func(t *testing.T) {
			var sentProject string
			var calls int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				sentProject = gjson.GetBytes(body, "ProjectName").String()
				if r.Method == "GET" {
					sentProject = r.URL.Query().Get("ProjectName")
				}
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"error":{"message":"stop after inspecting project"}}`))
			}))
			defer upstream.Close()
			r := assetSecurityRouter(t, upstream.URL, backend, "")
			ch, err := model.GetChannelById(20, true)
			require.NoError(t, err)
			profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
			scope, err := assetAccountScope(ch, profile)
			require.NoError(t, err)
			require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "asset", Project: "original-project"}, "owned"))
			for _, tc := range []struct {
				name, query, body string
				status            int
			}{
				{"omitted", "", `{}`, 503},
				{"empty", "", `{"ProjectName":""}`, 503},
				{"alias", "", `{"projectName":"original-project"}`, 503},
				{"foreign alias", "", `{"projectName":"other-project"}`, 404},
				{"query alias", "?projectName=original-project", `{}`, 503},
				{"foreign query", "?projectName=other-project", `{}`, 404},
				{"conflict", "?project_name=other-project", `{"ProjectName":"original-project"}`, 400},
				{"invalid type", "", `{"projectName":123}`, 400},
			} {
				t.Run(tc.name, func(t *testing.T) {
					before := calls
					got := seedanceCall(r, "GET", "/api/assets/owned"+tc.query, tc.body, 1)
					require.Equal(t, tc.status, got.Code, got.Body.String())
					if tc.status == 503 {
						require.Equal(t, before+1, calls)
						require.Equal(t, "original-project", sentProject)
					} else {
						require.Equal(t, before, calls, "invalid projects must not reach upstream")
					}
				})
			}
		})
	}
}

func TestAssetReviewVideoProjectNormalization(t *testing.T) {
	var sent []byte
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		sent, _ = io.ReadAll(r.Body)
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":{"message":"stop after inspecting project"}}`))
	}))
	defer upstream.Close()
	r := assetSecurityRouter(t, upstream.URL, "tgxmaas", "seedance-tgxmaas")
	r.POST("/v1/videos", middleware.TokenAuth(), middleware.Distribute(), RelayTask)
	ch, err := model.GetChannelById(20, true)
	require.NoError(t, err)
	profile, _ := configurable.AssetProfile(ch.GetSetting().Protocol)
	scope, err := assetAccountScope(ch, profile)
	require.NoError(t, err)
	require.NoError(t, model.SaveAssetBinding(model.AssetBinding{ChannelID: 20, UserID: 1, Scope: scope, Kind: "asset", Project: "original-project"}, "asset-owned"))
	for _, mode := range []string{"native", "generic"} {
		for _, tc := range []struct {
			name, field string
			status      int
		}{
			{"empty", `"ProjectName":""`, 503},
			{"alias", `"project_name":"original-project"`, 503},
			{"case alias", `"projectName":"original-project"`, 503},
			{"foreign", `"ProjectName":"other-project"`, 404},
			{"foreign alias", `"projectName":"other-project"`, 404},
			{"duplicate", `"ProjectName":"original-project","project_name":"other-project"`, 400},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				before := calls
				content := `"content":[{"type":"image_url","image_url":{"url":"asset://asset-owned"}}],` + tc.field
				path := "/api/v3/contents/generations/tasks"
				if mode == "generic" {
					path = "/v1/videos"
					content = `"metadata":{` + content + `}`
				}
				got := seedanceCall(r, "POST", path, `{"model":"`+tgxRegressionModel+`","duration":4,`+content+`}`, 1)
				require.Equal(t, tc.status, got.Code, got.Body.String())
				if tc.status == 503 {
					require.Equal(t, before+1, calls)
					require.Equal(t, "original-project", gjson.GetBytes(sent, "ProjectName").String())
					require.False(t, gjson.GetBytes(sent, "projectName").Exists())
					require.False(t, gjson.GetBytes(sent, "project_name").Exists())
				} else {
					require.Equal(t, before, calls)
				}
			})
		}
	}
}
