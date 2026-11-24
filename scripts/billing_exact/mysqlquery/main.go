// mysqlquery runs one parameterized query against the suite's temporary MySQL.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	_ "github.com/go-sql-driver/mysql"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var request struct {
		SQL  string
		Args []json.RawMessage
	}
	if err := common.DecodeJson(os.Stdin, &request); err != nil {
		return err
	}
	args := make([]any, len(request.Args))
	for i, raw := range request.Args {
		// Avoid passing a BIGINT argument through an intermediate float64.
		if common.GetJsonType(raw) == "number" {
			n := string(raw)
			if signed, err := strconv.ParseInt(n, 10, 64); err == nil {
				args[i] = signed
			} else if unsigned, err := strconv.ParseUint(n, 10, 64); err == nil {
				args[i] = unsigned
			} else {
				args[i] = n
			}
		} else if err := common.Unmarshal(raw, &args[i]); err != nil {
			return err
		}
	}
	db, err := sql.Open("mysql", os.Getenv("BILLING_EXACT_DSN"))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, request.SQL, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(cols))
		dest := make([]any, len(cols))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return err
		}
		row := make(map[string]any, len(cols))
		for i, col := range cols {
			if b, ok := values[i].([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = values[i]
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	data, err := common.Marshal(result)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(data)
	return err
}
