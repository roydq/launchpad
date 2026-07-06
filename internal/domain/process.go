package domain

import (
	"time"

	"github.com/google/uuid"
)

type Process struct {
	ID        uuid.UUID
	ServiceID uuid.UUID
	Name      string
	Command   string
	Quantity  int
	Expose    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ProcessSnapshot struct {
	Quantity int `json:"quantity"`
}