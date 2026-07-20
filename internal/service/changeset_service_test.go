package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
)

func TestUnstageLastChange(t *testing.T) {
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
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)
	project := &domain.Project{WorkspaceID: workspaceID, Name: "unstage-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	csSvc := NewChangesetService(st, projectSvc, releaseSvc)

	// Empty → not found
	if _, err := csSvc.UnstageLastChange(ctx, "unstage-app"); err == nil {
		t.Fatal("expected error when nothing staged")
	}

	v1, v2, v3 := "1", "2", "3"
	// Separate stage calls so created_at ordering is deterministic.
	for _, ch := range []StageChangeInput{
		{Type: "config", Key: "A", Value: &v1},
		{Type: "config", Key: "B", Value: &v2},
		{Type: "config", Key: "C", Value: &v3},
	} {
		if _, err := csSvc.StageChanges(ctx, "unstage-app", DefaultEnvironment, StageChangesInput{
			Changes: []StageChangeInput{ch},
		}); err != nil {
			t.Fatal(err)
		}
	}

	res, err := csSvc.UnstageLastChange(ctx, "unstage-app")
	if err != nil {
		t.Fatal(err)
	}
	if res.RemainingCount != 2 {
		t.Fatalf("remaining %d", res.RemainingCount)
	}
	var payload domain.ConfigChangePayload
	if err := json.Unmarshal(res.Change.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Key != "C" {
		t.Fatalf("expected last staged key C, got %q", payload.Key)
	}

	open, err := csSvc.GetChangeset(ctx, "unstage-app", DefaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.Changes) != 2 {
		t.Fatalf("open has %d changes", len(open.Changes))
	}

	// Unstage down to empty
	if _, err := csSvc.UnstageLastChange(ctx, "unstage-app"); err != nil {
		t.Fatal(err)
	}
	last, err := csSvc.UnstageLastChange(ctx, "unstage-app")
	if err != nil {
		t.Fatal(err)
	}
	if last.RemainingCount != 0 {
		t.Fatalf("remaining %d", last.RemainingCount)
	}
	if _, err := csSvc.UnstageLastChange(ctx, "unstage-app"); err == nil {
		t.Fatal("expected not found when empty")
	}
}

func TestMaterializeChanges(t *testing.T) {
	configPayload, _ := json.Marshal(domain.ConfigChangePayload{Key: "PORT", Value: strPtr("3000")})
	scalePayload, _ := json.Marshal(domain.ScaleChangePayload{Process: "web", Quantity: 3})
	imagePayload, _ := json.Marshal(domain.ImageChangePayload{ArtifactRef: "app:v2"})

	changes := []domain.ChangesetChange{
		{Type: domain.ChangeTypeConfig, Payload: configPayload},
		{Type: domain.ChangeTypeScale, Payload: scalePayload},
		{Type: domain.ChangeTypeImage, Payload: imagePayload},
	}

	shared, cfg, scales, image, err := materializeChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	if len(shared) != 0 {
		t.Fatalf("shared: %+v", shared)
	}
	if cfg["PORT"].Value == nil || *cfg["PORT"].Value != "3000" {
		t.Fatalf("config: %+v", cfg)
	}
	if scales["web"] != 3 {
		t.Fatalf("scales: %+v", scales)
	}
	if image != "app:v2" {
		t.Fatalf("image: %s", image)
	}
}

func TestChangesetStageAndPush(t *testing.T) {
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

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{
		WorkspaceID: workspaceID,
		Name:        "batch-app",
	}
	env := &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	if err := st.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}

	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	csSvc := NewChangesetService(st, projectSvc, releaseSvc)

	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	_, err = csSvc.StageChanges(ctx, "batch-app", DefaultEnvironment, StageChangesInput{Changes: []StageChangeInput{
		{Type: "config", Key: "FOO", Value: strPtr("bar")},
		{Type: "image", Image: "demo:v1"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	open, err := csSvc.GetChangeset(ctx, "batch-app", DefaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.Changes) != 2 {
		t.Fatalf("expected 2 staged changes, got %d", len(open.Changes))
	}

	result, err := csSvc.PushChangeset(ctx, "batch-app", DefaultEnvironment, PushChangesetInput{Description: "batch deploy"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Job.Type != domain.JobTypeDeploy {
		t.Fatalf("expected deploy job, got %s", result.Job.Type)
	}

	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, DefaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	vars, err := st.ListConfigVars(ctx, svc.ID, devEnv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "bar" {
		t.Fatalf("config not applied: %+v", vars)
	}
}

func TestPushAtomicOnActiveDeploy(t *testing.T) {
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

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "atomic-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	devEnv, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}

	// Block deploys with an existing active deployment.
	err = st.Transact(ctx, func(tx *sql.Tx) error {
		r := &domain.Release{
			ServiceID: svc.ID, Version: 1, ArtifactRef: "block:v1",
			ConfigResolved:  map[string]string{},
			ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1, Expose: "http"}},
		}
		if err := st.CreateRelease(ctx, tx, r); err != nil {
			return err
		}
		return st.CreateDeployment(ctx, tx, &domain.Deployment{
			ServiceID: svc.ID, EnvironmentID: devEnv.ID, ReleaseID: r.ID,
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	projectSvc := NewProjectService(st)
	csSvc := NewChangesetService(st, projectSvc, NewReleaseService(st, projectSvc))
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	_, err = csSvc.StageChanges(ctx, "atomic-app", DefaultEnvironment, StageChangesInput{Changes: []StageChangeInput{
		{Type: "config", Key: "FOO", Value: strPtr("bar")},
		{Type: "image", Image: "demo:v2"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = csSvc.PushChangeset(ctx, "atomic-app", DefaultEnvironment, PushChangesetInput{})
	if err == nil {
		t.Fatal("expected push to fail while deployment active")
	}

	vars, err := st.ListConfigVars(ctx, svc.ID, devEnv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := vars["FOO"]; ok {
		t.Fatalf("config should roll back on failed push, got %+v", vars)
	}
	open, err := csSvc.GetChangeset(ctx, "atomic-app", DefaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if open.Status != domain.ChangesetOpen || len(open.Changes) != 2 {
		t.Fatalf("changeset should remain open with staged changes, got status=%s n=%d", open.Status, len(open.Changes))
	}
}

func TestProcessSnapshotIncludesCommandAndExpose(t *testing.T) {
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
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "snap-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}

	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	result, err := releaseSvc.CreateRelease(ctx, "snap-app", DefaultEnvironment, CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "snap:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	web, ok := result.Release.ProcessSnapshot["web"]
	if !ok {
		t.Fatal("missing web snapshot")
	}
	if web.Quantity != 1 || web.Expose != "http" {
		t.Fatalf("unexpected snapshot: %+v", web)
	}
}

func TestRollbackCopiesArtifactAndSnapshot(t *testing.T) {
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
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "rb-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	r1, err := releaseSvc.CreateRelease(ctx, "rb-app", DefaultEnvironment, CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"}, Description: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	// mark first deploy terminal so second can enqueue
	_ = st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateDeploymentStatus(ctx, tx, r1.Deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go")
	})
	_ = st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateDeploymentStatus(ctx, tx, r1.Deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	})
	_ = st.UpdateReleaseStatus(ctx, nil, r1.Release.ID, domain.ReleaseStatusSucceeded)

	r2, err := releaseSvc.CreateRelease(ctx, "rb-app", DefaultEnvironment, CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v2"}, Description: "second",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Transact(ctx, func(tx *sql.Tx) error {
		if err := st.UpdateDeploymentStatus(ctx, tx, r2.Deployment.ID, domain.DeploymentPending, domain.DeploymentDeploying, "go"); err != nil {
			return err
		}
		return st.SupersedeRunningDeployments(ctx, tx, r2.Deployment.ServiceID, r2.Deployment.EnvironmentID, r2.Deployment.ID)
	})
	_ = st.Transact(ctx, func(tx *sql.Tx) error {
		return st.UpdateDeploymentStatus(ctx, tx, r2.Deployment.ID, domain.DeploymentDeploying, domain.DeploymentRunning, "ok")
	})

	rb, err := releaseSvc.Rollback(ctx, "rb-app", DefaultEnvironment, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if rb.Release.Version != 3 {
		t.Fatalf("expected v3, got v%d", rb.Release.Version)
	}
	if rb.Release.ArtifactRef != "app:v1" {
		t.Fatalf("artifact: %s", rb.Release.ArtifactRef)
	}
	if rb.Release.Description != "Rollback to v1" {
		t.Fatalf("desc: %s", rb.Release.Description)
	}
}

func TestChangesetPinBlocksOtherEnvironment(t *testing.T) {
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
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "pin-app"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	csSvc := NewChangesetService(st, projectSvc, releaseSvc)
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	if _, err := projectSvc.CreateEnvironment(ctx, "pin-app", CreateEnvironmentInput{
		Name:   "staging",
		Target: TargetInput{Type: "stub", Namespace: "stg"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := csSvc.StageChanges(ctx, "pin-app", "dev", StageChangesInput{Changes: []StageChangeInput{
		{Type: "config", Key: "A", Value: strPtr("1")},
	}}); err != nil {
		t.Fatal(err)
	}
	_, err = csSvc.StageChanges(ctx, "pin-app", "staging", StageChangesInput{Changes: []StageChangeInput{
		{Type: "config", Key: "B", Value: strPtr("2")},
	}})
	if err == nil {
		t.Fatal("expected conflict when staging on other env")
	}
}

func TestStageChangesRejectsWrongService(t *testing.T) {
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

	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	project := &domain.Project{WorkspaceID: workspaceID, Name: "my-api"}
	if err := st.CreateProject(ctx, project, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}

	projectSvc := NewProjectService(st)
	csSvc := NewChangesetService(st, projectSvc, NewReleaseService(st, projectSvc))
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	_, err = csSvc.StageChanges(ctx, "my-api", DefaultEnvironment, StageChangesInput{
		Service: "other-service",
		Changes: []StageChangeInput{{Type: "config", Key: "X", Value: strPtr("1")}},
	})
	if err == nil {
		t.Fatal("expected error for mismatched service")
	}
}

func strPtr(s string) *string { return &s }