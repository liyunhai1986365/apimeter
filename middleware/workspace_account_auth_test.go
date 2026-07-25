package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// The routes below are registered with the same patterns as router/api-router.go, because the
// workspace-account policy matches gin route patterns verbatim.
const (
	workspaceAccountTokenRoute      = "/api/token/"
	workspaceAccountSelfRoute       = "/api/user/self"
	workspaceAccountLogoutRoute     = "/api/user/logout"
	workspaceAccountForbiddenRoute  = "/api/user/topup"
	workspaceAccountUnlistedGetPath = "/api/user/aff"
)

func setupWorkspaceAccountAuthTest(t *testing.T, mustChangePassword bool) (*gin.Engine, *model.User, *model.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	originalDB := model.DB
	originalRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:workspace-account-auth-%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Workspace{}, &model.WorkspaceMember{}))
	model.DB = db
	owner := &model.User{
		Username: "workspace-owner", Password: "hashed", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", AffCode: "wown",
	}
	require.NoError(t, db.Create(owner).Error)
	child := &model.User{
		Username: "workspace-child", Password: "hashed", Role: common.RoleCommonUser,
		Status: common.UserStatusEnabled, Group: "default", ParentUserId: owner.Id,
		MustChangePassword: mustChangePassword, AffCode: "wact",
	}
	require.NoError(t, db.Create(child).Error)
	t.Cleanup(func() {
		model.DB = originalDB
		common.RedisEnabled = originalRedisEnabled
	})

	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("workspace-account-auth-test"))))
	// login?id= lets a test choose which account the session belongs to.
	router.GET("/login", func(c *gin.Context) {
		user := child
		if c.Query("id") == fmt.Sprintf("%d", owner.Id) {
			user = owner
		}
		session := sessions.Default(c)
		session.Set("username", user.Username)
		session.Set("role", user.Role)
		session.Set("id", user.Id)
		session.Set("status", common.UserStatusEnabled)
		session.Set("group", user.Group)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	noContent := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	for _, path := range []string{
		workspaceAccountTokenRoute, workspaceAccountSelfRoute, workspaceAccountLogoutRoute,
		workspaceAccountUnlistedGetPath,
	} {
		router.GET(path, UserAuth(), noContent)
	}
	router.PUT(workspaceAccountSelfRoute, UserAuth(), noContent)
	// Registered but absent from the allowlist: a workspace account must not reach it even
	// though no explicit rejection middleware guards the route.
	router.POST(workspaceAccountForbiddenRoute, UserAuth(), noContent)
	// Same pattern as an allowed route but with a method that spends the owner's balance.
	router.DELETE(workspaceAccountSelfRoute, UserAuth(), noContent)
	return router, child, owner
}

func workspaceAccountSessionCookies(t *testing.T, router *gin.Engine, userID int) []*http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, fmt.Sprintf("/login?id=%d", userID), nil))
	require.Equal(t, http.StatusNoContent, recorder.Code)
	require.NotEmpty(t, recorder.Result().Cookies())
	return recorder.Result().Cookies()
}

func performWorkspaceAccountAuthRequest(router *gin.Engine, userID int, cookies []*http.Cookie, method string, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set("New-Api-User", fmt.Sprintf("%d", userID))
	for _, sessionCookie := range cookies {
		request.AddCookie(sessionCookie)
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestWorkspaceAccountMustChangePasswordGatesOtherEndpoints(t *testing.T) {
	router, child, _ := setupWorkspaceAccountAuthTest(t, true)
	cookies := workspaceAccountSessionCookies(t, router, child.Id)

	require.Equal(t, http.StatusForbidden, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountTokenRoute).Code)
	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountSelfRoute).Code)
	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodPut, workspaceAccountSelfRoute).Code)
	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountLogoutRoute).Code)
}

// Default-deny: an authenticated workspace account is refused on any route that is not on
// the allowlist, with no per-route rejection middleware involved.
func TestWorkspaceAccountPolicyDeniesRoutesOutsideAllowlist(t *testing.T) {
	router, child, _ := setupWorkspaceAccountAuthTest(t, false)
	cookies := workspaceAccountSessionCookies(t, router, child.Id)

	require.Equal(t, http.StatusForbidden, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodPost, workspaceAccountForbiddenRoute).Code)
	require.Equal(t, http.StatusForbidden, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountUnlistedGetPath).Code)
	// The allowlist is keyed by method as well as pattern.
	require.Equal(t, http.StatusForbidden, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodDelete, workspaceAccountSelfRoute).Code)
	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountTokenRoute).Code)
}

// The policy applies only to child accounts; a main account keeps full access.
func TestWorkspaceAccountPolicyDoesNotRestrictMainAccount(t *testing.T) {
	router, _, owner := setupWorkspaceAccountAuthTest(t, false)
	cookies := workspaceAccountSessionCookies(t, router, owner.Id)

	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, owner.Id, cookies, http.MethodPost, workspaceAccountForbiddenRoute).Code)
	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, owner.Id, cookies, http.MethodDelete, workspaceAccountSelfRoute).Code)
}

func TestDisabledWorkspaceAccountInvalidatesExistingSession(t *testing.T) {
	router, child, _ := setupWorkspaceAccountAuthTest(t, false)
	cookies := workspaceAccountSessionCookies(t, router, child.Id)
	require.Equal(t, http.StatusNoContent, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountTokenRoute).Code)

	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", child.Id).Update("status", common.UserStatusDisabled).Error)
	require.Equal(t, http.StatusUnauthorized, performWorkspaceAccountAuthRequest(router, child.Id, cookies, http.MethodGet, workspaceAccountTokenRoute).Code)
}
