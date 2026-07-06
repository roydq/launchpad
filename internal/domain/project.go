package domain

import (
	"time"

	"github.com/google/uuid"
)

type Project struct {
	ID             uuid.UUID
	WorkspaceID    uuid.UUID
	Name           string
	PrimaryService string
	Status         ProjectStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time
}