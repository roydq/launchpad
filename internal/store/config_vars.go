package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
)

func (s *Store) ListConfigVars(ctx context.Context, appID uuid.UUID) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT key, value FROM config_vars WHERE app_id = ? ORDER BY key`), appID.String())
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

func (s *Store) MergeConfigVars(ctx context.Context, appID uuid.UUID, updates map[string]*string) error {
	now := formatTime(s.driver, time.Now().UTC())
	for key, value := range updates {
		if value == nil {
			if _, err := s.db.ExecContext(ctx, s.q(`DELETE FROM config_vars WHERE app_id = ? AND key = ?`),
				appID.String(), key); err != nil {
				return err
			}
			continue
		}
		_, err := s.db.ExecContext(ctx, s.q(`
			INSERT INTO config_vars (app_id, key, value, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(app_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`),
			appID.String(), key, *value, now, now,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateProcessType(ctx context.Context, pt *domain.ProcessType) error {
	if pt.ID == uuid.Nil {
		pt.ID = uuid.New()
	}
	now := time.Now().UTC()
	pt.CreatedAt = now
	pt.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, s.q(`
		INSERT INTO process_types (id, app_id, name, command, quantity, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		pt.ID.String(), pt.AppID.String(), pt.Name, pt.Command, pt.Quantity,
		formatTime(s.driver, pt.CreatedAt), formatTime(s.driver, pt.UpdatedAt),
	)
	return err
}

func (s *Store) ListProcessTypes(ctx context.Context, appID uuid.UUID) ([]domain.ProcessType, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, app_id, name, command, quantity, created_at, updated_at
		FROM process_types WHERE app_id = ? ORDER BY name`), appID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var processes []domain.ProcessType
	for rows.Next() {
		var pt domain.ProcessType
		var id, aid, createdAt, updatedAt string
		if err := rows.Scan(&id, &aid, &pt.Name, &pt.Command, &pt.Quantity, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		pt.ID = uuid.MustParse(id)
		pt.AppID = uuid.MustParse(aid)
		pt.CreatedAt = parseTime(s.driver, createdAt)
		pt.UpdatedAt = parseTime(s.driver, updatedAt)
		processes = append(processes, pt)
	}
	return processes, rows.Err()
}