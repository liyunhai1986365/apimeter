package model

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// AssetOperationTimeout bounds local ownership work, including pool waits and
// retries. Callers that have received an accepted upstream mutation use an
// independent context so client cancellation cannot abandon its bookkeeping.
const AssetOperationTimeout = 15 * time.Second

// Keep the part that can hold row locks much shorter than the whole operation.
const assetTransactionTimeout = 3 * time.Second

// MySQL 5.7 can keep executing a lock wait after the driver closes its socket.
// Bound the server wait too, so a canceled transaction cannot retain its earlier
// locks for the usual 50 seconds. This is per borrowed connection, never global.
const assetMySQLLockWaitSeconds = 2

func assetWriteTransaction(db *gorm.DB, write func(*gorm.DB) error) error {
	ctx, cancel := context.WithTimeout(db.Statement.Context, assetTransactionTimeout)
	defer cancel()
	db = db.WithContext(ctx)
	var err error
	if db.Dialector.Name() == common.DatabaseTypeMySQL {
		if conn, ok := db.Statement.ConnPool.(*sql.Conn); ok {
			err = assetMySQLWriteTransaction(db, conn, write)
		} else {
			err = db.Connection(func(pinned *gorm.DB) error {
				return assetMySQLWriteTransaction(pinned, pinned.Statement.ConnPool.(*sql.Conn), write)
			})
		}
	} else {
		err = db.Transaction(write)
	}
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func assetMySQLWriteTransaction(db *gorm.DB, conn *sql.Conn, write func(*gorm.DB) error) error {
	ctx := db.Statement.Context
	var previous int
	if err := conn.QueryRowContext(ctx, "SELECT @@SESSION.innodb_lock_wait_timeout").Scan(&previous); err != nil {
		return err
	}
	if previous <= assetMySQLLockWaitSeconds {
		return db.Transaction(write)
	}
	defer func() {
		// Cleanup uses this same reserved connection, never another pool slot.
		// If it cannot be reset, discard it so unrelated/video work never
		// inherits our shorter lock timeout. Keep an already committed result.
		if ctx.Err() != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return
		}
		restoreCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		if _, err := conn.ExecContext(restoreCtx, "SET SESSION innodb_lock_wait_timeout = ?", previous); err != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	if _, err := conn.ExecContext(ctx, "SET SESSION innodb_lock_wait_timeout = ?", assetMySQLLockWaitSeconds); err != nil {
		return err
	}
	return db.Transaction(write)
}
