package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "duplicate key") ||
		strings.Contains(msg, "unique index")
}

func (s *Store) NextReleaseVersion(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID) (int, error) {
	exec := s.exec(tx)
	row := exec.QueryRowContext(ctx, s.q(`SELECT COALESCE(MAX(version), 0) + 1 FROM releases WHERE service_id = ?`), serviceID.String())
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
	sens := release.ConfigSensitivity
	if sens == nil {
		sens = map[string]string{}
	}
	// Seal secret values for durable storage; in-memory release.ConfigResolved stays plaintext.
	sealedConfig, err := s.sealConfigMaps(release.ConfigResolved, sens)
	if err != nil {
		return err
	}
	configResolved, err := json.Marshal(sealedConfig)
	if err != nil {
		return err
	}
	processSnapshot, err := json.Marshal(release.ProcessSnapshot)
	if err != nil {
		return err
	}
	configSensitivity, err := json.Marshal(sens)
	if err != nil {
		return err
	}
	var createdByPrincipal, createdByToken any
	if release.CreatedByPrincipalID != nil {
		createdByPrincipal = release.CreatedByPrincipalID.String()
	}
	if release.CreatedByTokenID != nil {
		createdByToken = release.CreatedByTokenID.String()
	}
	exec := s.exec(tx)
	_, err = exec.ExecContext(ctx, s.q(`
		INSERT INTO releases (id, service_id, version, artifact_ref, config_resolved, process_snapshot, status, description, created_at, created_by_principal_id, created_by_token_id, config_sensitivity)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		release.ID.String(), release.ServiceID.String(), release.Version, release.ArtifactRef,
		string(configResolved), string(processSnapshot), string(release.Status), release.Description,
		formatTime(s.driver, release.CreatedAt), createdByPrincipal, createdByToken, string(configSensitivity),
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
		INSERT INTO deployments (id, service_id, environment_id, release_id, status, version, target_ref, message, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		deployment.ID.String(), deployment.ServiceID.String(), deployment.EnvironmentID.String(),
		deployment.ReleaseID.String(), string(deployment.Status), deployment.Version,
		deployment.TargetRef, deployment.Message,
		formatTime(s.driver, deployment.StartedAt), formatTime(s.driver, deployment.CreatedAt),
		formatTime(s.driver, deployment.UpdatedAt),
	)
	if err != nil && isUniqueViolation(err) {
		return fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict)
	}
	return err
}

func (s *Store) HasActiveDeployment(ctx context.Context, serviceID, environmentID uuid.UUID) (bool, error) {
	return s.HasActiveDeploymentTx(ctx, nil, serviceID, environmentID)
}

func (s *Store) HasActiveDeploymentTx(ctx context.Context, tx *sql.Tx, serviceID, environmentID uuid.UUID) (bool, error) {
	row := s.exec(tx).QueryRowContext(ctx, s.q(`
		SELECT COUNT(*) FROM deployments
		WHERE service_id = ? AND environment_id = ? AND status IN ('pending', 'deploying')`),
		serviceID.String(), environmentID.String())
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// SupersedeRunningDeployments marks all running deployments for the service×env as
// superseded, optionally excluding one deployment ID (the new active deploy).
func (s *Store) SupersedeRunningDeployments(ctx context.Context, tx *sql.Tx, serviceID, environmentID, exceptID uuid.UUID) error {
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	_, err := exec.ExecContext(ctx, s.q(`
		UPDATE deployments
		SET status = ?, message = ?, finished_at = COALESCE(finished_at, ?), updated_at = ?, version = version + 1
		WHERE service_id = ? AND environment_id = ? AND status = ? AND id != ?`),
		string(domain.DeploymentSuperseded), "superseded by newer deployment", now, now,
		serviceID.String(), environmentID.String(), string(domain.DeploymentRunning), exceptID.String(),
	)
	return err
}

func (s *Store) GetDeployment(ctx context.Context, id uuid.UUID) (*domain.Deployment, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, service_id, environment_id, release_id, status, version, target_ref, message, started_at, finished_at, created_at, updated_at
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

const releaseSelectCols = `id, service_id, version, artifact_ref, config_resolved, process_snapshot, status, description, created_at, created_by_principal_id, created_by_token_id, config_sensitivity`

func (s *Store) GetReleaseByID(ctx context.Context, id uuid.UUID) (*domain.Release, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT `+releaseSelectCols+`
		FROM releases WHERE id = ?`), id.String())
	return s.scanRelease(row)
}

func (s *Store) GetReleaseByVersion(ctx context.Context, serviceID uuid.UUID, version int) (*domain.Release, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT `+releaseSelectCols+`
		FROM releases WHERE service_id = ? AND version = ?`), serviceID.String(), version)
	return s.scanRelease(row)
}

func (s *Store) GetLatestSucceededRelease(ctx context.Context, serviceID uuid.UUID) (*domain.Release, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT `+releaseSelectCols+`
		FROM releases WHERE service_id = ? AND status = 'succeeded'
		ORDER BY version DESC LIMIT 1`), serviceID.String())
	return s.scanRelease(row)
}

