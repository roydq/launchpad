package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) GetWorkspaceByName(ctx context.Context, name string) (*domain.Workspace, error) {
	row := s.db.QueryRowContext(ctx, s.q(`SELECT id, name, created_at FROM workspaces WHERE name = ?`), name)
	return scanWorkspace(row, s.driver)
}

func (s *Store) GetWorkspaceByID(ctx context.Context, id uuid.UUID) (*domain.Workspace, error) {
	row := s.db.QueryRowContext(ctx, s.q(`SELECT id, name, created_at FROM workspaces WHERE id = ?`), id.String())
	return scanWorkspace(row, s.driver)
}

func scanWorkspace(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Workspace, error) {
	var id, name, createdAt string
	if err := scanner.Scan(&id, &name, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	return &domain.Workspace{
		ID:        uuid.MustParse(id),
		Name:      name,
		CreatedAt: parseTime(driver, createdAt),
	}, nil
}