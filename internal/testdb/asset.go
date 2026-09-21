// Package testdb provides isolated databases for the asset integration tests.
// It is imported by test files only.
package testdb

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	driver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// AssetOpener preserves the default SQLite fixture unless ASSET_TEST_MYSQL_DSN
// is explicitly set. The supplied MySQL database is never modified: each test
// creates a random schema and drops only that schema during cleanup. Reopening
// uses the same schema so persistence can be checked across connections.
func AssetOpener(t *testing.T, sqliteDSN string) func() *gorm.DB {
	t.Helper()
	dialector := func() gorm.Dialector { return sqlite.Open(sqliteDSN) }
	if dsn := strings.TrimSpace(os.Getenv("ASSET_TEST_MYSQL_DSN")); dsn != "" {
		cfg, err := driver.ParseDSN(dsn)
		if err != nil {
			t.Fatal("invalid ASSET_TEST_MYSQL_DSN")
		}
		cfg.DBName = ""
		cfg.ParseTime = true
		cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 15*time.Second, 15*time.Second
		admin, err := sql.Open("mysql", cfg.FormatDSN())
		if err != nil {
			t.Fatalf("open MySQL test server: %v", err)
		}
		t.Cleanup(func() { _ = admin.Close() })
		var nonce [12]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("apimeter_asset_test_%x", nonce)
		if _, err := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatalf("create isolated MySQL test schema: %v", err)
		}
		t.Cleanup(func() {
			if _, err := admin.Exec("DROP DATABASE `" + name + "`"); err != nil {
				t.Errorf("drop isolated MySQL test schema: %v", err)
			}
		})
		cfg.DBName = name
		dialector = func() gorm.Dialector { return mysql.Open(cfg.FormatDSN()) }
		var version string
		if err := admin.QueryRow("SELECT VERSION()").Scan(&version); err != nil {
			t.Fatal(err)
		}
		t.Logf("asset mock database: MySQL %s, isolated schema %s", version, name)
	}
	return func() *gorm.DB {
		t.Helper()
		db, err := gorm.Open(dialector(), &gorm.Config{})
		if err != nil {
			t.Fatalf("open asset test database: %v", err)
		}
		pool, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		pool.SetMaxOpenConns(8)
		t.Cleanup(func() { _ = pool.Close() })
		return db
	}
}
