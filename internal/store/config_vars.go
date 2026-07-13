package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func (s *Store) ListConfigVars(ctx context.Context, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	return s.ListConfigVarsTx(ctx, nil, serviceID, environmentID)
}

func (s *Store) ListConfigVarsTx(ctx context.Context, tx *sql.Tx, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	rows, err := s.exec(tx).QueryContext(ctx, s.q(`
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
	return s.Transact(ctx, func(tx *sql.Tx) error {
		return s.MergeConfigVarsTx(ctx, tx, serviceID, environmentID, updates)
	})
}

func (s *Store) ListSharedConfigVars(ctx context.Context, projectID, environmentID uuid.UUID) (map[string]string, error) {
	return s.ListSharedConfigVarsTx(ctx, nil, projectID, environmentID)
}

func (s *Store) ListSharedConfigVarsTx(ctx context.Context, tx *sql.Tx, projectID, environmentID uuid.UUID) (map[string]string, error) {
	rows, err := s.exec(tx).QueryContext(ctx, s.q(`
		SELECT key, value FROM shared_config_vars WHERE project_id = ? AND environment_id = ? ORDER BY key`),
		projectID.String(), environmentID.String())
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

func (s *Store) MergeSharedConfigVarsTx(ctx context.Context, tx *sql.Tx, projectID, environmentID uuid.UUID, updates map[string]*string) error {
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	for key, value := range updates {
		if value == nil {
			if _, err := exec.ExecContext(ctx, s.q(`
				DELETE FROM shared_config_vars WHERE project_id = ? AND environment_id = ? AND key = ?`),
				projectID.String(), environmentID.String(), key); err != nil {
				return err
			}
			continue
		}
		_, err := exec.ExecContext(ctx, s.q(`
			INSERT INTO shared_config_vars (project_id, environment_id, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(project_id, environment_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`),
			projectID.String(), environmentID.String(), key, *value, now, now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// ResolveConfig merges shared then service layers (service wins).
func (s *Store) ResolveConfigTx(ctx context.Context, tx *sql.Tx, projectID, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	shared, err := s.ListSharedConfigVarsTx(ctx, tx, projectID, environmentID)
	if err != nil {
		return nil, err
	}
	service, err := s.ListConfigVarsTx(ctx, tx, serviceID, environmentID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(shared)+len(service))
	for k, v := range shared {
		out[k] = v
	}
	for k, v := range service {
		out[k] = v
	}
	return out, nil
}

func (s *Store) ResolveConfig(ctx context.Context, projectID, serviceID, environmentID uuid.UUID) (map[string]string, error) {
	return s.ResolveConfigTx(ctx, nil, projectID, serviceID, environmentID)
}