package model

import (
	"context"
	"database/sql"
	"encoding/base64"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/internal/testdb"
	"github.com/stretchr/testify/require"
)

func TestSeedanceUsageExportMySQL(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")) == "" {
		t.Skip("requires ASSET_TEST_MYSQL_DSN")
	}
	for _, collation := range []string{"utf8mb4_general_ci", "utf8mb4_unicode_ci"} {
		t.Run(collation, func(t *testing.T) { testSeedanceUsageExport(t, collation) })
	}
}

func testSeedanceUsageExport(t *testing.T, collation string) {
	db := testdb.AssetOpener(t, "")()
	pool, err := db.DB()
	require.NoError(t, err)
	conn, err := pool.Conn(context.Background())
	require.NoError(t, err)
	defer conn.Close()
	var name string
	require.NoError(t, conn.QueryRowContext(context.Background(), "SELECT DATABASE()").Scan(&name))
	exec := func(query string, args ...any) {
		_, err := conn.ExecContext(context.Background(), query, args...)
		require.NoError(t, err)
	}
	exec("CREATE TABLE logs (id INT PRIMARY KEY, created_at BIGINT, user_id INT, request_id VARCHAR(128), type INT, model_name VARCHAR(128), quota INT, prompt_tokens INT, completion_tokens INT, other TEXT) DEFAULT CHARSET utf8mb4 COLLATE " + collation)
	exec("CREATE TABLE tasks (id INT PRIMARY KEY, user_id INT, task_id VARCHAR(128), status VARCHAR(32), private_data TEXT, data TEXT) DEFAULT CHARSET utf8mb4 COLLATE " + collation)
	body := `{"model":"seedance-2","duration":4,"metadata":{"aspect_ratio":"16:9","video_url":"https://example.com/ref.mp4"}}`
	private, err := common.Marshal(map[string]any{"billing_context": map[string]any{"billing_request_input": map[string]any{"Body": base64.StdEncoding.EncodeToString([]byte(body))}}})
	require.NoError(t, err)
	exec("INSERT INTO tasks VALUES (1,1,'task-one','SUCCESS',?,?)", string(private), `{"usage":{"completion_tokens":100,"total_tokens":100}}`)
	exec("SET SESSION time_zone = '+08:00'")
	for i, row := range []struct{ kind, quota int }{{2, 250000}, {2, 250000}, {6, 100000}} {
		exec("INSERT INTO logs VALUES (?,UNIX_TIMESTAMP('2026-09-10 12:00:00'),1,'request-one',?,'SEEDANCE-2',?,0,100,?)", i+1, row.kind, row.quota, `{"task_id":"task-one"}`)
	}
	raw, err := os.ReadFile("../docs/operations/seedance-usage-export.mysql.sql")
	require.NoError(t, err)
	query := strings.ReplaceAll(string(raw), "`modelsell-log`", "`"+name+"`")
	query = strings.ReplaceAll(query, "`modelsell`", "`"+name+"`")
	query = strings.Replace(query, "SET @user_id = NULL;", "SET @user_id = 1;", 1)
	var lines []string
	for _, line := range strings.Split(query, "\n") {
		lines = append(lines, strings.SplitN(line, "--", 2)[0])
	}
	statements := strings.Split(strings.Join(lines, "\n"), ";")
	for _, statement := range statements {
		statement = strings.TrimSpace(statement)
		if strings.HasPrefix(statement, "SET ") {
			exec(statement)
		}
		if strings.HasPrefix(statement, "SELECT") {
			query = statement
		}
	}
	rows, err := conn.QueryContext(context.Background(), query)
	require.NoError(t, err)
	defer rows.Close()
	columns, err := rows.Columns()
	require.NoError(t, err)
	count, total := 0, 0.0
	var ratios []sql.NullString
	for rows.Next() {
		count++
		values := make([]sql.NullString, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		for i, column := range columns {
			switch column {
			case "净消耗（USD）":
				value, e := strconv.ParseFloat(values[i].String, 64)
				require.NoError(t, e)
				total += value
			case "是否含参考视频":
				require.Equal(t, "是", values[i].String)
			case "请求画面比例":
				ratios = append(ratios, values[i])
			}
		}
	}
	require.NoError(t, rows.Err())
	require.Equal(t, 3, count, "one export row per log")
	require.InDelta(t, 0.8, total, 0.0000001, "net charges match consumption minus refunds")
	t.Logf("rows=%d netUSD=%v ratios=%+v", count, total, ratios)
	for _, ratio := range ratios {
		require.Equal(t, "16:9", ratio.String, "metadata.aspect_ratio is a supported request parameter")
	}
}
