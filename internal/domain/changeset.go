package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ChangesetStatus string

const (
	ChangesetOpen       ChangesetStatus = "open"
	ChangesetCommitted  ChangesetStatus = "committed"
	ChangesetDiscarded  ChangesetStatus = "discarded"
)

type ChangeType string

const (
	ChangeTypeConfig ChangeType = "config"
	ChangeTypeScale  ChangeType = "scale"
	ChangeTypeImage  ChangeType = "image"
)

type Changeset struct {
	ID          uuid.UUID
	AppID       uuid.UUID
	Status      ChangesetStatus
	Description string
	Changes     []ChangesetChange
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type ChangesetChange struct {
	ID          uuid.UUID
	ChangesetID uuid.UUID
	Type        ChangeType
	Payload     json.RawMessage
	CreatedAt   time.Time
}

type ConfigChangePayload struct {
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

type ScaleChangePayload struct {
	Process  string `json:"process"`
	Quantity int    `json:"quantity"`
}

type ImageChangePayload struct {
	Image string `json:"image"`
}