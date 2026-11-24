package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
)

func TestUpdateSelfCompletesRequiredPasswordChange(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, i18n.Init())
	previousRDB, previousSecret := common.RDB, common.SessionSecret
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	common.RDB, common.RedisEnabled = client, true
	common.SessionSecret = "required-password-change-test-secret"
	t.Cleanup(func() {
		_ = client.Close()
		common.RDB, common.SessionSecret = previousRDB, previousSecret
	})
	hash, err := common.Password2Hash("initial-password")
	require.NoError(t, err)
	user := model.User{Username: "password-child", Password: hash, Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1,
		ParentUserId: 42, MustChangePassword: true, Quota: 100}
	require.NoError(t, db.Create(&user).Error)
	bundle, err := service.CreateLoginSession(user.Id, "password", "127.0.0.1", "test")
	require.NoError(t, err)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.PUT("/api/user/self", middleware.UserAuth(), UpdateSelf)
	r.GET("/api/workspaces", middleware.UserAuth(), func(c *gin.Context) { c.Status(http.StatusOK) })
	call := func(method, path, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	require.Equal(t, http.StatusForbidden, call("GET", "/api/workspaces", bundle.AccessToken, "").Code)
	w := call("PUT", "/api/user/self", bundle.AccessToken,
		`{"username":"password-child","original_password":"wrong-password","password":"new-password"}`)
	require.Contains(t, w.Body.String(), `"success":false`)
	stored, err := model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.True(t, stored.MustChangePassword)
	require.EqualValues(t, 1, stored.AuthVersion)
	w = call("PUT", "/api/user/self", bundle.AccessToken,
		`{"username":"password-child","original_password":"initial-password","password":"new-password"}`)
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &result))
	require.True(t, result.Success, w.Body.String())
	require.NotEmpty(t, result.Data.AccessToken)
	stored, err = model.GetUserById(user.Id, true)
	require.NoError(t, err)
	require.False(t, stored.MustChangePassword)
	require.True(t, common.ValidatePasswordAndHash("new-password", stored.Password))
	require.EqualValues(t, 2, stored.AuthVersion)
	cached, err := model.GetUserCache(user.Id)
	require.NoError(t, err)
	require.False(t, cached.MustChangePassword)
	require.Equal(t, user.ParentUserId, cached.ParentUserId)
	require.Equal(t, http.StatusOK, call("GET", "/api/workspaces", result.Data.AccessToken, "").Code)
	require.Equal(t, http.StatusUnauthorized, call("GET", "/api/workspaces", bundle.AccessToken, "").Code)
}
