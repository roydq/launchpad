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
	_, err = s.db.ExecContext(ctx, s.q(`
		INSERT INTO api_tokens (id, team_id, name, token_hash, scopes, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		token.ID.String(), token.TeamID.String(), token.Name, token.TokenHash, string(scopes), expiresAt,
		formatTime(s.driver, token.CreatedAt),
	)
	return err
}

func (s *Store) GetTokenByHash(ctx context.Context, hash []byte) (*domain.APIToken, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, team_id, name, token_hash, scopes, expires_at, revoked_at, created_at
		FROM api_tokens WHERE token_hash = ? AND revoked_at IS NULL`), hash)
	return scanToken(row, s.driver)
}

func (s *Store) GetTeamByName(ctx context.Context, name string) (*domain.Team, error) {
	row := s.db.QueryRowContext(ctx, s.q(`SELECT id, name, created_at FROM teams WHERE name = ?`), name)
	var id, teamName, createdAt string
	if err := row.Scan(&id, &teamName, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	return &domain.Team{
		ID:        uuid.MustParse(id),
		Name:      teamName,
		CreatedAt: parseTime(s.driver, createdAt),
	}, nil
}

func scanToken(row *sql.Row, driver Driver) (*domain.APIToken, error) {
	var id, teamID, name, scopes string
	var tokenHash []byte
	var expiresAt, revokedAt, createdAt sql.NullString
	if err := row.Scan(&id, &teamID, &name, &tokenHash, &scopes, &expiresAt, &revokedAt, &createdAt); err != nil {
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
		ID:        uuid.MustParse(id),
		TeamID:    uuid.MustParse(teamID),
		Name:      name,
		TokenHash: tokenHash,
		Scopes:    parsedScopes,
		CreatedAt: parseTime(driver, createdAt.String),
	}
	if expiresAt.Valid {
		t := parseTime(driver, expiresAt.String)
		token.ExpiresAt = &t
	}
	if revokedAt.Valid {
		t := parseTime(driver, revokedAt.String)
		token.RevokedAt = &t
	}
	return token, nil
}