package domain

import (
	"time"

	"github.com/google/uuid"
)

type Build struct {
	ID         uuid.UUID
	AppID      uuid.UUID
	SourceType string
	SourceRef  string
	ImageRef   string
	Status     JobStatus
	LogsURL    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Release struct {
	ID             uuid.UUID
	AppID          uuid.UUID
	BuildID        *uuid.UUID
	Version        int
	ConfigSnapshot map[string]string
	ImageRef       string
	Status         ReleaseStatus
	Description    string
	CreatedAt      time.Time
}

type Deployment struct {
	ID         uuid.UUID
	AppID      uuid.UUID
	ReleaseID  uuid.UUID
	Status     DeploymentStatus
	Version    int
	TargetRef  string
	Message    string
	StartedAt  time.Time
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DeploymentEvent struct {
	ID           uuid.UUID
	DeploymentID uuid.UUID
	Type         string
	Message      string
	Metadata     map[string]any
	CreatedAt    time.Time
}