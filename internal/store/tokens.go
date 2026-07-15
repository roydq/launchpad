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

func (s *Store) CreateToken(ctx context.Context, token *domain.APIToken) error {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	token.CreatedAt = time.Now().UTC()
	scopes, err := json.Marshal(token.Scopes)
	if err != nil {
		return err
	}
	var expiresAt any
	if token.ExpiresAt != nil {
		expiresAt = formatTime(s.driver, *token.ExpiresAt)
	}
	var principalID any
	if token.PrincipalID != nil {
		principalID = token.PrincipalID.String()
	}
	_, err = s.db.ExecContext(ctx, s.q(`
		INSERT INTO api_tokens (id, workspace_id, name, token_hash, scopes, expires_at, created_at, principal_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		token.ID.String(), token.WorkspaceID.String(), token.Name, token.TokenHash, string(scopes), expiresAt,
		formatTime(s.driver, token.CreatedAt), principalID,
	)
	return err
}

// CreateTokenTx inserts a token inside an existing transaction.
func (s *Store) CreateTokenTx(ctx context.Context, tx *sql.Tx, token *domain.APIToken) error {
	if token.ID == uuid.Nil {
		token.ID = uuid.New()
	}
	token.CreatedAt = time.Now().UTC()
	scopes, err := json.Marshal(token.Scopes)
	if err != nil {
		return err
	}
	var expiresAt any
	if token.ExpiresAt != nil {
		expiresAt = formatTime(s.driver, *token.ExpiresAt)
	}
	var principalID any
	if token.PrincipalID != nil {
		principalID = token.PrincipalID.String()
	}
	exec := s.exec(tx)
	_, err = exec.ExecContext(ctx, s.q(`
		INSERT INTO api_tokens (id, workspace_id, name, token_hash, scopes, expires_at, created_at, principal_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		token.ID.String(), token.WorkspaceID.String(), token.Name, token.TokenHash, string(scopes), expiresAt,
		formatTime(s.driver, token.CreatedAt), principalID,
	)
	return err
}

func (s *Store) GetTokenByHash(ctx context.Context, hash []byte) (*domain.APIToken, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, workspace_id, name, token_hash, scopes, expires_at, revoked_at, created_at, principal_id
		FROM api_tokens WHERE token_hash = ? AND revoked_at IS NULL`), hash)
	return scanToken(row, s.driver)
}

func scanToken(row *sql.Row, driver Driver) (*domain.APIToken, error) {
	var id, workspaceID, name, scopes string
	var tokenHash []byte
	var expiresAt, revokedAt, createdAt sql.NullString
	var principalID sql.NullString
	if err := row.Scan(&id, &workspaceID, &name, &tokenHash, &scopes, &expiresAt, &revokedAt, &createdAt, &principalID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	var parsedScopes []string
	if err := json.Unmarshal([]byte(scopes), &parsedScopes); err != nil {
		return nil, err
	}
	token := &domain.APIToken{
		ID:          uuid.MustParse(id),
		WorkspaceID: uuid.MustParse(workspaceID),
		Name:        name,
		TokenHash:   tokenHash,
		Scopes:      parsedScopes,
		CreatedAt:   parseTime(driver, createdAt.String),
	}
	if expiresAt.Valid {
		t := parseTime(driver, expiresAt.String)
		token.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := parseTime(driver, revokedAt.String)
		token.RevokedAt = &t
	}
	if principalID.Valid && principalID.String != "" {
		pid := uuid.MustParse(principalID.String)
		token.PrincipalID = &pid
	}
	return token, nil
}
