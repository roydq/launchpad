package domain

import (
	"time"

	"github.com/google/uuid"
)

type Service struct {
	ID        uuid.UUID
	ProjectID uuid.UUID
	Name      string
	Kind      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ConfigVar struct {
	ServiceID     uuid.UUID
	EnvironmentID uuid.UUID
	Key           string
	Value         string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}