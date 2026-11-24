package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoRedirectDropsCredentialsAndPreservesRange(t *testing.T) {
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") != "" || req.Header.Get("x-goog-api-key") != "" ||
			req.Header.Get("Referer") != "" || req.Header.Get("Range") != "bytes=3-7" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusPartialContent)
	}))
	defer cdn.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, cdn.URL+"/file", http.StatusFound)
	}))
	defer origin.Close()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/content", nil)
	c.Request.Header.Set("Range", "bytes=3-7")
	req, err := http.NewRequest("GET", origin.URL+"/file?private=1", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer channel-secret")
	req.Header.Set("x-goog-api-key", "channel-secret")
	resp, err := fetchVideoContent(c, req, origin.Client(), cdn.Client())
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, http.StatusPartialContent, resp.StatusCode)
}

func TestGeminiVideoResultCredentialScope(t *testing.T) {
	const base = "https://provider.example"
	for _, tc := range []struct {
		uri  string
		auth bool
	}{
		{base + "/file", true}, {"https://cdn.example/file?signed=1", false},
		{"https://provider.example:8443/file", false}, {"http://provider.example/file", false},
	} {
		task := &model.Task{}
		var err error
		task.Data, err = common.Marshal(map[string]string{"uri": tc.uri})
		require.NoError(t, err)
		result, err := getGeminiVideoURL(&model.Channel{}, task, "secret")
		require.NoError(t, err)
		require.Equal(t, tc.uri, result, "never append the channel key to a URL")
		require.Equal(t, tc.auth, sameVideoOrigin(base, result))
	}
}

func TestVideoProxyGatewayLookupAndNativeFallback(t *testing.T) {
	r := tgxMaasResourceTestRouter(t, "https://example.test", "test-key", tgxRegressionModel)
	r.GET("/v1/videos/:task_id/content", middleware.TokenAuth(), VideoProxy)
	db := model.DB
	tasks := []model.Task{
		{TaskID: "task_gateway", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-owned", OfficialTaskID: "official-owned", ResultURL: "data:video/mp4;base64,YWJj"}},
		{TaskID: "task_foreign", UserId: 2, ChannelId: 20, Status: model.TaskStatusSuccess, PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-foreign"}},
		{TaskID: "task_ambiguous_a", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-ambiguous"}},
		{TaskID: "task_ambiguous_b", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{OfficialTaskID: "native-ambiguous"}},
		{TaskID: "task_alias_collision", UserId: 1, ChannelId: 20, PrivateData: model.TaskPrivateData{UpstreamTaskID: "task_gateway"}},
	}
	require.NoError(t, db.Create(&tasks).Error)
	var taskQueries []string
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:video_content_lookup", func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			taskQueries = append(taskQueries, tx.Statement.SQL.String())
		}
	}))
	t.Cleanup(func() { db.Callback().Query().Remove("test:video_content_lookup") })
	for _, tc := range []struct {
		id     string
		status int
	}{
		{"task_gateway", http.StatusOK},
		{"native-owned", http.StatusOK},
		{"official-owned", http.StatusOK},
		{"task_foreign", http.StatusNotFound},
		{"native-foreign", http.StatusNotFound},
		{"missing", http.StatusNotFound},
		{"native-ambiguous", http.StatusInternalServerError},
	} {
		t.Run(tc.id, func(t *testing.T) {
			taskQueries = nil
			out := seedanceCall(r, http.MethodGet, "/v1/videos/"+tc.id+"/content", "", 1)
			require.Equal(t, tc.status, out.Code, out.Body.String())
			if tc.status == http.StatusOK {
				require.Equal(t, "abc", out.Body.String())
			}
			if tc.id == "task_gateway" {
				// Avoid reintroducing a JSON scan of the user's entire task history.
				require.Len(t, taskQueries, 1)
				require.NotContains(t, strings.ToLower(taskQueries[0]), "json_extract")
			}
		})
	}
}

func TestVideoProxyEnforcesTokenPermissions(t *testing.T) {
	r := tgxMaasResourceTestRouter(t, "https://example.test", "test-key", tgxRegressionModel)
	r.GET("/v1/videos/:task_id/content", middleware.TokenOrUserAuth(), VideoProxy)
	r.GET("/dashboard/:task_id/content", func(c *gin.Context) {
		c.Set("id", 1) // Context set by dashboard session authentication.
		VideoProxy(c)
	})
	require.NoError(t, model.DB.Create(&model.Task{TaskID: "task_download", UserId: 1, ChannelId: 20, Status: model.TaskStatusSuccess,
		Properties:  model.Properties{OriginModelName: tgxRegressionModel},
		PrivateData: model.TaskPrivateData{UpstreamTaskID: "native-download", ResultURL: "data:video/mp4;base64,YWJj"},
	}).Error)
	for _, tc := range []struct {
		name, limits string
		enabled      bool
		status       int
	}{
		{"forbidden", "unrelated-model", true, http.StatusForbidden},
		{"empty_whitelist", "", true, http.StatusForbidden},
		{"allowed", tgxRegressionModel, true, http.StatusOK},
		{"unrestricted", "", false, http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": tc.enabled, "model_limits": tc.limits}).Error)
			for _, id := range []string{"task_download", "native-download"} {
				out := seedanceCall(r, http.MethodGet, "/v1/videos/"+id+"/content", "", 1)
				require.Equal(t, tc.status, out.Code, out.Body.String())
				if tc.status == http.StatusOK {
					require.Equal(t, "abc", out.Body.String())
				}
			}
		})
	}
	// Missing origin metadata fails closed only for a model-restricted token.
	require.NoError(t, model.DB.Model(&model.Task{}).Where("task_id = ?", "task_download").Update("properties", model.Properties{}).Error)
	require.NoError(t, model.DB.Model(&model.Token{}).Where("id = ?", 1).Updates(map[string]any{"model_limits_enabled": true, "model_limits": tgxRegressionModel}).Error)
	require.Equal(t, http.StatusForbidden, seedanceCall(r, http.MethodGet, "/v1/videos/task_download/content", "", 1).Code)
	for _, channelID := range []string{"20", "21", "invalid"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/content", nil)
		c.Params = gin.Params{{Key: "task_id", Value: "task_download"}}
		c.Set("id", 1)
		common.SetContextKey(c, constant.ContextKeyTokenSpecificChannelId, channelID)
		VideoProxy(c)
		status := http.StatusForbidden
		if channelID == "20" {
			status = http.StatusOK
		}
		require.Equal(t, status, w.Code, w.Body.String())
	}
	dashboard := httptest.NewRecorder()
	r.ServeHTTP(dashboard, httptest.NewRequest(http.MethodGet, "/dashboard/task_download/content", nil))
	require.Equal(t, http.StatusOK, dashboard.Code, dashboard.Body.String())
	require.Equal(t, "abc", dashboard.Body.String())
}
