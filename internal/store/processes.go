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

func (s *Store) UpdateProcessQuantity(ctx context.Context, tx *sql.Tx, appID uuid.UUID, name string, quantity int) error {
	exec := s.exec(tx)
	res, err := exec.ExecContext(ctx, s.q(`
		UPDATE process_types SET quantity = ?, updated_at = ? WHERE app_id = ? AND name = ?`),
		quantity, formatTime(s.driver, time.Now().UTC()), appID.String(), name,
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

func (s *Store) GetProcessType(ctx context.Context, appID uuid.UUID, name string) (*domain.ProcessType, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, app_id, name, command, quantity, created_at, updated_at
		FROM process_types WHERE app_id = ? AND name = ?`), appID.String(), name)
	var pt domain.ProcessType
	var id, aid, createdAt, updatedAt string
	if err := row.Scan(&id, &aid, &pt.Name, &pt.Command, &pt.Quantity, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	pt.ID = uuid.MustParse(id)
	pt.AppID = uuid.MustParse(aid)
	pt.CreatedAt = parseTime(s.driver, createdAt)
	pt.UpdatedAt = parseTime(s.driver, updatedAt)
	return &pt, nil
}