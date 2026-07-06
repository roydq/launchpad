package domain

import (
	"time"

	"github.com/google/uuid"
)

type Workspace struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}