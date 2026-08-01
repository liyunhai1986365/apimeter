package model

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// CheckDatabaseReadiness verifies that the database is reachable and writable.
// Application slave nodes still serve normal write-capable HTTP requests, so a
// read-only database must not be advertised as ready to a load balancer.
func CheckDatabaseReadiness(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get database connection: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	switch db.Dialector.Name() {
	case "mysql":
		var state struct {
			ReadOnly      int `gorm:"column:read_only"`
			SuperReadOnly int `gorm:"column:super_read_only"`
		}
		if err := db.WithContext(ctx).
			Raw("SELECT @@global.read_only AS read_only, @@global.super_read_only AS super_read_only").
			Scan(&state).Error; err != nil {
			return fmt.Errorf("check MySQL writable state: %w", err)
		}
		if state.ReadOnly != 0 || state.SuperReadOnly != 0 {
			return fmt.Errorf("database is read-only")
		}
	case "postgres":
		var state struct {
			InRecovery      bool   `gorm:"column:in_recovery"`
			TransactionMode string `gorm:"column:transaction_mode"`
		}
		if err := db.WithContext(ctx).
			Raw("SELECT pg_is_in_recovery() AS in_recovery, current_setting('transaction_read_only') AS transaction_mode").
			Scan(&state).Error; err != nil {
			return fmt.Errorf("check PostgreSQL writable state: %w", err)
		}
		if state.InRecovery || strings.EqualFold(state.TransactionMode, "on") {
			return fmt.Errorf("database is read-only")
		}
	case "sqlite":
		var queryOnly int
		if err := db.WithContext(ctx).Raw("PRAGMA query_only").Scan(&queryOnly).Error; err != nil {
			return fmt.Errorf("check SQLite writable state: %w", err)
		}
		if queryOnly != 0 {
			return fmt.Errorf("database is read-only")
		}
	default:
		return fmt.Errorf("unsupported database dialect: %s", db.Dialector.Name())
	}

	return nil
}
