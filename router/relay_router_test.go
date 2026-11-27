package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestListModelsSupportsOpenAIAndGeminiAuthentication(t *testing.T) {
	setupRelayRouterTestDB(t)

	user := model.User{
		Username: "models-user",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    100,
	}
	require.NoError(t, model.DB.Create(&user).Error)
	require.NoError(t, model.DB.Create(&model.Token{
		UserId:         user.Id,
		Key:            "modelstestkey",
		Status:         common.TokenStatusEnabled,
		ExpiredTime:    -1,
		UnlimitedQuota: true,
	}).Error)

	engine := gin.New()
	SetRelayRouter(engine)

	tests := []struct {
		name           string
		path           string
		headerName     string
		expectedObject string
		expectedField  string
	}{
		{
			name:           "OpenAI bearer token",
			path:           "/v1/models",
			headerName:     "Authorization",
			expectedObject: "list",
			expectedField:  "data",
		},
		{
			name:          "Gemini API key header",
			path:          "/v1/models",
			headerName:    "x-goog-api-key",
			expectedField: "models",
		},
		{
			name:          "Gemini API key query",
			path:          "/v1/models?key=modelstestkey",
			expectedField: "models",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			if test.headerName != "" {
				value := "modelstestkey"
				if test.headerName == "Authorization" {
					value = "Bearer " + value
				}
				request.Header.Set(test.headerName, value)
			}

			engine.ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			var payload map[string]any
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
			assert.Contains(t, payload, test.expectedField)
			assert.NotContains(t, payload, "error")
			if test.expectedObject != "" {
				assert.Equal(t, test.expectedObject, payload["object"])
			}
		})
	}
}

