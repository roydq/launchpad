package store

import (
	"context"
	"database/sql"
)

type Store struct {
	db     *sql.DB
	driver Driver
}

func New(db *sql.DB, driver Driver) *Store {
	return &Store{db: db, driver: driver}
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Driver() Driver {
	return s.driver
}

func (s *Store) Transact(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}