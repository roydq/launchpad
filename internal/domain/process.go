package domain

import (
	"encoding/json"
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
	// Health is optional portable readiness (nil / type none = no probe).
	Health *ProcessHealth
	// TargetExtensions is namespaced backend knobs, frozen on release.
	TargetExtensions map[string]json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ProcessHealth is portable readiness configuration (maps to target probes).
type ProcessHealth struct {
	Type                string `json:"type"` // http | tcp | exec | none
	Path                string `json:"path,omitempty"`
	Port                *int   `json:"port,omitempty"`
	InitialDelaySeconds int    `json:"initial_delay_seconds,omitempty"`
	PeriodSeconds       int    `json:"period_seconds,omitempty"`
	TimeoutSeconds      int    `json:"timeout_seconds,omitempty"`
	FailureThreshold    int    `json:"failure_threshold,omitempty"`
	SuccessThreshold    int    `json:"success_threshold,omitempty"`
}

// ProcessSnapshot is the deployable process topology frozen on a release.
// Empty Command means use the image entrypoint/CMD.
type ProcessSnapshot struct {
	Command          string                     `json:"command"`
	Quantity         int                        `json:"quantity"`
	Expose           string                     `json:"expose"`
	Health           *ProcessHealth             `json:"health,omitempty"`
	TargetExtensions map[string]json.RawMessage `json:"target_extensions,omitempty"`
}

// ProcessFromSnapshot rebuilds a Process suitable for Target.Deploy from a release snapshot entry.
func ProcessFromSnapshot(serviceID uuid.UUID, name string, snap ProcessSnapshot) Process {
	return Process{
		ServiceID:        serviceID,
		Name:             name,
		Command:          snap.Command,
		Quantity:         snap.Quantity,
		Expose:           snap.Expose,
		Health:           snap.Health,
		TargetExtensions: snap.TargetExtensions,
	}
}