package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

func Migrate(ctx context.Context, db *sql.DB, driver Driver) error {
	filename := "migrations/001_initial.sqlite.up.sql"
	if driver == DriverPostgres {
		filename = "migrations/001_initial.postgres.up.sql"
	}

	sqlBytes, err := migrationFS.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}

	if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	return nil
}