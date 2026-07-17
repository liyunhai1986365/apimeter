package common

const (
	DatabaseTypeMySQL      = "mysql"
	DatabaseTypeSQLite     = "sqlite"
	DatabaseTypePostgreSQL = "postgres"
)

var UsingSQLite = false
var UsingPostgreSQL = false
var LogSqlType = DatabaseTypeSQLite // Default to SQLite for logging SQL queries
var UsingMySQL = false

func UsingMainDatabase(databaseType string) bool {
	switch databaseType {
	case DatabaseTypeSQLite:
		return UsingSQLite
	case DatabaseTypePostgreSQL:
		return UsingPostgreSQL
	case DatabaseTypeMySQL:
		return UsingMySQL
	default:
		return false
	}
}

func SetDatabaseTypes(mainType, logType string) {
	UsingSQLite = mainType == DatabaseTypeSQLite
	UsingPostgreSQL = mainType == DatabaseTypePostgreSQL
	UsingMySQL = mainType == DatabaseTypeMySQL
	LogSqlType = logType
}

var UsingClickHouse = false

var SQLitePath = "one-api.db?_busy_timeout=30000"
