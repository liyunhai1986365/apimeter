package common

const (
	DatabaseTypeMySQL      = "mysql"
	DatabaseTypeSQLite     = "sqlite"
	DatabaseTypePostgreSQL = "postgres"
	DatabaseTypeClickHouse = "clickhouse"
)

type DatabaseType = string

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
	SetMainDatabaseType(mainType)
	SetLogDatabaseType(logType)
}

var UsingClickHouse = false

func MainDatabaseType() DatabaseType {
	switch {
	case UsingPostgreSQL:
		return DatabaseTypePostgreSQL
	case UsingMySQL:
		return DatabaseTypeMySQL
	default:
		return DatabaseTypeSQLite
	}
}

func LogDatabaseType() DatabaseType {
	return LogSqlType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	UsingSQLite = databaseType == DatabaseTypeSQLite
	UsingPostgreSQL = databaseType == DatabaseTypePostgreSQL
	UsingMySQL = databaseType == DatabaseTypeMySQL
}

func SetLogDatabaseType(databaseType DatabaseType) {
	LogSqlType = databaseType
	UsingClickHouse = databaseType == DatabaseTypeClickHouse
}

func UsingLogDatabase(databaseType string) bool {
	return LogSqlType == databaseType
}

var SQLitePath = "one-api.db?_busy_timeout=30000"
