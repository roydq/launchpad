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
	imagePayload, _ := json.Marshal(domain.ImageChangePayload{Image: "app:v2"})

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

	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	app := &domain.App{TeamID: teamID, Name: "batch-app", TargetType: "stub", TargetConfig: json.RawMessage(`{"namespace":"default"}`)}
	if err := st.CreateApp(ctx, app); err != nil {
		t.Fatal(err)
	}

	appSvc := NewAppService(st)
	releaseSvc := NewReleaseService(st, appSvc)
	csSvc := NewChangesetService(st, appSvc, releaseSvc)

	ctx = context.WithValue(ctx, auth.ContextTeamID, teamID)

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

	vars, err := st.ListConfigVars(ctx, app.ID)
	if err != nil {
		t.Fatal(err)
	}
	if vars["FOO"] != "bar" {
		t.Fatalf("config not applied: %+v", vars)
	}
}

func strPtr(s string) *string { return &s }