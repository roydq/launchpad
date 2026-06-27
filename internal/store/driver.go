package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Driver string

const (
	DriverPostgres Driver = "postgres"
	DriverSQLite   Driver = "sqlite"
)

func Open(ctx context.Context, databaseURL string) (*sql.DB, Driver, error) {
	driver, dsn, err := parseDatabaseURL(databaseURL)
	if err != nil {
		return nil, "", err
	}

	db, err := sql.Open(string(driver), dsn)
	if err != nil {
		return nil, "", err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, "", fmt.Errorf("ping database: %w", err)
	}
	return db, driver, nil
}

func parseDatabaseURL(databaseURL string) (Driver, string, error) {
	switch {
	case strings.HasPrefix(databaseURL, "postgres://"),
		strings.HasPrefix(databaseURL, "postgresql://"):
		return DriverPostgres, databaseURL, nil
	case strings.HasPrefix(databaseURL, "file:"),
		strings.HasSuffix(databaseURL, ".db"),
		databaseURL == ":memory:":
		dsn := databaseURL
		if !strings.HasPrefix(databaseURL, "file:") && databaseURL != ":memory:" {
			dsn = "file:" + databaseURL + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
		}
		return DriverSQLite, dsn, nil
	default:
		return "", "", fmt.Errorf("unsupported database URL: %s", databaseURL)
	}
}