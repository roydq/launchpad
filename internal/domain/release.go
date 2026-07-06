package domain

import (
	"time"

	"github.com/google/uuid"
)

type Release struct {
	ID              uuid.UUID
	ServiceID       uuid.UUID
	Version         int
	ArtifactRef     string
	ConfigResolved  map[string]string
	ProcessSnapshot map[string]ProcessSnapshot
	Status          ReleaseStatus
	Description     string
	CreatedAt       time.Time
}

type Deployment struct {
	ID            uuid.UUID
	ServiceID     uuid.UUID
	EnvironmentID uuid.UUID
	ReleaseID     uuid.UUID
	Status        DeploymentStatus
	Version       int
	TargetRef     string
	Message       string
	StartedAt     time.Time
	FinishedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}