func (s *Store) ListReleases(ctx context.Context, serviceID uuid.UUID) ([]domain.Release, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT `+releaseSelectCols+`
		FROM releases WHERE service_id = ? ORDER BY version DESC`), serviceID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var releases []domain.Release
	for rows.Next() {
		r, err := s.scanRelease(rows)
		if err != nil {
			return nil, err
		}
		releases = append(releases, *r)
	}
	return releases, rows.Err()
}

// GetLatestDeploymentForServiceEnv returns the most recently created deployment for a service×env pair.
func (s *Store) GetLatestDeploymentForServiceEnv(ctx context.Context, serviceID, environmentID uuid.UUID) (*domain.Deployment, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, service_id, environment_id, release_id, status, version, target_ref, message, started_at, finished_at, created_at, updated_at
		FROM deployments
		WHERE service_id = ? AND environment_id = ?
		ORDER BY created_at DESC LIMIT 1`), serviceID.String(), environmentID.String())
	return scanDeployment(row, s.driver)
}

// ListDeploymentsForService returns deployments for a service ordered by created_at desc (for release annotations).
func (s *Store) ListDeploymentsForService(ctx context.Context, serviceID uuid.UUID) ([]domain.Deployment, error) {
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, service_id, environment_id, release_id, status, version, target_ref, message, started_at, finished_at, created_at, updated_at
		FROM deployments WHERE service_id = ? ORDER BY created_at DESC`), serviceID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Deployment
	for rows.Next() {
		d, err := scanDeployment(rows, s.driver)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (s *Store) UpdateReleaseStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.ReleaseStatus) error {
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`UPDATE releases SET status = ? WHERE id = ?`), string(status), id.String())
	return err
}

func scanDeployment(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Deployment, error) {
	var id, serviceID, environmentID, releaseID, status, targetRef, message, startedAt, createdAt, updatedAt string
	var version int
	var finishedAt sql.NullString
	if err := scanner.Scan(&id, &serviceID, &environmentID, &releaseID, &status, &version, &targetRef, &message,
		&startedAt, &finishedAt, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	d := &domain.Deployment{
		ID:            uuid.MustParse(id),
		ServiceID:     uuid.MustParse(serviceID),
		EnvironmentID: uuid.MustParse(environmentID),
		ReleaseID:     uuid.MustParse(releaseID),
		Status:        domain.DeploymentStatus(status),
		Version:       version,
		TargetRef:     targetRef,
		Message:       message,
		StartedAt:     parseTime(driver, startedAt),
		CreatedAt:     parseTime(driver, createdAt),
		UpdatedAt:     parseTime(driver, updatedAt),
	}
	if finishedAt.Valid {
		t := parseTime(driver, finishedAt.String)
		d.FinishedAt = &t
	}
	return d, nil
}

func (s *Store) scanRelease(scanner interface{ Scan(...any) error }) (*domain.Release, error) {
	var id, serviceID, artifactRef, configResolved, processSnapshot, status, description, createdAt string
	var version int
	var createdByPrincipal, createdByToken, configSensitivity sql.NullString
	if err := scanner.Scan(&id, &serviceID, &version, &artifactRef, &configResolved, &processSnapshot,
		&status, &description, &createdAt, &createdByPrincipal, &createdByToken, &configSensitivity); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	var config map[string]string
	_ = json.Unmarshal([]byte(configResolved), &config)
	var snapshot map[string]domain.ProcessSnapshot
	_ = json.Unmarshal([]byte(processSnapshot), &snapshot)
	var sens map[string]string
	if configSensitivity.Valid && configSensitivity.String != "" {
		_ = json.Unmarshal([]byte(configSensitivity.String), &sens)
	}
	opened, err := s.openConfigMaps(config, sens)
	if err != nil {
		return nil, err
	}
	rel := &domain.Release{
		ID:                uuid.MustParse(id),
		ServiceID:         uuid.MustParse(serviceID),
		Version:           version,
		ArtifactRef:       artifactRef,
		ConfigResolved:    opened,
		ConfigSensitivity: sens,
		ProcessSnapshot:   snapshot,
		Status:            domain.ReleaseStatus(status),
		Description:       description,
		CreatedAt:         parseTime(s.driver, createdAt),
	}
	if createdByPrincipal.Valid && createdByPrincipal.String != "" {
		pid := uuid.MustParse(createdByPrincipal.String)
		rel.CreatedByPrincipalID = &pid
	}
	if createdByToken.Valid && createdByToken.String != "" {
		tid := uuid.MustParse(createdByToken.String)
		rel.CreatedByTokenID = &tid
	}
	return rel, nil
}