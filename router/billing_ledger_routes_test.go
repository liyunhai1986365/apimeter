package router

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDerivedAccountLedgerRoutesAreNotRegistered(t *testing.T) {
	registered := make(map[string]struct{})
	for _, route := range registeredAPIRoutes(t) {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	disabled := []string{
		"GET /api/billing/account-ledger",
		"GET /api/billing/daily-reconciliations",
		"POST /api/billing/admin/backfill-v2",
		"POST /api/billing/admin/generate-recent-month",
	}
	for _, route := range disabled {
		_, ok := registered[route]
		require.False(t, ok, "derived account-ledger route %q must stay disabled", route)
	}
}
