package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) GetEnvironmentByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*domain.Environment, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, project_id, name, target_type, target_config, ephemeral, created_at, updated_at
		FROM environments WHERE project_id = ? AND name = ?`), projectID.String(), name)
	return scanEnvironment(row, s.driver)
}

func (s *Store) GetEnvironmentByID(ctx context.Context, id uuid.UUID) (*domain.Environment, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, project_id, name, target_type, target_config, ephemeral, created_at, updated_at
		FROM environments WHERE id = ?`), id.String())
	return scanEnvironment(row, s.driver)
}

func (s *Store) ListEnvironments(ctx context.Context, projectID uuid.UUID) ([]domain.Environment, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, project_id, name, target_type, target_config, ephemeral, created_at, updated_at
		FROM environments WHERE project_id = ? ORDER BY name`), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Environment
	for rows.Next() {
		env, err := scanEnvironment(rows, s.driver)
		if err != nil {
			return nil, err
		}
		out = append(out, *env)
	}
	return out, rows.Err()
}

func (s *Store) CreateEnvironment(ctx context.Context, env *domain.Environment) error {
	return s.Transact(ctx, func(tx *sql.Tx) error {
		return s.createEnvironmentTx(ctx, tx, env)
	})
}

func scanEnvironment(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Environment, error) {
	var id, projectID, name, targetType, targetConfig string
	var createdAt, updatedAt string
	var ephemeral bool
	if driver == DriverPostgres {
		if err := scanner.Scan(&id, &projectID, &name, &targetType, &targetConfig, &ephemeral,
			&createdAt, &updatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, launchpad.ErrNotFound
			}
			return nil, err
		}
	} else {
		var ephemeralInt int
		if err := scanner.Scan(&id, &projectID, &name, &targetType, &targetConfig, &ephemeralInt,
			&createdAt, &updatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, launchpad.ErrNotFound
			}
			return nil, err
		}
		ephemeral = ephemeralInt != 0
	}
	return &domain.Environment{
		ID:           uuid.MustParse(id),
		ProjectID:    uuid.MustParse(projectID),
		Name:         name,
		TargetType:   targetType,
		TargetConfig: json.RawMessage(targetConfig),
		Ephemeral:    ephemeral,
		CreatedAt:    parseTime(driver, createdAt),
		UpdatedAt:    parseTime(driver, updatedAt),
	}, nil
}
