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

func (s *Store) GetOrCreateOpenChangeset(ctx context.Context, tx *sql.Tx, projectID uuid.UUID) (*domain.Changeset, error) {
	cs, err := s.getOpenChangeset(ctx, tx, projectID)
	if err == nil {
		return cs, nil
	}
	if !errors.Is(err, launchpad.ErrNotFound) {
		return nil, err
	}

	now := time.Now().UTC()
	cs = &domain.Changeset{
		ID:        uuid.New(),
		ProjectID: projectID,
		Status:    domain.ChangesetOpen,
		CreatedAt: now,
		UpdatedAt: now,
	}
	exec := s.exec(tx)
	_, err = exec.ExecContext(ctx, s.q(`
		INSERT INTO changesets (id, project_id, environment_id, status, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		cs.ID.String(), cs.ProjectID.String(), nil, string(cs.Status), cs.Description,
		formatTime(s.driver, cs.CreatedAt), formatTime(s.driver, cs.UpdatedAt),
	)
	return cs, err
}

func (s *Store) GetOpenChangeset(ctx context.Context, projectID uuid.UUID) (*domain.Changeset, error) {
	return s.getOpenChangeset(ctx, nil, projectID)
}

func (s *Store) getOpenChangeset(ctx context.Context, tx *sql.Tx, projectID uuid.UUID) (*domain.Changeset, error) {
	row := s.exec(tx).QueryRowContext(ctx, s.q(`
		SELECT id, project_id, environment_id, status, description, created_at, updated_at
		FROM changesets WHERE project_id = ? AND status = 'open'`), projectID.String())

	cs, err := scanChangeset(row, s.driver)
	if err != nil {
		return nil, err
	}
	changes, err := s.listChangesetChanges(ctx, tx, cs.ID)
	if err != nil {
		return nil, err
	}
	cs.Changes = changes
	return cs, nil
}

// SetChangesetEnvironment pins an open changeset to an environment (idempotent if already set to same).
func (s *Store) SetChangesetEnvironment(ctx context.Context, tx *sql.Tx, changesetID, environmentID uuid.UUID) error {
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	_, err := exec.ExecContext(ctx, s.q(`
		UPDATE changesets SET environment_id = ?, updated_at = ? WHERE id = ? AND status = 'open'`),
		environmentID.String(), now, changesetID.String(),
	)
	return err
}

func (s *Store) AddChangesetChanges(ctx context.Context, tx *sql.Tx, changesetID uuid.UUID, changes []domain.ChangesetChange) error {
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	for i := range changes {
		if changes[i].ID == uuid.Nil {
			changes[i].ID = uuid.New()
		}
		changes[i].ChangesetID = changesetID
		changes[i].CreatedAt = time.Now().UTC()
		var serviceID any
		if changes[i].ServiceID != nil {
			serviceID = changes[i].ServiceID.String()
		}
		_, err := exec.ExecContext(ctx, s.q(`
			INSERT INTO changeset_changes (id, changeset_id, service_id, service_name, type, payload, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`),
			changes[i].ID.String(), changesetID.String(), serviceID, changes[i].ServiceName,
			string(changes[i].Type), string(changes[i].Payload), now,
		)
		if err != nil {
			return err
		}
	}
	_, err := exec.ExecContext(ctx, s.q(`UPDATE changesets SET updated_at = ? WHERE id = ?`), now, changesetID.String())
	return err
}

func (s *Store) DiscardOpenChangeset(ctx context.Context, projectID uuid.UUID) error {
	now := formatTime(s.driver, time.Now().UTC())
	res, err := s.db.ExecContext(ctx, s.q(`
		UPDATE changesets SET status = ?, updated_at = ? WHERE project_id = ? AND status = 'open'`),
		string(domain.ChangesetDiscarded), now, projectID.String(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return launchpad.ErrNotFound
	}
	return nil
}

func (s *Store) CommitChangeset(ctx context.Context, tx *sql.Tx, changesetID uuid.UUID) error {
	exec := s.exec(tx)
	now := formatTime(s.driver, time.Now().UTC())
	res, err := exec.ExecContext(ctx, s.q(`
		UPDATE changesets SET status = ?, updated_at = ? WHERE id = ? AND status = 'open'`),
		string(domain.ChangesetCommitted), now, changesetID.String(),
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

func (s *Store) listChangesetChanges(ctx context.Context, tx *sql.Tx, changesetID uuid.UUID) ([]domain.ChangesetChange, error) {
	var exec interface {
		QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	}
	if tx != nil {
		exec = tx
	} else {
		exec = s.db
	}
	rows, err := exec.QueryContext(ctx, s.q(`
		SELECT id, changeset_id, service_id, service_name, type, payload, created_at
		FROM changeset_changes WHERE changeset_id = ? ORDER BY created_at`), changesetID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var changes []domain.ChangesetChange
	for rows.Next() {
		c, err := scanChangesetChange(rows, s.driver)
		if err != nil {
			return nil, err
		}
		changes = append(changes, *c)
	}
	return changes, rows.Err()
}

func scanChangeset(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Changeset, error) {
	var id, projectID, status, description, createdAt, updatedAt string
	var environmentID sql.NullString
	if err := scanner.Scan(&id, &projectID, &environmentID, &status, &description, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	cs := &domain.Changeset{
		ID:          uuid.MustParse(id),
		ProjectID:   uuid.MustParse(projectID),
		Status:      domain.ChangesetStatus(status),
		Description: description,
		CreatedAt:   parseTime(driver, createdAt),
		UpdatedAt:   parseTime(driver, updatedAt),
	}
	if environmentID.Valid && environmentID.String != "" {
		eid := uuid.MustParse(environmentID.String)
		cs.EnvironmentID = &eid
	}
	return cs, nil
}

func scanChangesetChange(scanner interface{ Scan(...any) error }, driver Driver) (*domain.ChangesetChange, error) {
	var id, changesetID, serviceName, changeType, payload, createdAt string
	var serviceID sql.NullString
	if err := scanner.Scan(&id, &changesetID, &serviceID, &serviceName, &changeType, &payload, &createdAt); err != nil {
		return nil, err
	}
	change := &domain.ChangesetChange{
		ID:          uuid.MustParse(id),
		ChangesetID: uuid.MustParse(changesetID),
		ServiceName: serviceName,
		Type:        domain.ChangeType(changeType),
		Payload:     json.RawMessage(payload),
		CreatedAt:   parseTime(driver, createdAt),
	}
	if serviceID.Valid {
		sid := uuid.MustParse(serviceID.String)
		change.ServiceID = &sid
	}
	return change, nil
}
