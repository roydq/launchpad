package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Team struct {
	ID        uuid.UUID
	Name      string
	CreatedAt time.Time
}

type App struct {
	ID                 uuid.UUID
	TeamID             uuid.UUID
	Name               string
	Stack              string
	TargetType         string
	TargetConfig       json.RawMessage
	Status             AppStatus
	ActiveDeploymentID *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}

type ProcessType struct {
	ID        uuid.UUID
	AppID     uuid.UUID
	Name      string
	Command   string
	Quantity  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ConfigVar struct {
	AppID     uuid.UUID
	Key       string
	Value     string
	CreatedAt time.Time
	UpdatedAt time.Time
}