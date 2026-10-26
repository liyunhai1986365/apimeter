package controller

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

// Compare the pre-fix owner-filtered queries with unfiltered scope discovery on
// a pinned connection so Handler_read_*
// deltas measure this query alone. No counters/settings/indexes are modified.
func diagnoseMultiUserAssetQueries(t *testing.T, pool *sql.DB, scope string, users int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	conn, err := pool.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	readRows := func(query string, args ...any) []map[string]string {
		rows, err := conn.QueryContext(ctx, query, args...)
		require.NoError(t, err)
		defer rows.Close()
		columns, err := rows.Columns()
		require.NoError(t, err)
		var result []map[string]string
		for rows.Next() {
			values := make([]sql.NullString, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			require.NoError(t, rows.Scan(pointers...))
			row := map[string]string{}
			for i, column := range columns {
				row[column] = values[i].String
			}
			result = append(result, row)
		}
		require.NoError(t, rows.Err())
		return result
	}
	counters := func() map[string]int64 {
		values := map[string]int64{}
		for _, row := range readRows("SHOW SESSION STATUS WHERE Variable_name IN ('Handler_read_first','Handler_read_key','Handler_read_next','Handler_read_rnd','Handler_read_rnd_next')") {
			value, err := strconv.ParseInt(row["Value"], 10, 64)
			require.NoError(t, err)
			values[row["Variable_name"]] = value
		}
		return values
	}
	const prefix = "channel_id, profile_id, resource_id, pre_request_id, user_id, token_id"
	const table = "configurable_resource_states FORCE INDEX (idx_configurable_resource_state_scope)"
	const account = "channel_id = 20 AND profile_id = 'asset-access-v1' AND resource_id = 'asset' AND pre_request_id = ?"
	var report []map[string]any
	for _, user := range []int{1, users / 2, users} {
		for _, variant := range []string{"owner_scope_discovery", "owner_first_page", "scope_discovery_without_owner_filter"} {
			query := "SELECT " + prefix + " FROM " + table + " WHERE " + account
			args := []any{scope}
			switch variant {
			case "owner_scope_discovery":
				query += " AND state_value = ? AND status = 'active' GROUP BY " + prefix + " ORDER BY user_id, token_id"
				args = append(args, fmt.Sprint(user))
			case "owner_first_page":
				query = "SELECT id, metadata, state_key FROM " + table + " WHERE " + account + " AND state_value = ? AND status = 'active' AND user_id = 0 AND token_id = 0 AND state_key > '' ORDER BY state_key LIMIT 500"
				args = append(args, fmt.Sprint(user))
			case "scope_discovery_without_owner_filter":
				query += " GROUP BY " + prefix + " ORDER BY user_id, token_id"
			}
			explain := readRows("EXPLAIN "+query, args...)
			before := counters()
			start := time.Now()
			rows := readRows(query, args...)
			elapsed := float64(time.Since(start)) / float64(time.Millisecond)
			after := counters()
			for key := range after {
				after[key] -= before[key]
			}
			report = append(report, map[string]any{"variant": variant, "user": user, "sql": query, "explain": explain, "duration_ms": elapsed, "returned_rows": len(rows), "handler_reads": after})
			t.Logf("SQL diagnosis %s user=%d %.2fms rows=%d reads=%v", variant, user, elapsed, len(rows), after)
		}
	}
	data, err := common.Marshal(report)
	require.NoError(t, err)
	path := filepath.Join(filepath.Dir(os.Getenv("ASSET_LOAD_REPORT_PATH")), "query-diagnosis.json")
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0600))
}
