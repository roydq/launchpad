package store

import (
	"context"
	"database/sql"

	"github.com/launchpad/launchpad/internal/secrets"
)

type Store struct {
	db      *sql.DB
	driver  Driver
	secrets *secrets.Box // optional; required to write/decrypt secret ciphertext
}

func New(db *sql.DB, driver Driver) *Store {
	return &Store{db: db, driver: driver}
}

// WithSecrets returns the store configured with an encryption box for secret config values.
// Call on process start when LAUNCHPAD_SECRETS_KEY is set. Nil box leaves plain-only mode
// (secret writes fail; legacy plaintext secrets still readable).
func (s *Store) WithSecrets(box *secrets.Box) *Store {
	s.secrets = box
	return s
}

func (s *Store) Secrets() *secrets.Box {
	return s.secrets
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