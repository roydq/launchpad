package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type JobType string

const (
	JobTypeDeploy  JobType = "deploy"
	JobTypeScale   JobType = "scale"
	JobTypeDestroy JobType = "destroy"
	JobTypeRollback JobType = "rollback"
)

type Job struct {
	ID           uuid.UUID
	Type         JobType
	ResourceType string
	ResourceID   uuid.UUID
	Status       JobStatus
	Payload      json.RawMessage
	Attempt      int
	MaxAttempts  int
	RunAt        time.Time
	LeasedUntil  *time.Time
	LeasedBy     string
	LastError    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DeployPayload struct {
	DeploymentID uuid.UUID `json:"deployment_id"`
	AppID        uuid.UUID `json:"app_id"`
	ReleaseID    uuid.UUID `json:"release_id"`
}

type ScalePayload struct {
	AppID       uuid.UUID `json:"app_id"`
	ProcessName string    `json:"process_name"`
	Quantity    int       `json:"quantity"`
}