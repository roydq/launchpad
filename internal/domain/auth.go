package domain

import (
	"time"

	"github.com/google/uuid"
)

type APIToken struct {
	ID          uuid.UUID
	WorkspaceID uuid.UUID
	Name        string
	TokenHash   []byte
	Scopes      []string
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
}