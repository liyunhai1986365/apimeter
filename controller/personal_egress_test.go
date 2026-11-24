package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPersonalProviderCRUDLeavesPlatformCacheAndPoolIntact(t *testing.T) {
	prev := common.MemoryCacheEnabled
	t.Cleanup(func() {
		common.MemoryCacheEnabled = prev
		if prev && model.DB != nil {
			model.InitChannelCache()
		}
	})
	setupChannelControllerBatchTestDB(t)
	common.MemoryCacheEnabled = true
	platform := model.Channel{Group: "platform-test", Models: "platform-model"}
	require.NoError(t, platform.Insert())
	model.InitChannelCache()
	cached, err := model.CacheGetChannel(platform.Id)
	require.NoError(t, err)
	const proxy = "http://fixture.invalid:8080"
	t.Cleanup(func() { service.InvalidateProxyClient(proxy) })
	client, err := service.GetHttpClientWithProxy(proxy)
	require.NoError(t, err)
	id := 0
	call := func(handler gin.HandlerFunc, method, body string) {
		t.Helper()
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("id", 77)
		c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(id)}}
		c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
		handler(c)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"success":true`)
		again, err := model.CacheGetChannel(platform.Id)
		require.NoError(t, err)
		require.Same(t, cached, again)
		pool, err := service.GetHttpClientWithProxy(proxy)
		require.NoError(t, err)
		require.Same(t, client, pool)
	}
	call(CreateUserOwnedProvider, http.MethodPost, `{"channel":{"type":1,"key":"fixture","name":"local","group":"platform-test","models":"before","base_url":"https://provider.example"}}`)
	var row model.Channel
	require.NoError(t, model.DB.First(&row, "owner_user_id = ?", 77).Error)
	id = row.Id
	require.Equal(t, model.BuildUserOwnedProviderGroup(77, id), row.Group)
	require.True(t, model.IsChannelEnabledForGroupModel(row.Group, "before", id))
	call(UpdateUserOwnedProvider, http.MethodPut, `{"channel":{"models":"after","group":"platform-test"}}`)
	require.False(t, model.IsChannelEnabledForGroupModel(row.Group, "before", id))
	require.True(t, model.IsChannelEnabledForGroupModel(row.Group, "after", id))
	current, err := model.CacheGetChannel(id)
	require.NoError(t, err)
	require.Equal(t, "fixture", current.Key)
	require.Equal(t, "after", current.Models)
	call(DeleteUserOwnedProvider, http.MethodDelete, "")
	require.False(t, model.IsChannelEnabledForGroupModel(row.Group, "after", id))
	_, err = model.CacheGetChannel(id)
	require.Error(t, err)
	var n int64
	require.NoError(t, model.DB.Model(&model.Ability{}).Where("channel_id = ?", id).Count(&n).Error)
	require.Zero(t, n)
}

func TestPersonalProviderWriteBoundary(t *testing.T) {
	setupChannelControllerBatchTestDB(t)
	body := `{"channel":{"type":1,"key":"fixture","name":"personal","models":"example","base_url":"https://127.0.0.1"}}`
	call := func(handler gin.HandlerFunc, method string) string {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Set("id", 77)
		c.Params = gin.Params{{Key: "id", Value: "42"}}
		c.Request = httptest.NewRequest(method, "/", strings.NewReader(body))
		handler(c)
		require.Equal(t, http.StatusOK, rec.Code)
		return rec.Body.String()
	}
	require.Contains(t, call(CreateUserOwnedProvider, http.MethodPost), `"success":false`)
	var count int64
	require.NoError(t, model.DB.Model(&model.Channel{}).Count(&count).Error)
	require.Zero(t, count)
	row := &model.Channel{Id: 42, Type: 1, Key: "fixture", Models: "example", Group: model.BuildUserOwnedProviderGroup(77, 42), Scope: model.ChannelScopeUserOwned, OwnerUserId: 77, BaseURL: common.GetPointer("https://provider.example")}
	require.NoError(t, row.Insert())
	require.Contains(t, call(UpdateUserOwnedProvider, http.MethodPut), `"success":false`)
	stored, err := model.GetChannelById(42, true)
	require.NoError(t, err)
	require.Equal(t, "https://provider.example", stored.GetBaseURL())
}

func TestModelUpdateBatchesExcludePersonalChannels(t *testing.T) {
	setupChannelControllerBatchTestDB(t)
	for i, scope := range []string{model.ChannelScopePlatform, model.ChannelScopeUserOwned, model.ChannelScopePlatform} {
		row := &model.Channel{Id: i + 1, Scope: scope, Status: common.ChannelStatusEnabled}
		require.NoError(t, model.DB.Create(row).Error)
	}
	require.NoError(t, model.DB.Model(&model.Channel{}).Where("id = ?", 1).Update("scope", "").Error)
	first, err := findEnabledChannelsAfterID(0, 1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, 1, first[0].Id)
	second, err := findEnabledChannelsAfterID(first[0].Id, 2)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, 3, second[0].Id)
	require.Equal(t, model.ChannelScopePlatform, second[0].Scope)
}

func TestHistoricalPersonalChannelSelectionAndDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name, scope, base, path string
		allowed                 bool
	}{
		{"private personal", model.ChannelScopeUserOwned, "https://127.0.0.1", "/v1/chat/completions", false},
		{"unsupported transport", model.ChannelScopeUserOwned, "https://provider.example", "/v1/realtime", false},
		{"public personal", model.ChannelScopeUserOwned, "https://provider.example", "/v1/chat/completions", true},
		{"platform private", model.ChannelScopePlatform, "http://127.0.0.1", "/v1/chat/completions", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ch := &model.Channel{Id: 42, Type: constant.ChannelTypeOpenAI, Key: "fixture", Scope: tc.scope, BaseURL: &tc.base}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, tc.path, nil)
			err := middleware.SetupContextForSelectedChannel(c, ch, "example")
			if tc.allowed {
				require.Nil(t, err)
				settings, ok := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
				require.True(t, ok)
				require.Equal(t, tc.scope == model.ChannelScopeUserOwned, settings.UserOwned)
			} else {
				require.NotNil(t, err)
				_, selected := c.Get("relay_selected_channel")
				require.False(t, selected)
			}
			if tc.scope == model.ChannelScopeUserOwned {
				require.ErrorIs(t, testChannel(ch, "example", "", false).localErr, model.ErrPlatformChannelRequired)
			}
		})
	}
}
