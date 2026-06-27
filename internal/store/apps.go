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

func (s *Store) CreateApp(ctx context.Context, app *domain.App) error {
	if app.ID == uuid.Nil {
		app.ID = uuid.New()
	}
	now := time.Now().UTC()
	app.CreatedAt = now
	app.UpdatedAt = now
	if app.Stack == "" {
		app.Stack = "container"
	}
	if app.TargetType == "" {
		app.TargetType = "kubernetes"
	}
	if len(app.TargetConfig) == 0 {
		app.TargetConfig = json.RawMessage(`{}`)
	}
	if app.Status == "" {
		app.Status = domain.AppStatusCreated
	}

	_, err := s.db.ExecContext(ctx, s.q(`
		INSERT INTO apps (id, team_id, name, stack, target_type, target_config, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		app.ID.String(), app.TeamID.String(), app.Name, app.Stack, app.TargetType,
		string(app.TargetConfig), string(app.Status), formatTime(s.driver, app.CreatedAt), formatTime(s.driver, app.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("create app: %w", err)
	}

	// Default web process
	process := &domain.ProcessType{
		ID:       uuid.New(),
		AppID:    app.ID,
		Name:     "web",
		Quantity: 1,
	}
	return s.CreateProcessType(ctx, process)
}

func (s *Store) GetAppByTeamAndName(ctx context.Context, teamID uuid.UUID, name string) (*domain.App, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, team_id, name, stack, target_type, target_config, status, active_deployment_id,
		       created_at, updated_at, deleted_at
		FROM apps WHERE team_id = ? AND name = ? AND deleted_at IS NULL`),
		teamID.String(), name,
	)
	return scanApp(row, s.driver)
}

func (s *Store) GetAppByID(ctx context.Context, id uuid.UUID) (*domain.App, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, team_id, name, stack, target_type, target_config, status, active_deployment_id,
		       created_at, updated_at, deleted_at
		FROM apps WHERE id = ? AND deleted_at IS NULL`), id.String())
	return scanApp(row, s.driver)
}

func (s *Store) ListAppsByTeam(ctx context.Context, teamID uuid.UUID) ([]domain.App, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, team_id, name, stack, target_type, target_config, status, active_deployment_id,
		       created_at, updated_at, deleted_at
		FROM apps WHERE team_id = ? AND deleted_at IS NULL ORDER BY name`),
		teamID.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []domain.App
	for rows.Next() {
		app, err := scanApp(rows, s.driver)
		if err != nil {
			return nil, err
		}
		apps = append(apps, *app)
	}
	return apps, rows.Err()
}

func (s *Store) UpdateAppStatus(ctx context.Context, appID uuid.UUID, status domain.AppStatus) error {
	_, err := s.db.ExecContext(ctx, s.q(`
		UPDATE apps SET status = ?, updated_at = ? WHERE id = ?`),
		string(status), formatTime(s.driver, time.Now().UTC()), appID.String(),
	)
	return err
}

func (s *Store) SetActiveDeployment(ctx context.Context, tx *sql.Tx, appID, deploymentID uuid.UUID) error {
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`
		UPDATE apps SET active_deployment_id = ?, status = ?, updated_at = ? WHERE id = ?`),
		deploymentID.String(), string(domain.AppStatusDeploying),
		formatTime(s.driver, time.Now().UTC()), appID.String(),
	)
	return err
}

func (s *Store) ClearActiveDeployment(ctx context.Context, tx *sql.Tx, appID uuid.UUID, status domain.AppStatus) error {
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`
		UPDATE apps SET active_deployment_id = NULL, status = ?, updated_at = ? WHERE id = ?`),
		string(status), formatTime(s.driver, time.Now().UTC()), appID.String(),
	)
	return err
}

func scanApp(scanner interface{ Scan(...any) error }, driver Driver) (*domain.App, error) {
	var (
		id, teamID, name, stack, targetType, targetConfig, status string
		activeDeployment                                          sql.NullString
		createdAt, updatedAt                                      string
		deletedAt                                                 sql.NullString
	)
	if err := scanner.Scan(&id, &teamID, &name, &stack, &targetType, &targetConfig, &status,
		&activeDeployment, &createdAt, &updatedAt, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}

	app := &domain.App{
		ID:           uuid.MustParse(id),
		TeamID:       uuid.MustParse(teamID),
		Name:         name,
		Stack:        stack,
		TargetType:   targetType,
		TargetConfig: json.RawMessage(targetConfig),
		Status:       domain.AppStatus(status),
		CreatedAt:    parseTime(driver, createdAt),
		UpdatedAt:    parseTime(driver, updatedAt),
	}
	if activeDeployment.Valid {
		depID := uuid.MustParse(activeDeployment.String)
		app.ActiveDeploymentID = &depID
	}
	if deletedAt.Valid {
		t := parseTime(driver, deletedAt.String)
		app.DeletedAt = &t
	}
	return app, nil
}