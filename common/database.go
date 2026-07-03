package common

type DatabaseType string

const (
	DatabaseTypeMySQL      DatabaseType = "mysql"
	DatabaseTypeSQLite     DatabaseType = "sqlite"
	DatabaseTypePostgreSQL DatabaseType = "postgres"
	DatabaseTypeClickHouse DatabaseType = "clickhouse"
)

// Legacy compatibility flags kept for local tests and older call sites.
// New code should prefer MainDatabaseType/UsingMainDatabase.
var UsingSQLite bool
var UsingPostgreSQL bool
var UsingMySQL bool

var mainDatabaseType = DatabaseTypeSQLite
var logDatabaseType = DatabaseTypeSQLite

func MainDatabaseType() DatabaseType {
	return mainDatabaseType
}

func LogDatabaseType() DatabaseType {
	return logDatabaseType
}

func SetMainDatabaseType(databaseType DatabaseType) {
	mainDatabaseType = databaseType
	syncLegacyMainDatabaseFlags(databaseType)
}

func SetLogDatabaseType(databaseType DatabaseType) {
	logDatabaseType = databaseType
}

func SetDatabaseTypes(mainType DatabaseType, logType DatabaseType) {
	mainDatabaseType = mainType
	logDatabaseType = logType
	syncLegacyMainDatabaseFlags(mainType)
}

func UsingMainDatabase(databaseType DatabaseType) bool {
	if legacyType, ok := legacyMainDatabaseType(); ok {
		return legacyType == databaseType
	}
	return mainDatabaseType == databaseType
}

func UsingLogDatabase(databaseType DatabaseType) bool {
	return logDatabaseType == databaseType
}

func syncLegacyMainDatabaseFlags(databaseType DatabaseType) {
	UsingSQLite = databaseType == DatabaseTypeSQLite
	UsingMySQL = databaseType == DatabaseTypeMySQL
	UsingPostgreSQL = databaseType == DatabaseTypePostgreSQL
}

func legacyMainDatabaseType() (DatabaseType, bool) {
	switch {
	case UsingSQLite:
		return DatabaseTypeSQLite, true
	case UsingMySQL:
		return DatabaseTypeMySQL, true
	case UsingPostgreSQL:
		return DatabaseTypePostgreSQL, true
	default:
		return "", false
	}
}

var SQLitePath = "one-api.db?_busy_timeout=30000"
