package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var migrationFiles = map[Driver][]string{
	DriverSQLite: {
		"001_initial.sqlite.up.sql",
		"002_changeset_environment.sqlite.up.sql",
		"003_shared_config.sqlite.up.sql",
		"004_identity_principals.sqlite.up.sql",
	},
	DriverPostgres: {
		"001_initial.postgres.up.sql",
		"002_changeset_environment.postgres.up.sql",
		"003_shared_config.postgres.up.sql",
		"004_identity_principals.postgres.up.sql",
	},
}

func Migrate(ctx context.Context, db *sql.DB, driver Driver) error {
	if err := ensureSchemaMigrations(ctx, db, driver); err != nil {
		return err
	}
	if err := bootstrapLegacyMigrations(ctx, db, driver); err != nil {
		return err
	}

	files := migrationFiles[driver]
	for _, file := range files {
		version := strings.TrimSuffix(file, ".up.sql")
		applied, err := isMigrationApplied(ctx, db, driver, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		sqlBytes, err := migrationFS.ReadFile("migrations/" + file)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file, err)
		}
		if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("apply migration %s: %w", file, err)
		}
		if err := markMigrationApplied(ctx, db, driver, version); err != nil {
			return err
		}
	}
	return nil
}

func ensureSchemaMigrations(ctx context.Context, db *sql.DB, driver Driver) error {
	var ddl string
	if driver == DriverPostgres {
		ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`
	}
	_, err := db.ExecContext(ctx, ddl)
	return err
}

func bootstrapLegacyMigrations(ctx context.Context, db *sql.DB, driver Driver) error {
	exists, err := tableExists(ctx, db, driver, "apps")
	if err != nil {
		return err
	}
	if !exists {
		exists, err = tableExists(ctx, db, driver, "projects")
		if err != nil || !exists {
			return err
		}
	}
	files := migrationFiles[driver]
	if len(files) == 0 {
		return nil
	}
	initial := strings.TrimSuffix(files[0], ".up.sql")
	applied, err := isMigrationApplied(ctx, db, driver, initial)
	if err != nil {
		return err
	}
	if !applied {
		return markMigrationApplied(ctx, db, driver, initial)
	}
	return nil
}

func tableExists(ctx context.Context, db *sql.DB, driver Driver, name string) (bool, error) {
	var count int
	var err error
	if driver == DriverPostgres {
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = $1`, name).Scan(&count)
	} else {
		err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&count)
	}
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func isMigrationApplied(ctx context.Context, db *sql.DB, driver Driver, version string) (bool, error) {
	var count int
	err := db.QueryRowContext(ctx, rebind(driver, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`), version).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func markMigrationApplied(ctx context.Context, db *sql.DB, driver Driver, version string) error {
	now := formatTime(driver, time.Now().UTC())
	_, err := db.ExecContext(ctx, rebind(driver, `
		INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`), version, now)
	return err
}

// ListMigrations returns applied migration versions (for debugging).
func ListMigrations(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	sort.Strings(versions)
	return versions, rows.Err()
}