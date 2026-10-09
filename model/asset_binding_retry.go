package model

import (
	"errors"
	"math/rand"
	"time"

	sqlitedriver "github.com/glebarez/go-sqlite"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// Retry only idempotent mutex initialization and the entire rolled-back local
// ownership transaction. Upstream operations must never run in this callback.
func retryAssetBindingWrite(db *gorm.DB, write func() error) error {
	const attempts = 4
	ctx := db.Statement.Context
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := write()
		if !isAssetBindingWriteConflict(err) || attempt == attempts-1 {
			return err
		}
		delay := (10 * time.Millisecond) << attempt
		timer := time.NewTimer(delay + time.Duration(rand.Int63n(int64(delay))))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func isAssetBindingWriteConflict(err error) bool {
	if errors.Is(err, errAssetBindingLockRace) || errors.Is(err, errAssetBindingPlanChanged) {
		return true
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1213
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40P01" || pgErr.Code == "40001"
	}
	var sqliteErr *sqlitedriver.Error
	if errors.As(err, &sqliteErr) {
		// Include extended SQLITE_BUSY codes such as SQLITE_BUSY_SNAPSHOT.
		return sqliteErr.Code()&0xff == 5
	}
	return false
}
