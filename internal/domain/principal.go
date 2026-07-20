package domain

import (
	"time"

	"github.com/google/uuid"
)

// PrincipalKind distinguishes humans from automation identities.
type PrincipalKind string

const (
	PrincipalKindUser           PrincipalKind = "user"
	PrincipalKindServiceAccount PrincipalKind = "service_account"
)

// PrincipalStatus is lifecycle state for a principal.
type PrincipalStatus string

const (
	PrincipalStatusActive   PrincipalStatus = "active"
	PrincipalStatusDisabled PrincipalStatus = "disabled"
)

// WorkspaceRole is membership power (scopes still authorize API in phase 1).
type WorkspaceRole string

const (
	WorkspaceRoleOwner    WorkspaceRole = "owner"
	WorkspaceRoleAdmin    WorkspaceRole = "admin"
	WorkspaceRoleOperator WorkspaceRole = "operator"
	WorkspaceRoleViewer   WorkspaceRole = "viewer"
)

// Principal is who acts in the control plane.
type Principal struct {
	ID          uuid.UUID
	Kind        PrincipalKind
	DisplayName string
	Email       string
	Status      PrincipalStatus
	CreatedAt   time.Time
}

// WorkspaceMember binds a principal to a workspace with a role.
type WorkspaceMember struct {
	WorkspaceID uuid.UUID
	PrincipalID uuid.UUID
	Role        WorkspaceRole
	CreatedAt   time.Time
}

// AuditAction names a recorded control-plane mutation.
type AuditAction string

const (
	AuditActionReleaseCreate   AuditAction = "release.create"
	AuditActionReleasePromote  AuditAction = "release.promote"
	AuditActionReleaseRollback AuditAction = "release.rollback"
	AuditActionChangesetPush   AuditAction = "changeset.push"
	AuditActionConfigSet       AuditAction = "config.set"
)

// AuditEvent is an append-only record of who did what.
type AuditEvent struct {
	ID           uuid.UUID
	WorkspaceID  uuid.UUID
	PrincipalID  *uuid.UUID
	TokenID      *uuid.UUID
	Action       AuditAction
	ResourceType string
	ResourceID   uuid.UUID
	ProjectName  string
	Detail       map[string]string
	CreatedAt    time.Time
}
