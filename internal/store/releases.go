package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) NextReleaseVersion(ctx context.Context, tx *sql.Tx, appID uuid.UUID) (int, error) {
	exec := s.exec(tx)
	row := exec.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(version), 0) + 1 FROM releases WHERE app_id = ?`), appID.String())
	var version int
	if err := row.Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func (s *Store) CreateRelease(ctx context.Context, tx *sql.Tx, release *domain.Release) error {
	if release.ID == uuid.Nil {
		release.ID = uuid.New()
	}
	release.CreatedAt = time.Now().UTC()
	if release.Status == "" {
		release.Status = domain.ReleaseStatusPending
	}
	snapshot, err := json.Marshal(release.ConfigSnapshot)
	if err != nil {
		return err
	}
	var buildID any
	if release.BuildID != nil {
		buildID = release.BuildID.String()
	}
	exec := s.exec(tx)
	_, err = exec.ExecContext(ctx, s.q(`
		INSERT INTO releases (id, app_id, build_id, version, config_snapshot, image_ref, status, description, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		release.ID.String(), release.AppID.String(), buildID, release.Version, string(snapshot),
		release.ImageRef, string(release.Status), release.Description, formatTime(s.driver, release.CreatedAt),
	)
	return err
}

func (s *Store) CreateDeployment(ctx context.Context, tx *sql.Tx, deployment *domain.Deployment) error {
	if deployment.ID == uuid.Nil {
		deployment.ID = uuid.New()
	}
	now := time.Now().UTC()
	deployment.CreatedAt = now
	deployment.UpdatedAt = now
	if deployment.StartedAt.IsZero() {
		deployment.StartedAt = now
	}
	if deployment.Status == "" {
		deployment.Status = domain.DeploymentPending
	}
	if deployment.Version == 0 {
		deployment.Version = 1
	}
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`
		INSERT INTO deployments (id, app_id, release_id, status, version, target_ref, message, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		deployment.ID.String(), deployment.AppID.String(), deployment.ReleaseID.String(),
		string(deployment.Status), deployment.Version, deployment.TargetRef, deployment.Message,
		formatTime(s.driver, deployment.StartedAt), formatTime(s.driver, deployment.CreatedAt),
		formatTime(s.driver, deployment.UpdatedAt),
	)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id uuid.UUID) (*domain.Deployment, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, app_id, release_id, status, version, target_ref, message, started_at, finished_at, created_at, updated_at
		FROM deployments WHERE id = ?`), id.String())
	return scanDeployment(row, s.driver)
}

func (s *Store) UpdateDeploymentStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, from domain.DeploymentStatus, to domain.DeploymentStatus, message string) error {
	if err := domain.ValidateDeploymentTransition(from, to); err != nil {
		return err
	}
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	var finishedAt any
	if to == domain.DeploymentRunning || to == domain.DeploymentFailed || to == domain.DeploymentCancelled || to == domain.DeploymentSuperseded {
		finishedAt = now
	}
	res, err := exec.ExecContext(ctx, s.q(`
		UPDATE deployments SET status = ?, message = ?, finished_at = COALESCE(?, finished_at), updated_at = ?, version = version + 1
		WHERE id = ? AND status = ?`),
		string(to), message, finishedAt, now, id.String(), string(from),
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return launchpad.ErrConflict
	}
	return nil
}

func (s *Store) ListReleases(ctx context.Context, appID uuid.UUID) ([]domain.Release, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, app_id, build_id, version, config_snapshot, image_ref, status, description, created_at
		FROM releases WHERE app_id = ? ORDER BY version DESC`), appID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var releases []domain.Release
	for rows.Next() {
		r, err := scanRelease(rows, s.driver)
		if err != nil {
			return nil, err
		}
		releases = append(releases, *r)
	}
	return releases, rows.Err()
}

func (s *Store) UpdateReleaseStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.ReleaseStatus) error {
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`UPDATE releases SET status = ? WHERE id = ?`), string(status), id.String())
	return err
}

func scanDeployment(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Deployment, error) {
	var id, appID, releaseID, status, targetRef, message, startedAt, createdAt, updatedAt string
	var version int
	var finishedAt sql.NullString
	if err := scanner.Scan(&id, &appID, &releaseID, &status, &version, &targetRef, &message,
		&startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	d := &domain.Deployment{
		ID:        uuid.MustParse(id),
		AppID:     uuid.MustParse(appID),
		ReleaseID: uuid.MustParse(releaseID),
		Status:    domain.DeploymentStatus(status),
		Version:   version,
		TargetRef: targetRef,
		Message:   message,
		StartedAt: parseTime(driver, startedAt),
		CreatedAt: parseTime(driver, createdAt),
		UpdatedAt: parseTime(driver, updatedAt),
	}
	if finishedAt.Valid {
		t := parseTime(driver, finishedAt.String)
		d.FinishedAt = &t
	}
	return d, nil
}

func scanRelease(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Release, error) {
	var id, appID, snapshot, imageRef, status, description, createdAt string
	var buildID sql.NullString
	var version int
	if err := scanner.Scan(&id, &appID, &buildID, &version, &snapshot, &imageRef, &status, &description, &createdAt); err != nil {
		return nil, err
	}
	var config map[string]string
	_ = json.Unmarshal([]byte(snapshot), &config)
	r := &domain.Release{
		ID:             uuid.MustParse(id),
		AppID:          uuid.MustParse(appID),
		Version:        version,
		ConfigSnapshot: config,
		ImageRef:       imageRef,
		Status:         domain.ReleaseStatus(status),
		Description:    description,
		CreatedAt:      parseTime(driver, createdAt),
	}
	if buildID.Valid {
		bid := uuid.MustParse(buildID.String)
		r.BuildID = &bid
	}
	return r, nil
}