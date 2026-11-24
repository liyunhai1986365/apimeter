package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestEmbeddingRoutesUseOpenAIRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db := openChannelRouteAuthTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}, &model.Log{}, &model.RetryRouteEvent{}))
	oldCache := common.MemoryCacheEnabled
	oldRatios := ratio_setting.ModelRatio2JSONString()
	common.MemoryCacheEnabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldCache
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
	})
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"text-embedding-ada-002":0.05,"embedding-alias":0.05,"azure-alias":0.05}`))
	service.InitHttpClient()
	service.InitTokenEncoders()
	const modelName = "text-embedding-ada-002"
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "embedding-user", Group: "default",
		Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000000}).Error)
	require.NoError(t, db.Create(&model.Token{UserId: 1, Key: "embeddingtoken", Group: "default",
		Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}).Error)
	type upstreamRequest struct {
		Path string
		Body map[string]any
		Err  error
	}
	requests := make(chan upstreamRequest, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		err := common.DecodeJson(r.Body, &body)
		requests <- upstreamRequest{r.URL.RequestURI(), body, err}
		if strings.Contains(r.URL.Path, "/engines/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],"model":"text-embedding-ada-002","usage":{"prompt_tokens":10,"total_tokens":10}}`))
	}))
	defer upstream.Close()
	mapping := `{"embedding-alias":"text-embedding-ada-002","azure-alias":"embedding-deployment"}`
	for _, channel := range []model.Channel{
		{Id: 1, Type: constant.ChannelTypeOpenAI, Models: modelName + ",embedding-alias"},
		{Id: 2, Type: constant.ChannelTypeAzure, Models: "azure-alias", Other: "2024-02-01"},
	} {
		channel.Key, channel.Name, channel.Group = "mock", "embedding", "default"
		channel.BaseURL, channel.Status, channel.ModelMapping = &upstream.URL, common.ChannelStatusEnabled, &mapping
		require.NoError(t, db.Create(&channel).Error)
		for _, name := range strings.Split(channel.Models, ",") {
			require.NoError(t, db.Create(&model.Ability{ChannelId: channel.Id, Model: name, Group: "default", Enabled: true}).Error)
		}
	}
	r := gin.New()
	SetRelayRouter(r)
	for _, tc := range []struct{ name, path, body, wantUpstream, wantModel string }{
		{"modern", "/v1/embeddings", `{"model":"text-embedding-ada-002","input":["one"],"dimensions":3}`, "/v1/embeddings", modelName},
		{"legacy body model", "/v1/engines/text-embedding-ada-002/embeddings", `{"model":"text-embedding-ada-002","input":["one"],"dimensions":3}`, "/v1/embeddings", modelName},
		{"legacy path model", "/v1/engines/text-embedding-ada-002/embeddings", `{"input":["one"],"dimensions":3}`, "/v1/embeddings", modelName},
		{"legacy mapped alias with query", "/v1/engines/embedding-alias/embeddings?check=1", `{"input":["one"],"dimensions":3}`, "/v1/embeddings?check=1", modelName},
		{"legacy Azure deployment", "/v1/engines/azure-alias/embeddings", `{"input":["one"],"dimensions":3}`, "/openai/deployments/embedding-deployment/embeddings?api-version=2024-02-01", "embedding-deployment"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Authorization", "Bearer embeddingtoken")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			select {
			case upstream := <-requests:
				require.NoError(t, upstream.Err)
				require.Equal(t, tc.wantUpstream, upstream.Path)
				require.Equal(t, tc.wantModel, upstream.Body["model"])
				require.Equal(t, []any{"one"}, upstream.Body["input"])
				require.Equal(t, float64(3), upstream.Body["dimensions"])
			default:
				t.Fatal("request did not reach embedding upstream")
			}
			require.Contains(t, w.Body.String(), `"embedding":[0.1,0.2,0.3]`)
		})
	}
}
