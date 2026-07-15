package service

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
)

func TestCreateTokenLinksServiceAccountAndReleaseAttribution(t *testing.T) {
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver)
	authSvc := auth.NewService(st, "boot-secret")
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)

	plaintext, token, principal, err := authSvc.CreateToken(ctx, "default", "ci-bot", []string{"admin", "deploy", "project:write", "project:read"})
	if err != nil {
		t.Fatal(err)
	}
	if plaintext == "" || token == nil || principal == nil {
		t.Fatal("expected token + principal")
	}
	if principal.Kind != domain.PrincipalKindServiceAccount {
		t.Fatalf("kind: %s", principal.Kind)
	}
	if token.PrincipalID == nil || *token.PrincipalID != principal.ID {
		t.Fatalf("token principal link: %+v", token.PrincipalID)
	}

	info, err := authSvc.Authenticate(ctx, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if info.TokenID == nil || info.PrincipalID == nil {
		t.Fatal("auth info missing token/principal")
	}

	// Act as the minted token.
	actx := context.WithValue(ctx, auth.ContextTeamID, info.WorkspaceID)
	actx = context.WithValue(actx, auth.ContextTokenID, *info.TokenID)
	actx = context.WithValue(actx, auth.ContextPrincipalID, *info.PrincipalID)

	project := &domain.Project{WorkspaceID: info.WorkspaceID, Name: "id-app"}
	if err := st.CreateProject(actx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}

	res, err := releaseSvc.CreateRelease(actx, "id-app", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:1"}, Description: "attributed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Release.CreatedByPrincipalID == nil || *res.Release.CreatedByPrincipalID != principal.ID {
		t.Fatalf("release principal: %+v", res.Release.CreatedByPrincipalID)
	}
	if res.Release.CreatedByTokenID == nil || *res.Release.CreatedByTokenID != token.ID {
		t.Fatalf("release token: %+v", res.Release.CreatedByTokenID)
	}

	// Persist + re-read
	got, err := st.GetReleaseByID(actx, res.Release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedByPrincipalID == nil || *got.CreatedByPrincipalID != principal.ID {
		t.Fatalf("stored principal: %+v", got.CreatedByPrincipalID)
	}

	events, err := releaseSvc.ListAuditEvents(actx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected audit event")
	}
	found := false
	for _, ev := range events {
		if ev.Action == domain.AuditActionReleaseCreate && ev.ResourceID == res.Release.ID {
			found = true
			if ev.PrincipalID == nil || *ev.PrincipalID != principal.ID {
				t.Fatalf("audit principal: %+v", ev.PrincipalID)
			}
		}
	}
	if !found {
		t.Fatalf("audit missing release.create: %+v", events)
	}

	cb := releaseSvc.ResolveCreatedBy(actx, res.Release)
	if cb == nil || cb.DisplayName != "ci-bot" || cb.Kind != string(domain.PrincipalKindServiceAccount) {
		t.Fatalf("created_by: %+v", cb)
	}

	// Bootstrap path still works without principal.
	bctx := context.WithValue(ctx, auth.ContextTeamID, info.WorkspaceID)
	// Mark first deploy terminal so second can enqueue
	_ = st.Transact(actx, func(tx *sql.Tx) error {
		if err := st.UpdateDeploymentStatus(actx, tx, res.Deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go"); err != nil {
			return err
		}
		return st.UpdateDeploymentStatus(actx, tx, res.Deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	})
	_ = st.UpdateReleaseStatus(actx, nil, res.Release.ID, domain.ReleaseStatusSucceeded)

	res2, err := releaseSvc.CreateRelease(bctx, "id-app", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Release.CreatedByPrincipalID != nil {
		t.Fatalf("bootstrap should leave principal nil, got %v", res2.Release.CreatedByPrincipalID)
	}
}

func TestBootstrapAuthenticateHasNoPrincipal(t *testing.T) {
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver)
	authSvc := auth.NewService(st, "boot")
	info, err := authSvc.Authenticate(ctx, "boot")
	if err != nil {
		t.Fatal(err)
	}
	if info.PrincipalID != nil || info.TokenID != nil {
		t.Fatalf("bootstrap should not set principal/token: %+v", info)
	}
	if info.WorkspaceID == uuid.Nil {
		t.Fatal("workspace required")
	}
}
