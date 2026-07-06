package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func (s *Store) ListConfigVars(ctx context.Context, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT key, value FROM config_vars WHERE service_id = ? AND environment_id = ? ORDER BY key`),
		serviceID.String(), environmentID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	vars := make(map[string]string)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		vars[key] = value
	}
	return vars, rows.Err()
}

func (s *Store) MergeConfigVarsTx(ctx context.Context, tx *sql.Tx, serviceID, environmentID uuid.UUID, updates map[string]*string) error {
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	for key, value := range updates {
		if value == nil {
			if _, err := exec.ExecContext(ctx, s.q(`
				DELETE FROM config_vars WHERE service_id = ? AND environment_id = ? AND key = ?`),
				serviceID.String(), environmentID.String(), key); err != nil {
				return err
			}
			continue
		}
		_, err := exec.ExecContext(ctx, s.q(`
			INSERT INTO config_vars (service_id, environment_id, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_id, environment_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`),
			serviceID.String(), environmentID.String(), key, *value, now, now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MergeConfigVars(ctx context.Context, serviceID, environmentID uuid.UUID, updates map[string]*string) error {
	now := formatTime(s.driver, time.Now().UTC())
	for key, value := range updates {
		if value == nil {
			if _, err := s.db.ExecContext(ctx, s.q(`
				DELETE FROM config_vars WHERE service_id = ? AND environment_id = ? AND key = ?`),
				serviceID.String(), environmentID.String(), key); err != nil {
				return err
			}
			continue
		}
		_, err := s.db.ExecContext(ctx, s.q(`
			INSERT INTO config_vars (service_id, environment_id, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(service_id, environment_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`),
			serviceID.String(), environmentID.String(), key, *value, now, now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}