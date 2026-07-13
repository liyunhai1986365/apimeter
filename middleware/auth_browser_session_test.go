package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestBrowserSessionAuthDoesNotRequireNewAPIUserHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("session", cookie.NewStore([]byte("browser-session-auth-test"))))
	router.GET("/login", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Set("id", 42)
		session.Set("username", "browser-user")
		session.Set("role", common.RoleCommonUser)
		session.Set("status", common.UserStatusEnabled)
		require.NoError(t, session.Save())
		c.Status(http.StatusNoContent)
	})
	router.GET("/authorize", BrowserSessionAuth(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.GetInt("id")})
	})

	login := httptest.NewRecorder()
	router.ServeHTTP(login, httptest.NewRequest(http.MethodGet, "/login", nil))
	require.NotEmpty(t, login.Result().Cookies())

	request := httptest.NewRequest(http.MethodGet, "/authorize", nil)
	request.AddCookie(login.Result().Cookies()[0])
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	require.Contains(t, response.Body.String(), `"id":42`)
}
