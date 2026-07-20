package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) CreateProcess(ctx context.Context, process *domain.Process) error {
	return s.createProcessTx(ctx, nil, process)
}

func (s *Store) createProcessTx(ctx context.Context, tx *sql.Tx, process *domain.Process) error {
	if process.ID == uuid.Nil {
		process.ID = uuid.New()
	}
	now := time.Now().UTC()
	process.CreatedAt = now
	process.UpdatedAt = now
	if process.Expose == "" {
		process.Expose = "none"
	}
	_, err := s.exec(tx).ExecContext(ctx, s.q(`
		INSERT INTO processes (id, service_id, name, command, quantity, expose, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		process.ID.String(), process.ServiceID.String(), process.Name, process.Command,
		process.Quantity, process.Expose,
		formatTime(s.driver, process.CreatedAt), formatTime(s.driver, process.UpdatedAt),
	)
	return err
}

func (s *Store) ListProcesses(ctx context.Context, serviceID uuid.UUID) ([]domain.Process, error) {
	return s.ListProcessesTx(ctx, nil, serviceID)
}

func (s *Store) ListProcessesTx(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID) ([]domain.Process, error) {
	rows, err := s.exec(tx).QueryContext(ctx, s.q(`
		SELECT id, service_id, name, command, quantity, expose, created_at, updated_at
		FROM processes WHERE service_id = ? ORDER BY name`), serviceID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var processes []domain.Process
	for rows.Next() {
		p, err := scanProcess(rows, s.driver)
		if err != nil {
			return nil, err
		}
		processes = append(processes, *p)
	}
	return processes, rows.Err()
}

func (s *Store) GetProcess(ctx context.Context, serviceID uuid.UUID, name string) (*domain.Process, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, service_id, name, command, quantity, expose, created_at, updated_at
		FROM processes WHERE service_id = ? AND name = ?`), serviceID.String(), name)
	return scanProcess(row, s.driver)
}

func (s *Store) UpdateProcessQuantity(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID, name string, quantity int) error {
	exec := s.exec(tx)
	res, err := exec.ExecContext(ctx, s.q(`
		UPDATE processes SET quantity = ?, updated_at = ? WHERE service_id = ? AND name = ?`),
		quantity, formatTime(s.driver, time.Now().UTC()), serviceID.String(), name,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return launchpad.ErrNotFound
	}
	return nil
}

// UpsertProcessTx inserts or updates a process definition by (service_id, name).
func (s *Store) UpsertProcessTx(ctx context.Context, tx *sql.Tx, process *domain.Process) error {
	if process.Name == "" {
		return fmt.Errorf("%w: process name required", launchpad.ErrBadRequest)
	}
	if process.Expose == "" {
		process.Expose = "none"
	}
	now := formatTime(s.driver, time.Now().UTC())
	existing, err := s.getProcessTx(ctx, tx, process.ServiceID, process.Name)
	if err != nil && !errors.Is(err, launchpad.ErrNotFound) {
		return err
	}
	if existing == nil {
		if process.ID == uuid.Nil {
			process.ID = uuid.New()
		}
		process.CreatedAt = time.Now().UTC()
		process.UpdatedAt = process.CreatedAt
		_, err := s.exec(tx).ExecContext(ctx, s.q(`
			INSERT INTO processes (id, service_id, name, command, quantity, expose, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
			process.ID.String(), process.ServiceID.String(), process.Name, process.Command,
			process.Quantity, process.Expose, now, now,
		)
		return err
	}
	process.ID = existing.ID
	process.CreatedAt = existing.CreatedAt
	process.UpdatedAt = time.Now().UTC()
	_, err = s.exec(tx).ExecContext(ctx, s.q(`
		UPDATE processes SET command = ?, quantity = ?, expose = ?, updated_at = ?
		WHERE service_id = ? AND name = ?`),
		process.Command, process.Quantity, process.Expose, now,
		process.ServiceID.String(), process.Name,
	)
	return err
}

// DeleteProcessTx removes a process by name.
func (s *Store) DeleteProcessTx(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID, name string) error {
	res, err := s.exec(tx).ExecContext(ctx, s.q(`
		DELETE FROM processes WHERE service_id = ? AND name = ?`),
		serviceID.String(), name,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return launchpad.ErrNotFound
	}
	return nil
}

func (s *Store) getProcessTx(ctx context.Context, tx *sql.Tx, serviceID uuid.UUID, name string) (*domain.Process, error) {
	row := s.exec(tx).QueryRowContext(ctx, s.q(`
		SELECT id, service_id, name, command, quantity, expose, created_at, updated_at
		FROM processes WHERE service_id = ? AND name = ?`), serviceID.String(), name)
	return scanProcess(row, s.driver)
}

func scanProcess(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Process, error) {
	var id, serviceID, name, command, expose, createdAt, updatedAt string
	var quantity int
	if err := scanner.Scan(&id, &serviceID, &name, &command, &quantity, &expose, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	return &domain.Process{
		ID:        uuid.MustParse(id),
		ServiceID: uuid.MustParse(serviceID),
		Name:      name,
		Command:   command,
		Quantity:  quantity,
		Expose:    expose,
		CreatedAt: parseTime(driver, createdAt),
		UpdatedAt: parseTime(driver, updatedAt),
	}, nil
}