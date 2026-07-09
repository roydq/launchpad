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

// ProcessSnapshot is the deployable process topology frozen on a release.
// Empty Command means use the image entrypoint/CMD.
type ProcessSnapshot struct {
	Command  string `json:"command"`
	Quantity int    `json:"quantity"`
	Expose   string `json:"expose"`
}

// ProcessFromSnapshot rebuilds a Process suitable for Target.Deploy from a release snapshot entry.
func ProcessFromSnapshot(serviceID uuid.UUID, name string, snap ProcessSnapshot) Process {
	return Process{
		ServiceID: serviceID,
		Name:      name,
		Command:   snap.Command,
		Quantity:  snap.Quantity,
		Expose:    snap.Expose,
	}
}