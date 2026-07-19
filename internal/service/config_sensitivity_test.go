package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
)

func TestSecretConfigRedactedAndDeployable(t *testing.T) {
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
	project := &domain.Project{WorkspaceID: workspaceID, Name: "sec-app"}
	env := &domain.Environment{
		TargetType:   "stub",
		TargetConfig: json.RawMessage(`{"namespace":"default"}`),
	}
	if err := st.CreateProject(ctx, project, env); err != nil {
		t.Fatal(err)
	}
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)

	ps := NewProjectService(st)
	cs := NewConfigService(st, ps)
	rs := NewReleaseService(st, ps)
	chs := NewChangesetService(st, ps, rs)

	sec := domain.SensitivitySecret
	_, err = chs.StageChanges(ctx, "sec-app", DefaultEnvironment, StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "config", Key: "DATABASE_URL", Value: strPtr("postgres://secret"), Sensitivity: &sec},
			{Type: "config", Key: "PORT", Value: strPtr("3000")},
			{Type: "image", Image: "app:1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := chs.PushChangeset(ctx, "sec-app", DefaultEnvironment, PushChangesetInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Release.ConfigResolved["DATABASE_URL"] != "postgres://secret" {
		t.Fatalf("internal release must keep plaintext: %+v", result.Release.ConfigResolved)
	}
	if result.Release.ConfigSensitivity["DATABASE_URL"] != domain.SensitivitySecret {
		t.Fatalf("sensitivity snapshot: %+v", result.Release.ConfigSensitivity)
	}

	got, err := cs.GetConfig(ctx, "sec-app", DefaultEnvironment, "")
	if err != nil {
		t.Fatal(err)
	}
	if got["DATABASE_URL"] != domain.SecretSentinel {
		t.Fatalf("expected redacted secret, got %q", got["DATABASE_URL"])
	}
	if got["PORT"] != "3000" {
		t.Fatalf("PORT: %q", got["PORT"])
	}

	typed, err := cs.GetConfigTyped(ctx, "sec-app", DefaultEnvironment, "service")
	if err != nil {
		t.Fatal(err)
	}
	if typed["DATABASE_URL"].Sensitivity != domain.SensitivitySecret || typed["DATABASE_URL"].Value != nil || !typed["DATABASE_URL"].Set {
		t.Fatalf("typed secret entry: %+v", typed["DATABASE_URL"])
	}

	// Sticky: re-merge without sensitivity stays secret.
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	envRow, err := st.GetEnvironmentByProjectAndName(ctx, project.ID, DefaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MergeConfigWritesTx(ctx, nil, "service", svc.ID, envRow.ID, uuid.Nil, map[string]store.ConfigWrite{
		"DATABASE_URL": {Value: strPtr("postgres://rotated")},
	}); err != nil {
		t.Fatal(err)
	}
	_, sens, err := st.ResolveConfigWithSensitivity(ctx, project.ID, svc.ID, envRow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sens["DATABASE_URL"] != domain.SensitivitySecret {
		t.Fatalf("sticky expected secret, got %+v", sens)
	}

	// Stage + preview must not leak secret plaintext in summary.
	_, err = chs.StageChanges(ctx, "sec-app", DefaultEnvironment, StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "config", Key: "DATABASE_URL", Value: strPtr("postgres://new"), Sensitivity: &sec},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, err := chs.PreviewPending(ctx, "sec-app", DefaultEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prev.Summary, "postgres://") {
		t.Fatalf("summary leaked secret: %s", prev.Summary)
	}
	if !strings.Contains(prev.Summary, "[secret]") && !strings.Contains(prev.Summary, domain.SecretSentinel) {
		// Accept either display form
		t.Logf("summary: %s", prev.Summary)
	}
}
