package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// Workspace (sub-)accounts are default-deny: every authenticated route is closed to them
// unless it appears below. The previous design was the opposite — a blacklist of
// RejectWorkspaceSubaccount() call sites — so every newly added route silently defaulted
// to "a sub-account may call this", including top-up, affiliate and provider endpoints
// that spend the owner's money.
//
// Matching uses the gin route pattern (c.FullPath()) verbatim, never the raw URL, so no
// path trickery in the request can widen the allowlist. Entries must match the registered
// pattern exactly, trailing slash included; router/workspace_account_policy_test.go
// asserts every entry corresponds to a real route.

// workspaceAccountAllowedRoutes is keyed by "METHOD PATTERN". The method matters: a
// workspace account may read and update its own profile but must not delete the account.
var workspaceAccountAllowedRoutes = map[string]struct{}{
	// Identity and session
	"GET /api/user/self":        {},
	"PUT /api/user/self":        {},
	"GET /api/user/self/groups": {},
	"GET /api/user/models":      {},
	"PUT /api/user/setting":     {},
	"GET /api/user/logout":      {},
	"POST /api/verify":          {},
	"GET /api/models":           {},
	"GET /api/group/":           {},

	// Own security settings
	"GET /api/user/passkey":                        {},
	"DELETE /api/user/passkey":                     {},
	"POST /api/user/passkey/register/begin":        {},
	"POST /api/user/passkey/register/finish":       {},
	"POST /api/user/passkey/verify/begin":          {},
	"POST /api/user/passkey/verify/finish":         {},
	"GET /api/user/2fa/status":                     {},
	"POST /api/user/2fa/setup":                     {},
	"POST /api/user/2fa/enable":                    {},
	"POST /api/user/2fa/disable":                   {},
	"POST /api/user/2fa/backup_codes":              {},
	"GET /api/user/oauth/bindings":                 {},
	"DELETE /api/user/oauth/bindings/:provider_id": {},

	// Own usage views, already restricted to the account's workspaces by the scope filter
	"GET /api/dashboard/flow":       {},
	"GET /api/log/self":             {},
	"GET /api/log/self/stat":        {},
	"GET /api/log/self/search":      {},
	"GET /api/data/self":            {},
	"GET /api/data/self/dimensions": {},
	"GET /api/data/self/tokens":     {},
	"GET /api/task/self":            {},

	// The bare list route; the rest of the tree is covered by the prefixes below.
	"GET /api/workspaces": {},
}

// workspaceAccountAllowedPrefixes cover whole route trees. Mutating endpoints inside them
// stay guarded by RequireWorkspaceMainAccount(), and every query inside them is scoped by
// WorkspaceAccessScope.WorkspaceFilter().
var workspaceAccountAllowedPrefixes = []string{
	"/api/token/",      // key management inside the account's own workspaces
	"/api/workspaces/", // read its own workspaces; writes require a main account
}

// IsWorkspaceAccountRouteAllowed reports whether a workspace account may call the route.
func IsWorkspaceAccountRouteAllowed(method string, routePattern string) bool {
	if routePattern == "" {
		return false
	}
	if _, ok := workspaceAccountAllowedRoutes[method+" "+routePattern]; ok {
		return true
	}
	for _, prefix := range workspaceAccountAllowedPrefixes {
		if strings.HasPrefix(routePattern, prefix) {
			return true
		}
	}
	return false
}

// WorkspaceAccountAllowedRoutesForTest exposes the allowlist so router tests can assert
// every entry still matches a registered route.
func WorkspaceAccountAllowedRoutesForTest() map[string]struct{} {
	return workspaceAccountAllowedRoutes
}

// enforceWorkspaceAccountPolicy aborts the request when a workspace account reaches a
// route outside the allowlist. It reports whether the request may continue.
func enforceWorkspaceAccountPolicy(c *gin.Context, parentUserId int) bool {
	if parentUserId == 0 {
		return true
	}
	if IsWorkspaceAccountRouteAllowed(c.Request.Method, c.FullPath()) {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{
		"success": false,
		"message": "workspace accounts cannot access this feature",
	})
	c.Abort()
	return false
}
