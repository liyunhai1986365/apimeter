package model

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCheckDatabaseReadinessSQLiteWritable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:readiness-writable?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	require.NoError(t, CheckDatabaseReadiness(context.Background(), db))
}

func TestCheckDatabaseReadinessSQLiteReadOnly(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:readiness-readonly?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec("PRAGMA query_only = ON").Error)

	err = CheckDatabaseReadiness(context.Background(), db)
	require.ErrorContains(t, err, "read-only")
}
