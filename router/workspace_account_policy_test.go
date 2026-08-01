package router

import (
	"testing"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func registeredAPIRoutes(t *testing.T) []gin.RouteInfo {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetApiRouter(engine)
	return engine.Routes()
}

// The workspace-account policy matches gin route patterns verbatim, so a typo or a renamed
// route would silently deny a feature the account is supposed to reach. Assert instead that
// every allowlisted (method, pattern) pair still exists in the router.
func TestWorkspaceAccountAllowlistMatchesRegisteredRoutes(t *testing.T) {
	registered := make(map[string]struct{})
	for _, route := range registeredAPIRoutes(t) {
		registered[route.Method+" "+route.Path] = struct{}{}
	}
	for key := range middleware.WorkspaceAccountAllowedRoutesForTest() {
		_, ok := registered[key]
		require.True(t, ok, "allowlisted route %q is not registered", key)
	}
}

// Default-deny is the point of the policy: spot-check that the endpoints which move the
// owner's money or expose the owner's whole tenant stay closed.
func TestWorkspaceAccountPolicyDeniesSensitiveRoutes(t *testing.T) {
	denied := []struct {
		method string
		path   string
	}{
		{"POST", "/api/user/topup"},
		{"POST", "/api/user/pay"},
		{"POST", "/api/user/stripe/pay"},
		{"GET", "/api/user/stripe/auto-recharge"},
		{"POST", "/api/user/stripe/auto-recharge/setup"},
		{"GET", "/api/user/stripe/auto-recharge/setup"},
		{"PUT", "/api/user/stripe/auto-recharge"},
		{"DELETE", "/api/user/stripe/auto-recharge"},
		{"POST", "/api/user/aff_transfer"},
		{"GET", "/api/user/aff"},
		{"GET", "/api/user/token"},
		{"DELETE", "/api/user/self"},
		{"GET", "/api/user/self/providers"},
		{"POST", "/api/user/self/providers"},
		{"GET", "/api/user/checkin"},
		{"POST", "/api/user/checkin"},
		{"GET", "/api/billing/account-ledger"},
		{"GET", "/api/billing/breakdowns"},
		{"GET", "/api/subscription/self"},
		{"GET", "/api/mj/self"},
		{"GET", "/api/agent/self"},
		{"GET", "/api/workspace-subaccounts"},
		{"POST", "/api/workspace-subaccounts"},
		{"GET", "/api/log/"},
		{"GET", "/api/data/"},
		{"GET", "/api/task/"},
		{"GET", "/api/integrations/openmosaic/authorize"},
		{"POST", "/pg/chat/completions"},
	}
	for _, route := range denied {
		require.False(t, middleware.IsWorkspaceAccountRouteAllowed(route.method, route.path),
			"%s %s must stay closed to workspace accounts", route.method, route.path)
	}
}

// Every allowlisted route must still be reachable; this guards the inverse mistake of
// tightening the policy until the workspace console breaks.
func TestWorkspaceAccountPolicyAllowsConsoleRoutes(t *testing.T) {
	allowed := []struct {
		method string
		path   string
	}{
		{"GET", "/api/user/self"},
		{"PUT", "/api/user/self"},
		{"GET", "/api/token/"},
		{"POST", "/api/token/"},
		{"DELETE", "/api/token/:id"},
		{"POST", "/api/token/batch/keys"},
		{"GET", "/api/workspaces"},
		{"GET", "/api/workspaces/:id/usage"},
		{"GET", "/api/log/self"},
		{"GET", "/api/data/self/tokens"},
		{"GET", "/api/task/self"},
		{"GET", "/api/dashboard/flow"},
	}
	for _, route := range allowed {
		require.True(t, middleware.IsWorkspaceAccountRouteAllowed(route.method, route.path),
			"%s %s must stay open to workspace accounts", route.method, route.path)
	}
}