func TestOrderedTokenListsAndRoutesModels(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, i18n.Init())
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}, &model.Model{}, &model.Vendor{}, &model.Log{}, &model.BillingUsageItem{}))
	const geminiModel = "gemini-3.8-flash"
	groups := []string{"ch_GPT特价", "ch_OpenAI-5.5及以下", "ch_Gemini_CLI", "ch_asgemini-hou", "ch_Claude-h55*f52-hou", "ch_Claude_kiro"}
	channels := []struct {
		kind   int
		models []string
	}{
		{constant.ChannelTypeOpenAI, []string{"gpt-4o-mini"}},
		{constant.ChannelTypeOpenAI, []string{"gpt-4o", "gpt-4o-mini"}},
		{constant.ChannelTypeGemini, []string{geminiModel}},
		{constant.ChannelTypeGemini, []string{"gemini-2.5-pro", geminiModel}},
		{constant.ChannelTypeAnthropic, []string{"claude-sonnet-4-5"}},
		{constant.ChannelTypeAnthropic, []string{"claude-haiku-4-5", "claude-sonnet-4-5"}},
	}
	usable, ratios := map[string]string{}, map[string]float64{}
	for _, group := range groups {
		usable[group], ratios[group] = group, 1
	}
	oldUsable, oldRatios, oldPrices := setting.UserUsableGroups2JSONString(), ratio_setting.GroupRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	oldCache := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = oldCache
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUsable))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
	})
	usableJSON, err := common.Marshal(usable)
	require.NoError(t, err)
	ratioJSON, err := common.Marshal(ratios)
	require.NoError(t, err)
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(usableJSON)))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(string(ratioJSON)))
	prices := map[string]float64{}
	for _, ch := range channels {
		for _, name := range ch.models {
			prices[name] = 0
		}
	}
	pricesJSON, err := common.Marshal(prices)
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(string(pricesJSON)))

	user := model.User{Username: "ordered-user", Group: "default", Status: common.UserStatusEnabled, Quota: 100000}
	require.NoError(t, model.DB.Create(&user).Error)
	policy, err := common.Marshal(service.TokenGroupPolicy{Type: service.TokenGroupPolicyTypeOrdered, Groups: groups})
	require.NoError(t, err)
	token := model.Token{UserId: user.Id, Key: "orderedtestkey", Group: groups[0], GroupPolicy: string(policy), UnlimitedQuota: true, ExpiredTime: -1, CrossGroupRetry: true}
	require.NoError(t, model.DB.Create(&token).Error)

	service.InitHttpClient()
	for i, ch := range channels {
		marker := fmt.Sprintf("group-%d-reply", i+1)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			switch ch.kind {
			case constant.ChannelTypeOpenAI:
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				fmt.Fprintf(w, `{"object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`, marker)
			case constant.ChannelTypeGemini:
				assert.Contains(t, r.URL.Path, ":generateContent")
				fmt.Fprintf(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":%q}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`, marker)
			case constant.ChannelTypeAnthropic:
				assert.Equal(t, "/v1/messages", r.URL.Path)
				fmt.Fprintf(w, `{"type":"message","role":"assistant","content":[{"type":"text","text":%q}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`, marker)
			}
		}))
		t.Cleanup(upstream.Close)
		channel := model.Channel{Type: ch.kind, Name: marker, Key: "test-upstream", BaseURL: &upstream.URL,
			Group: groups[i], Models: strings.Join(ch.models, ","), Status: common.ChannelStatusEnabled, Priority: common.GetPointer(int64(i))}
		require.NoError(t, channel.Insert())
	}

	engine := gin.New()
	SetRelayRouter(engine)
	request := func(kind int, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		switch kind {
		case constant.ChannelTypeGemini:
			req.Header.Set("x-goog-api-key", token.Key)
		case constant.ChannelTypeAnthropic:
			req.Header.Set("x-api-key", token.Key)
			req.Header.Set("anthropic-version", "2023-06-01")
		default:
			req.Header.Set("Authorization", "Bearer "+token.Key)
		}
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	generate := func(kind int, name string) *httptest.ResponseRecorder {
		switch kind {
		case constant.ChannelTypeGemini:
			return request(kind, http.MethodPost, "/v1beta/models/"+name+":generateContent", `{"contents":[{"parts":[{"text":"hello"}]}]}`)
		case constant.ChannelTypeAnthropic:
			return request(kind, http.MethodPost, "/v1/messages", fmt.Sprintf(`{"model":%q,"max_tokens":8,"messages":[{"role":"user","content":"hello"}]}`, name))
		default:
			return request(kind, http.MethodPost, "/v1/chat/completions", fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, name))
		}
	}
	listModels := func(t *testing.T, kind int) []string {
		t.Helper()
		path := "/v1/models"
		if kind == constant.ChannelTypeGemini {
			path = "/v1beta/models"
		}
		list := request(kind, http.MethodGet, path, "")
		require.Equal(t, http.StatusOK, list.Code, list.Body.String())
		var payload struct {
			Data   []struct{ ID string }
			Models []struct{ Name string }
		}
		require.NoError(t, common.Unmarshal(list.Body.Bytes(), &payload))
		var names []string
		for _, item := range payload.Data {
			names = append(names, item.ID)
		}
		for _, item := range payload.Models {
			names = append(names, item.Name)
		}
		return names
	}
	var wantNames []string
	for _, ch := range channels {
		wantNames = append(wantNames, ch.models[0])
	}
	var tokenQueries atomic.Int64
	require.NoError(t, model.DB.Callback().Query().Before("gorm:query").Register("test:count_token_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "tokens" {
			tokenQueries.Add(1)
		}
	}))
	delete(usable, groups[2])
	delete(usable, groups[5])
	restrictedJSON, err := common.Marshal(usable)
	require.NoError(t, err)
	for _, cache := range []struct {
		memory bool
		legacy string
	}{
		{false, ""}, {true, ""}, {false, "missing"}, {true, "empty"}, {true, "stale-order"},
	} {
		t.Run(fmt.Sprintf("channel-cache=%t/legacy=%s", cache.memory, cache.legacy), func(t *testing.T) {
			common.MemoryCacheEnabled = cache.memory
			model.InitChannelCache()
			if cache.legacy != "" {
				previous := common.RDB
				var options redis.Options
				if socket := os.Getenv("APIMETER_TEST_REDIS_SOCKET"); socket != "" {
					options.Network, options.Addr = "unix", socket
				} else {
					options.Addr = miniredis.RunT(t).Addr()
				}
				client := redis.NewClient(&options)
				common.RDB, common.RedisEnabled = client, true
				t.Cleanup(func() {
					common.RDB, common.RedisEnabled = previous, false
					require.NoError(t, client.Close())
				})
				_, err := model.GetTokenByKey(token.Key, false)
				require.NoError(t, err)
				hash := "token:" + common.GenerateHMAC(token.Key)
				require.NoError(t, client.HDel(t.Context(), hash, "RoutingVersion").Err())
				switch cache.legacy {
				case "missing":
					require.NoError(t, client.HDel(t.Context(), hash, "GroupPolicy").Err())
				case "empty":
					require.NoError(t, client.HSet(t.Context(), hash, "GroupPolicy", "").Err())
				case "stale-order":
					oldPolicy, err := common.Marshal(service.TokenGroupPolicy{Type: service.TokenGroupPolicyTypeOrdered, Groups: []string{groups[3], groups[2]}})
					require.NoError(t, err)
					require.NoError(t, client.HSet(t.Context(), hash, "Group", groups[3], "GroupPolicy", string(oldPolicy)).Err())
				}
				// Simulate Redis balances newer than the database, retained across the update.
				require.NoError(t, client.HSet(t.Context(), hash, "RemainQuota", 83, "UsedQuota", 17).Err())
				before := tokenQueries.Load()
				reply := generate(constant.ChannelTypeGemini, geminiModel)
				require.Equal(t, http.StatusOK, reply.Code, reply.Body.String())
				require.Contains(t, reply.Body.String(), "group-3-reply")
				require.Greater(t, tokenQueries.Load(), before)
				fields, err := client.HGetAll(t.Context(), hash).Result()
				require.NoError(t, err)
				require.Equal(t, token.GroupPolicy, fields["GroupPolicy"])
				require.Equal(t, groups[0], fields["Group"])
				require.NotEmpty(t, fields["RoutingVersion"])
				require.Equal(t, "83", fields["RemainQuota"])
				require.Equal(t, "17", fields["UsedQuota"])
				before = tokenQueries.Load()
				_, err = model.GetTokenByKey(token.Key, false)
				require.NoError(t, err)
				require.Equal(t, before, tokenQueries.Load(), "upgraded token should be served by Redis")
			}
			for _, kind := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeGemini, constant.ChannelTypeAnthropic} {
				require.ElementsMatch(t, wantNames, listModels(t, kind))
			}
			for i, ch := range channels {
				t.Run(ch.models[0], func(t *testing.T) {
					reply := generate(ch.kind, ch.models[0])
					require.Equal(t, http.StatusOK, reply.Code, reply.Body.String())
					require.Contains(t, reply.Body.String(), fmt.Sprintf("group-%d-reply", i+1))
				})
			}
			missing := generate(constant.ChannelTypeGemini, "unconfigured")
			require.Equal(t, http.StatusServiceUnavailable, missing.Code, missing.Body.String())
			require.Contains(t, missing.Body.String(), strings.Join(groups, " → "))
			require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(restrictedJSON)))
			t.Cleanup(func() { require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(string(usableJSON))) })
			names := listModels(t, constant.ChannelTypeGemini)
			require.Contains(t, names, geminiModel)
			require.NotContains(t, names, channels[5].models[0])
			reply := generate(constant.ChannelTypeGemini, geminiModel)
			require.Equal(t, http.StatusOK, reply.Code, reply.Body.String())
			require.Contains(t, reply.Body.String(), "group-4-reply")
			denied := generate(constant.ChannelTypeAnthropic, channels[5].models[0])
			require.Equal(t, http.StatusServiceUnavailable, denied.Code, denied.Body.String())
			require.NotContains(t, denied.Body.String(), groups[5])
		})
	}
}

func setupRelayRouterTestDB(t *testing.T) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	originalIsMasterNode := common.IsMasterNode
	originalRedisEnabled := common.RedisEnabled
	originalSQLitePath := common.SQLitePath
	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalSQLDSN, hadSQLDSN := os.LookupEnv("SQL_DSN")

	common.IsMasterNode = false
	common.RedisEnabled = false
	common.SQLitePath = fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	require.NoError(t, os.Setenv("SQL_DSN", "local"))
	require.NoError(t, model.InitDB())
	model.LOG_DB = model.DB
	require.NoError(t, model.DB.AutoMigrate(&model.User{}, &model.Token{}, &model.Ability{}))

	t.Cleanup(func() {
		if sqlDB, err := model.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
		common.IsMasterNode = originalIsMasterNode
		common.RedisEnabled = originalRedisEnabled
		common.SQLitePath = originalSQLitePath
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
		if hadSQLDSN {
			require.NoError(t, os.Setenv("SQL_DSN", originalSQLDSN))
		} else {
			require.NoError(t, os.Unsetenv("SQL_DSN"))
		}
	})
}
