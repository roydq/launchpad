package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func (s *Store) CreatePrincipal(ctx context.Context, tx *sql.Tx, p *domain.Principal) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	if p.Status == "" {
		p.Status = domain.PrincipalStatusActive
	}
	p.CreatedAt = time.Now().UTC()
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`
		INSERT INTO principals (id, kind, display_name, email, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`),
		p.ID.String(), string(p.Kind), p.DisplayName, p.Email, string(p.Status),
		formatTime(s.driver, p.CreatedAt),
	)
	return err
}

func (s *Store) GetPrincipal(ctx context.Context, id uuid.UUID) (*domain.Principal, error) {
	row := s.db.QueryRowContext(ctx, s.q(`
		SELECT id, kind, display_name, email, status, created_at FROM principals WHERE id = ?`), id.String())
	return scanPrincipal(row, s.driver)
}

func (s *Store) AddWorkspaceMember(ctx context.Context, tx *sql.Tx, m *domain.WorkspaceMember) error {
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	exec := s.exec(tx)
	_, err := exec.ExecContext(ctx, s.q(`
		INSERT INTO workspace_members (workspace_id, principal_id, role, created_at)
		VALUES (?, ?, ?, ?)`),
		m.WorkspaceID.String(), m.PrincipalID.String(), string(m.Role),
		formatTime(s.driver, m.CreatedAt),
	)
	return err
}

func (s *Store) CreateAuditEvent(ctx context.Context, tx *sql.Tx, ev *domain.AuditEvent) error {
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}
	if ev.CreatedAt.IsZero() {
		ev.CreatedAt = time.Now().UTC()
	}
	if ev.Detail == nil {
		ev.Detail = map[string]string{}
	}
	detail, err := json.Marshal(ev.Detail)
	if err != nil {
		return err
	}
	var principalID, tokenID any
	if ev.PrincipalID != nil {
		principalID = ev.PrincipalID.String()
	}
	if ev.TokenID != nil {
		tokenID = ev.TokenID.String()
	}
	exec := s.exec(tx)
	_, err = exec.ExecContext(ctx, s.q(`
		INSERT INTO audit_events (id, workspace_id, principal_id, token_id, action, resource_type, resource_id, project_name, detail, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		ev.ID.String(), ev.WorkspaceID.String(), principalID, tokenID,
		string(ev.Action), ev.ResourceType, ev.ResourceID.String(), ev.ProjectName,
		string(detail), formatTime(s.driver, ev.CreatedAt),
	)
	return err
}

func (s *Store) ListAuditEvents(ctx context.Context, workspaceID uuid.UUID, limit int) ([]domain.AuditEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, s.q(`
		SELECT id, workspace_id, principal_id, token_id, action, resource_type, resource_id, project_name, detail, created_at
		FROM audit_events WHERE workspace_id = ?
		ORDER BY created_at DESC LIMIT ?`), workspaceID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.AuditEvent
	for rows.Next() {
		ev, err := scanAuditEvent(rows, s.driver)
		if err != nil {
			return nil, err
		}
		out = append(out, *ev)
	}
	return out, rows.Err()
}

func scanPrincipal(scanner interface{ Scan(...any) error }, driver Driver) (*domain.Principal, error) {
	var id, kind, displayName, email, status, createdAt string
	if err := scanner.Scan(&id, &kind, &displayName, &email, &status, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	return &domain.Principal{
		ID:          uuid.MustParse(id),
		Kind:        domain.PrincipalKind(kind),
		DisplayName: displayName,
		Email:       email,
		Status:      domain.PrincipalStatus(status),
		CreatedAt:   parseTime(driver, createdAt),
	}, nil
}

func scanAuditEvent(scanner interface{ Scan(...any) error }, driver Driver) (*domain.AuditEvent, error) {
	var id, workspaceID, action, resourceType, resourceID, projectName, detail, createdAt string
	var principalID, tokenID sql.NullString
	if err := scanner.Scan(&id, &workspaceID, &principalID, &tokenID, &action, &resourceType, &resourceID, &projectName, &detail, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, launchpad.ErrNotFound
		}
		return nil, err
	}
	ev := &domain.AuditEvent{
		ID:           uuid.MustParse(id),
		WorkspaceID:  uuid.MustParse(workspaceID),
		Action:       domain.AuditAction(action),
		ResourceType: resourceType,
		ResourceID:   uuid.MustParse(resourceID),
		ProjectName:  projectName,
		CreatedAt:    parseTime(driver, createdAt),
	}
	if principalID.Valid && principalID.String != "" {
		pid := uuid.MustParse(principalID.String)
		ev.PrincipalID = &pid
	}
	if tokenID.Valid && tokenID.String != "" {
		tid := uuid.MustParse(tokenID.String)
		ev.TokenID = &tid
	}
	_ = json.Unmarshal([]byte(detail), &ev.Detail)
	if ev.Detail == nil {
		ev.Detail = map[string]string{}
	}
	return ev, nil
}
