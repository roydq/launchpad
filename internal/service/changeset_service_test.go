package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
)

func TestMaterializeChanges(t *testing.T) {
	configPayload, _ := json.Marshal(domain.ConfigChangePayload{Key: "PORT", Value: strPtr("3000")})
	scalePayload, _ := json.Marshal(domain.ScaleChangePayload{Process: "web", Quantity: 3})
	imagePayload, _ := json.Marshal(domain.ImageChangePayload{ArtifactRef: "app:v2"})

	changes := []domain.ChangesetChange{
		{Type: domain.ChangeTypeConfig, Payload: configPayload},
		{Type: domain.ChangeTypeScale, Payload: scalePayload},
		{Type: domain.ChangeTypeImage, Payload: imagePayload},
	}

	cfg, scales, image, err := materializeChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	if cfg["PORT"] == nil || *cfg["PORT"] != "3000" {
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

	_, err = csSvc.StageChanges(ctx, "batch-app", StageChangesInput{Changes: []StageChangeInput{
		{Type: "config", Key: "FOO", Value: strPtr("bar")},
		{Type: "image", Image: "demo:v1"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	open, err := csSvc.GetChangeset(ctx, "batch-app")
	if err != nil {
		t.Fatal(err)
	}
	if len(open.Changes) != 2 {
		t.Fatalf("expected 2 staged changes, got %d", len(open.Changes))
	}

	result, err := csSvc.PushChangeset(ctx, "batch-app", PushChangesetInput{Description: "batch deploy"})
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
	devEnv, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, devEnvironment)
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

	_, err = csSvc.StageChanges(ctx, "my-api", StageChangesInput{
		Service: "other-service",
		Changes: []StageChangeInput{{Type: "config", Key: "X", Value: strPtr("1")}},
	})
	if err == nil {
		t.Fatal("expected error for mismatched service")
	}
}

func strPtr(s string) *string { return &s }