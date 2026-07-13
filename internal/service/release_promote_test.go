package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

func setupPromoteFixture(t *testing.T) (context.Context, *store.Store, *ReleaseService, *ProjectService, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	db, driver, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(ctx, db, driver); err != nil {
		t.Fatal(err)
	}
	st := store.New(db, driver)
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	return ctx, st, releaseSvc, projectSvc, workspaceID
}

func markDeploySucceeded(t *testing.T, ctx context.Context, st *store.Store, dep domain.Deployment) {
	t.Helper()
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateDeploymentStatus(ctx, tx, dep.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go")
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateDeploymentStatus(ctx, tx, dep.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateReleaseStatus(ctx, nil, dep.ReleaseID, domain.ReleaseStatusSucceeded); err != nil {
		t.Fatal(err)
	}
}

func TestPromoteReResolvesTargetConfig(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)

	project := &domain.Project{WorkspaceID: workspaceID, Name: "promo-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	// Create staging + production
	staging, err := projectSvc.CreateEnvironment(ctx, "promo-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prod, err := projectSvc.CreateEnvironment(ctx, "promo-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}

	// Distinct layers: staging vs production
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		if err := st.MergeSharedConfigVarsTx(ctx, tx, project.ID, staging.ID, map[string]*string{
			"LOG_LEVEL": strPtr("debug"),
		}); err != nil {
			return err
		}
		if err := st.MergeConfigVarsTx(ctx, tx, svc.ID, staging.ID, map[string]*string{
			"PORT": strPtr("3000"),
		}); err != nil {
			return err
		}
		if err := st.MergeSharedConfigVarsTx(ctx, tx, project.ID, prod.ID, map[string]*string{
			"LOG_LEVEL": strPtr("info"),
		}); err != nil {
			return err
		}
		return st.MergeConfigVarsTx(ctx, tx, svc.ID, prod.ID, map[string]*string{
			"PORT": strPtr("8080"),
		})
	}); err != nil {
		t.Fatal(err)
	}

	src, err := releaseSvc.CreateRelease(ctx, "promo-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app@sha:abc"}, Description: "staging ship",
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, src.Deployment)

	if src.Release.ConfigResolved["LOG_LEVEL"] != "debug" || src.Release.ConfigResolved["PORT"] != "3000" {
		t.Fatalf("source config unexpected: %+v", src.Release.ConfigResolved)
	}

	promoted, err := releaseSvc.Promote(ctx, "promo-app", "staging", "production", 1, "")
	if err != nil {
		t.Fatal(err)
	}

	if promoted.Release.Version != 2 {
		t.Fatalf("version: got %d want 2", promoted.Release.Version)
	}
	if promoted.Release.ArtifactRef != "app@sha:abc" {
		t.Fatalf("artifact: %s", promoted.Release.ArtifactRef)
	}
	if promoted.Release.ConfigResolved["LOG_LEVEL"] != "info" || promoted.Release.ConfigResolved["PORT"] != "8080" {
		t.Fatalf("target config not re-resolved: %+v", promoted.Release.ConfigResolved)
	}
	if promoted.Release.ConfigResolved["LOG_LEVEL"] == src.Release.ConfigResolved["LOG_LEVEL"] {
		t.Fatal("config_resolved must not be copied from source")
	}
	wantResolved, err := st.ResolveConfig(ctx, project.ID, svc.ID, prod.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Release.ConfigResolved["LOG_LEVEL"] != wantResolved["LOG_LEVEL"] ||
		promoted.Release.ConfigResolved["PORT"] != wantResolved["PORT"] {
		t.Fatalf("config_resolved != ResolveConfig(target): got %+v want %+v",
			promoted.Release.ConfigResolved, wantResolved)
	}
	if promoted.Deployment.EnvironmentID != prod.ID {
		t.Fatalf("deployment env: got %s want production %s", promoted.Deployment.EnvironmentID, prod.ID)
	}
	if promoted.Release.Description != "Promote v1 from staging to production" {
		t.Fatalf("desc: %s", promoted.Release.Description)
	}
	// Process snapshot matches source
	if len(promoted.Release.ProcessSnapshot) != len(src.Release.ProcessSnapshot) {
		t.Fatalf("snapshot len: got %d want %d", len(promoted.Release.ProcessSnapshot), len(src.Release.ProcessSnapshot))
	}
	for name, snap := range src.Release.ProcessSnapshot {
		got, ok := promoted.Release.ProcessSnapshot[name]
		if !ok || got != snap {
			t.Fatalf("snapshot %s: got %+v want %+v", name, got, snap)
		}
	}
}

