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

func (s *Store) EnqueueJob(ctx context.Context, tx *sql.Tx, job *domain.Job) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	now := time.Now().UTC()
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.Status == "" {
		job.Status = domain.JobStatusQueued
	}
	if job.MaxAttempts == 0 {
		job.MaxAttempts = 5
	}
	if job.RunAt.IsZero() {
		job.RunAt = now
	}
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`
		INSERT INTO jobs (id, type, resource_type, resource_id, status, payload, attempt, max_attempts, run_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		job.ID.String(), string(job.Type), job.ResourceType, job.ResourceID.String(),
		string(job.Status), string(job.Payload), job.Attempt, job.MaxAttempts,
		formatTime(s.driver, job.RunAt), formatTime(s.driver, job.CreatedAt), formatTime(s.driver, job.UpdatedAt),
	)
	return err
}

func (s *Store) GetJob(ctx context.Context, id uuid.UUID) (*domain.Job, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, type, resource_type, resource_id, status, payload, attempt, max_attempts,
		       run_at, leased_until, leased_by, last_error, created_at, updated_at
		FROM jobs WHERE id = ?`), id.String())
	return scanJob(row, s.driver)
}

func (s *Store) LeaseNext(ctx context.Context, workerID string, types []domain.JobType, lease time.Duration) (*domain.Job, error) {
	typeStrings := make([]string, len(types))
	for i, t := range types {
		typeStrings[i] = string(t)
	}

	var job *domain.Job
	err := s.Transact(ctx, func(tx *sql.Tx) error {
		query := s.q(`
			SELECT id, type, resource_type, resource_id, status, payload, attempt, max_attempts,
			       run_at, leased_until, leased_by, last_error, created_at, updated_at
			FROM jobs
			WHERE status = 'queued' AND run_at <= ?
			ORDER BY run_at
			LIMIT 1`)
		if s.driver == DriverPostgres {
			query += " FOR UPDATE SKIP LOCKED"
		}

		row := tx.QueryRowContext(ctx, query, formatTime(s.driver, time.Now().UTC()))
		j, err := scanJob(row, s.driver)
		if err != nil {
			return err
		}

		leasedUntil := time.Now().UTC().Add(lease)
		_, err = tx.ExecContext(ctx, s.q(`
			UPDATE jobs SET status = ?, leased_until = ?, leased_by = ?, updated_at = ? WHERE id = ?`),
			string(domain.JobStatusLeased), formatTime(s.driver, leasedUntil), workerID,
			formatTime(s.driver, time.Now().UTC()), j.ID.String(),
		)
		if err != nil {
			return err
		}
		j.Status = domain.JobStatusLeased
		j.LeasedUntil = &leasedUntil
		j.LeasedBy = workerID
		job = j
		return nil
	})
	if errors.Is(err, launchpad.ErrNotFound) {
		return nil, nil
	}
	return job, err
}

func (s *Store) CompleteJob(ctx context.Context, id uuid.UUID, status domain.JobStatus, lastError string) error {
	_, err := s.db.ExecContext(ctx, s.q(`
		UPDATE jobs SET status = ?, last_error = ?, leased_until = NULL, leased_by = '', updated_at = ? WHERE id = ?`),
		string(status), lastError, formatTime(s.driver, time.Now().UTC()), id.String(),
	)
	return err
}

func (s *Store) ReclaimExpiredLeases(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE jobs SET status = 'queued', leased_until = NULL, leased_by = '', updated_at = ?
		WHERE status = 'leased' AND leased_until < ?`),
		formatTime(s.driver, time.Now().UTC()), formatTime(s.driver, time.Now().UTC()),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func scanJob(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Job, error) {
	var id, jobType, resourceType, resourceID, status, payload string
	var attempt, maxAttempts int
	var runAt string
	var leasedUntil, leasedBy, lastError sql.NullString
	var createdAt, updatedAt string

	if err := scanner.Scan(&id, &jobType, &resourceType, &resourceID, &status, &payload,
		&attempt, &maxAttempts, &runAt, &leasedUntil, &leasedBy, &lastError, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}

	job := &domain.Job{
		ID:           uuid.MustParse(id),
		Type:         domain.JobType(jobType),
		ResourceType: resourceType,
		ResourceID:   uuid.MustParse(resourceID),
		Status:       domain.JobStatus(status),
		Payload:      json.RawMessage(payload),
		Attempt:      attempt,
		MaxAttempts:  maxAttempts,
		RunAt:        parseTime(driver, runAt),
		CreatedAt:    parseTime(driver, createdAt),
		UpdatedAt:    parseTime(driver, updatedAt),
	}
	if leasedUntil.Valid {
		t := parseTime(driver, leasedUntil.String)
		job.LeasedUntil = &t
	}
	if leasedBy.Valid {
		job.LeasedBy = leasedBy.String
	}
	if lastError.Valid {
		job.LastError = lastError.String
	}
	return job, nil
}