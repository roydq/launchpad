package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Environment struct {
	ID           uuid.UUID
	ProjectID    uuid.UUID
	Name         string
	TargetType   string
	TargetConfig json.RawMessage
	Ephemeral    bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}