package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) CreateProject(ctx context.Context, project *domain.Project, env *domain.Environment) error {
	if project.ID == uuid.Nil {
		project.ID = uuid.New()
	}
	now := time.Now().UTC()
	project.CreatedAt = now
	project.UpdatedAt = now
	if project.PrimaryService == "" {
		project.PrimaryService = project.Name
	}
	if project.Status == "" {
		project.Status = domain.ProjectStatusCreated
	}

	return s.Transact(ctx, func(tx *sql.Tx) error {
		_, err := s.exec(tx).ExecContext(ctx, s.q(`
			INSERT INTO projects (id, workspace_id, name, primary_service, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
			project.ID.String(), project.WorkspaceID.String(), project.Name, project.PrimaryService,
			string(project.Status), formatTime(s.driver, project.CreatedAt), formatTime(s.driver, project.UpdatedAt),
		)
		if err != nil {
			return fmt.Errorf("create project: %w", err)
		}

		if env == nil {
			env = &domain.Environment{Name: "dev"}
		}
		env.ProjectID = project.ID
		if err := s.createEnvironmentTx(ctx, tx, env); err != nil {
			return err
		}

		svc := &domain.Service{
			ProjectID: project.ID,
			Name:      project.PrimaryService,
		}
		if err := s.createServiceTx(ctx, tx, svc); err != nil {
			return err
		}

		process := &domain.Process{
			ServiceID: svc.ID,
			Name:      "web",
			Quantity:  1,
			Expose:    "http",
		}
		return s.createProcessTx(ctx, tx, process)
	})
}

func (s *Store) GetProjectByWorkspaceAndName(ctx context.Context, workspaceID uuid.UUID, name string) (*domain.Project, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, workspace_id, name, primary_service, status, created_at, updated_at, deleted_at
		FROM projects WHERE workspace_id = ? AND name = ? AND deleted_at IS NULL`),
		workspaceID.String(), name,
	)
	return scanProject(row, s.driver)
}

func (s *Store) GetProjectByID(ctx context.Context, id uuid.UUID) (*domain.Project, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, workspace_id, name, primary_service, status, created_at, updated_at, deleted_at
		FROM projects WHERE id = ? AND deleted_at IS NULL`), id.String())
	return scanProject(row, s.driver)
}

func (s *Store) ListProjectsByWorkspace(ctx context.Context, workspaceID uuid.UUID) ([]domain.Project, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, workspace_id, name, primary_service, status, created_at, updated_at, deleted_at
		FROM projects WHERE workspace_id = ? AND deleted_at IS NULL ORDER BY name`),
		workspaceID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []domain.Project
	for rows.Next() {
		p, err := scanProject(rows, s.driver)
		if err != nil {
			return nil, err
		}
		projects = append(projects, *p)
	}
	return projects, rows.Err()
}

func (s *Store) UpdateProjectStatus(ctx context.Context, projectID uuid.UUID, status domain.ProjectStatus) error {
	return s.UpdateProjectStatusTx(ctx, nil, projectID, status)
}

func (s *Store) UpdateProjectStatusTx(ctx context.Context, tx *sql.Tx, projectID uuid.UUID, status domain.ProjectStatus) error {
	_, err := s.exec(tx).ExecContext(ctx, s.q(`
		UPDATE projects SET status = ?, updated_at = ? WHERE id = ?`),
		string(status), formatTime(s.driver, time.Now().UTC()), projectID.String(),
	)
	return err
}

func scanProject(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Project, error) {
	var (
		id, workspaceID, name, primaryService, status string
		createdAt, updatedAt                            string
		deletedAt                                       sql.NullString
	)
	if err := scanner.Scan(&id, &workspaceID, &name, &primaryService, &status,
		&createdAt, &updatedAt, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}

	project := &domain.Project{
		ID:             uuid.MustParse(id),
		WorkspaceID:    uuid.MustParse(workspaceID),
		Name:           name,
		PrimaryService: primaryService,
		Status:         domain.ProjectStatus(status),
		CreatedAt:      parseTime(driver, createdAt),
		UpdatedAt:      parseTime(driver, updatedAt),
	}
	if deletedAt.Valid {
		t := parseTime(driver, deletedAt.String)
		project.DeletedAt = &t
	}
	return project, nil
}

func (s *Store) createEnvironmentTx(ctx context.Context, tx *sql.Tx, env *domain.Environment) error {
	if env.ID == uuid.Nil {
		env.ID = uuid.New()
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now
	if env.Name == "" {
		env.Name = "dev"
	}
	if env.TargetType == "" {
		env.TargetType = "kubernetes"
	}
	if len(env.TargetConfig) == 0 {
		env.TargetConfig = json.RawMessage(`{}`)
	}
	ephemeral := any(env.Ephemeral)
	if s.driver != DriverPostgres {
		if env.Ephemeral {
			ephemeral = 1
		} else {
			ephemeral = 0
		}
	}
	_, err := s.exec(tx).ExecContext(ctx, s.q(`
		INSERT INTO environments (id, project_id, name, target_type, target_config, ephemeral, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		env.ID.String(), env.ProjectID.String(), env.Name, env.TargetType,
		string(env.TargetConfig), ephemeral,
		formatTime(s.driver, env.CreatedAt), formatTime(s.driver, env.UpdatedAt),
	)
	return err
}