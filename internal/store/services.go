package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) CreateService(ctx context.Context, svc *domain.Service) error {
	return s.createServiceTx(ctx, nil, svc)
}

func (s *Store) createServiceTx(ctx context.Context, tx *sql.Tx, svc *domain.Service) error {
	if svc.ID == uuid.Nil {
		svc.ID = uuid.New()
	}
	now := time.Now().UTC()
	svc.CreatedAt = now
	svc.UpdatedAt = now
	if svc.Kind == "" {
		svc.Kind = "application"
	}
	_, err := s.exec(tx).ExecContext(ctx, s.q(`
		INSERT INTO services (id, project_id, name, kind, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		svc.ID.String(), svc.ProjectID.String(), svc.Name, svc.Kind,
		formatTime(s.driver, svc.CreatedAt), formatTime(s.driver, svc.UpdatedAt),
	)
	return err
}

func (s *Store) GetServiceByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*domain.Service, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, project_id, name, kind, created_at, updated_at
		FROM services WHERE project_id = ? AND name = ?`), projectID.String(), name)
	return scanService(row, s.driver)
}

func (s *Store) GetServiceByID(ctx context.Context, id uuid.UUID) (*domain.Service, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, project_id, name, kind, created_at, updated_at
		FROM services WHERE id = ?`), id.String())
	return scanService(row, s.driver)
}

func scanService(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Service, error) {
	var id, projectID, name, kind, createdAt, updatedAt string
	if err := scanner.Scan(&id, &projectID, &name, &kind, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	return &domain.Service{
		ID:        uuid.MustParse(id),
		ProjectID: uuid.MustParse(projectID),
		Name:      name,
		Kind:      kind,
		CreatedAt: parseTime(driver, createdAt),
		UpdatedAt: parseTime(driver, updatedAt),
	}, nil
}