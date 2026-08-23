package store

import (
	"errors"
	"strings"
)

// Config selects the persistence backend for an environment.
type Config struct {
	Driver     string
	SQLitePath string
}

// OpenWithConfig opens the SQLite repository. The config wrapper remains for
// migration/CLI callers, but WOL deliberately has one persistence path now.
func OpenWithConfig(config Config) (Repository, error) {
	driver := strings.ToLower(strings.TrimSpace(config.Driver))
	if driver == "" {
		driver = "sqlite"
	}
	if driver != "sqlite" {
		return nil, errors.New("unsupported storage driver " + driver + "; WOL uses sqlite")
	}
	sqlitePath := strings.TrimSpace(config.SQLitePath)
	if sqlitePath == "" {
		sqlitePath = DefaultDatabasePath()
	}
	return Open(sqlitePath)
}