func TestPromoteCopiesProcessSnapshotNotLiveTable(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)

	project := &domain.Project{WorkspaceID: workspaceID, Name: "snap-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "snap-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "snap-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}

	src, err := releaseSvc.CreateRelease(ctx, "snap-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:1"}, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, src.Deployment)

	origQty := src.Release.ProcessSnapshot["web"].Quantity
	// Mutate live process table after source release
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateProcessQuantity(ctx, tx, svc.ID, "web", origQty+5)
	}); err != nil {
		t.Fatal(err)
	}

	promoted, err := releaseSvc.Promote(ctx, "snap-app", "staging", "production", 1, "ship")
	if err != nil {
		t.Fatal(err)
	}
	got := promoted.Release.ProcessSnapshot["web"].Quantity
	if got != origQty {
		t.Fatalf("process snapshot must come from source release, got qty %d want %d", got, origQty)
	}
}

func TestPromoteDefaultVersionUsesRunningInFrom(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)

	project := &domain.Project{WorkspaceID: workspaceID, Name: "def-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "def-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "def-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}

	src, err := releaseSvc.CreateRelease(ctx, "def-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:run"}, Description: "run",
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, src.Deployment)

	promoted, err := releaseSvc.Promote(ctx, "def-app", "staging", "production", 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Release.ArtifactRef != "app:run" {
		t.Fatalf("artifact: %s", promoted.Release.ArtifactRef)
	}
}

func TestPromoteRejectsSameEnv(t *testing.T) {
	ctx, st, releaseSvc, _, workspaceID := setupPromoteFixture(t)
	project := &domain.Project{WorkspaceID: workspaceID, Name: "same-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	_, err := releaseSvc.Promote(ctx, "same-app", "dev", "dev", 1, "")
	if !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
}

func TestPromoteRejectsFailedSource(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)
	project := &domain.Project{WorkspaceID: workspaceID, Name: "fail-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "fail-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "fail-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}

	src, err := releaseSvc.CreateRelease(ctx, "fail-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:bad"}, Description: "bad",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Mark deploy failed path and release failed
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		if err := st.UpdateDeploymentStatus(ctx, tx, src.Deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go"); err != nil {
			return err
		}
		return st.UpdateDeploymentStatus(ctx, tx, src.Deployment.ID, domain.DeploymentDeploying, domain.DeploymentFailed, "boom")
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateReleaseStatus(ctx, nil, src.Release.ID, domain.ReleaseStatusFailed); err != nil {
		t.Fatal(err)
	}

	_, err = releaseSvc.Promote(ctx, "fail-app", "staging", "production", 1, "")
	if !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
}

func TestPromoteRejectsNeverDeployedToFrom(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)
	project := &domain.Project{WorkspaceID: workspaceID, Name: "never-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "never-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "never-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}

	// Deploy only to dev, then try promote from staging
	src, err := releaseSvc.CreateRelease(ctx, "never-app", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:x"}, Description: "dev only",
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, src.Deployment)

	_, err = releaseSvc.Promote(ctx, "never-app", "staging", "production", 1, "")
	if !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("want ErrBadRequest, got %v", err)
	}
}

func TestPromoteExplicitVersionAllowsSuperseded(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)
	project := &domain.Project{WorkspaceID: workspaceID, Name: "super-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "super-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "super-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}

	v1, err := releaseSvc.CreateRelease(ctx, "super-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"}, Description: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, v1.Deployment)

	v2, err := releaseSvc.CreateRelease(ctx, "super-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v2"}, Description: "v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		if err := st.UpdateDeploymentStatus(ctx, tx, v2.Deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go"); err != nil {
			return err
		}
		return st.SupersedeRunningDeployments(ctx, tx, v2.Deployment.ServiceID, v2.Deployment.EnvironmentID, v2.Deployment.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateDeploymentStatus(ctx, tx, v2.Deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateReleaseStatus(ctx, nil, v2.Release.ID, domain.ReleaseStatusSucceeded); err != nil {
		t.Fatal(err)
	}

	// Promote superseded v1 explicitly
	promoted, err := releaseSvc.Promote(ctx, "super-app", "staging", "production", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if promoted.Release.ArtifactRef != "app:v1" {
		t.Fatalf("want app:v1, got %s", promoted.Release.ArtifactRef)
	}
}

func TestPromoteRejectsActiveDeployOnTarget(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)
	project := &domain.Project{WorkspaceID: workspaceID, Name: "active-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "active-app", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "active-app", CreateEnvironmentInput{
		Name: "production", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}

	src, err := releaseSvc.CreateRelease(ctx, "active-app", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:s"}, Description: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, src.Deployment)

	// Leave an active deploy on production
	active, err := releaseSvc.CreateRelease(ctx, "active-app", "production", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:p"}, Description: "p",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = active

	_, err = releaseSvc.Promote(ctx, "active-app", "staging", "production", 1, "")
	if !errors.Is(err, launchpad.ErrConflict) {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}
