package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
)

type manifestHarness struct {
	ctx        context.Context
	st         *store.Store
	projects   *ProjectService
	changesets *ChangesetService
	manifests  *ManifestService
}

func newManifestHarness(t *testing.T) *manifestHarness {
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
	st := store.New(db, driver).WithSecrets(testSecretsBox(t))
	workspaceID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, auth.ContextTeamID, workspaceID)
	ps := NewProjectService(st)
	cfg := NewConfigService(st, ps)
	rel := NewReleaseService(st, ps)
	cs := NewChangesetService(st, ps, rel)
	return &manifestHarness{
		ctx: ctx, st: st, projects: ps, changesets: cs,
		manifests: NewManifestService(st, ps, cfg, cs, rel),
	}
}

func qty(n int) *int { return &n }

func sampleDoc(name string) domain.Manifest {
	return domain.Manifest{
		Version: 1,
		Project: name,
		Processes: map[string]domain.ManifestProcess{
			"web": {Command: "serve", Quantity: qty(1), Expose: "http"},
		},
		Environments: map[string]domain.ManifestEnvironment{
			"dev": {
				Target:    "stub",
				Namespace: "default",
				Image:     "hello:v1",
				Config: domain.ManifestConfig{
					Service: map[string]string{"PORT": "8080"},
				},
			},
		},
	}
}

func TestManifestApplyCreatesProjectAndStages(t *testing.T) {
	h := newManifestHarness(t)
	rep, err := h.manifests.Apply(h.ctx, "yaml-new", "dev", sampleDoc("yaml-new"))
	if err != nil {
		t.Fatal(err)
	}
	if !rep.CreatedProject {
		t.Fatal("expected created_project")
	}
	if rep.Changeset == nil || rep.Changeset.ChangeCount == 0 {
		t.Fatalf("expected staged changeset, got %+v staged=%v", rep.Changeset, rep.Staged)
	}
	joined := strings.Join(rep.Staged, ",")
	if !strings.Contains(joined, "image") || !strings.Contains(joined, "config.service.PORT") {
		t.Fatalf("staged %v", rep.Staged)
	}
}

func TestManifestApplyNoOp(t *testing.T) {
	h := newManifestHarness(t)
	if _, err := h.projects.CreateProject(h.ctx, CreateProjectInput{
		Name: "yaml-noop", Target: TargetInput{Type: "stub", Namespace: "default"},
	}); err != nil {
		t.Fatal(err)
	}
	doc := domain.Manifest{
		Version: 1,
		Project: "yaml-noop",
		Processes: map[string]domain.ManifestProcess{
			"web": {Command: "", Quantity: qty(1), Expose: "http"},
		},
		Environments: map[string]domain.ManifestEnvironment{
			"dev": {Target: "stub", Namespace: "default"},
		},
	}
	rep, err := h.manifests.Apply(h.ctx, "yaml-noop", "dev", doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Staged) != 0 {
		t.Fatalf("staged %v", rep.Staged)
	}
	if rep.Changeset != nil {
		t.Fatalf("changeset %v", rep.Changeset)
	}
}

func TestManifestApplyRejectsSecretValue(t *testing.T) {
	h := newManifestHarness(t)
	doc := sampleDoc("yaml-sec")
	env := doc.Environments["dev"]
	env.Config.Service["DATABASE_URL"] = "postgres://x"
	env.Config.SecretKeys.Service = []string{"DATABASE_URL"}
	doc.Environments["dev"] = env
	_, err := h.manifests.Apply(h.ctx, "yaml-sec", "dev", doc)
	if err == nil || !strings.Contains(err.Error(), "manifest must not contain a secret value") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestApplyDoesNotPrune(t *testing.T) {
	h := newManifestHarness(t)
	if _, err := h.projects.CreateProject(h.ctx, CreateProjectInput{
		Name: "yaml-prune", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	cmd := "run-worker"
	q := 1
	exp := "none"
	img := "hello:v1"
	if _, err := h.changesets.StageChanges(h.ctx, "yaml-prune", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.set", Name: "worker", Command: &cmd, Quantity: &q, Expose: &exp},
			{Type: "image", Image: img},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.changesets.PushChangeset(h.ctx, "yaml-prune", "dev", PushChangesetInput{Description: "add worker"}); err != nil {
		t.Fatal(err)
	}
	doc := domain.Manifest{
		Version: 1,
		Project: "yaml-prune",
		Processes: map[string]domain.ManifestProcess{
			"web": {Quantity: qty(1), Expose: "http"},
		},
		Environments: map[string]domain.ManifestEnvironment{
			"dev": {Target: "stub"},
		},
	}
	rep, err := h.manifests.Apply(h.ctx, "yaml-prune", "dev", doc)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range rep.Warnings {
		if strings.Contains(w, "worker") && strings.Contains(w, "not pruned") {
			found = true
		}
	}
	if !found {
		t.Fatalf("warnings %v", rep.Warnings)
	}
	for _, s := range rep.Staged {
		if strings.Contains(s, "unset") {
			t.Fatalf("pruned via %v", rep.Staged)
		}
	}
}

func TestManifestApplyMissingDevCannotCreate(t *testing.T) {
	h := newManifestHarness(t)
	doc := domain.Manifest{
		Version: 1,
		Project: "yaml-staging-only",
		Environments: map[string]domain.ManifestEnvironment{
			"staging": {Target: "stub"},
		},
	}
	_, err := h.manifests.Apply(h.ctx, "yaml-staging-only", "staging", doc)
	if err == nil || !strings.Contains(err.Error(), "environments.dev") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestApplyCreatesSelectedEnv(t *testing.T) {
	h := newManifestHarness(t)
	if _, err := h.projects.CreateProject(h.ctx, CreateProjectInput{
		Name: "yaml-env", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	doc := domain.Manifest{
		Version: 1,
		Project: "yaml-env",
		Environments: map[string]domain.ManifestEnvironment{
			"dev":     {Target: "stub"},
			"staging": {Target: "stub", Namespace: "stg", Image: "hello:v2"},
		},
	}
	rep, err := h.manifests.Apply(h.ctx, "yaml-env", "staging", doc)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.CreatedEnvironment {
		t.Fatal("expected created_environment")
	}
	if _, err := h.projects.GetEnvironment(h.ctx, "yaml-env", "staging"); err != nil {
		t.Fatal(err)
	}
}

func TestManifestExportRedactsAndIgnoresPending(t *testing.T) {
	h := newManifestHarness(t)
	if _, err := h.projects.CreateProject(h.ctx, CreateProjectInput{
		Name: "yaml-exp2", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	project, svc, env, err := h.projects.resolvePrimaryService(h.ctx, "yaml-exp2", "dev")
	if err != nil {
		t.Fatal(err)
	}
	plain := "8080"
	secret := "postgres://secret"
	sec := domain.SensitivitySecret
	pln := domain.SensitivityPlain
	if err := h.st.Transact(h.ctx, func(tx *sql.Tx) error {
		return h.st.MergeConfigWritesTx(h.ctx, tx, "service", svc.ID, env.ID, uuid.Nil, map[string]store.ConfigWrite{
			"PORT":         {Value: &plain, Sensitivity: &pln},
			"DATABASE_URL": {Value: &secret, Sensitivity: &sec},
		})
	}); err != nil {
		t.Fatal(err)
	}
	_ = project

	pending := "9999"
	if _, err := h.changesets.StageChanges(h.ctx, "yaml-exp2", "dev", StageChangesInput{
		Changes: []StageChangeInput{{Type: "config", Key: "PENDING", Value: &pending}},
	}); err != nil {
		t.Fatal(err)
	}

	doc, err := h.manifests.Export(h.ctx, "yaml-exp2", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Environments["dev"].Config.Service["PENDING"]; ok {
		t.Fatal("export must not overlay pending config")
	}
	if doc.Environments["dev"].Config.Service["PORT"] != "8080" {
		t.Fatalf("plain PORT: %+v", doc.Environments["dev"].Config)
	}
	found := false
	for _, k := range doc.Environments["dev"].Config.SecretKeys.Service {
		if k == "DATABASE_URL" {
			found = true
		}
	}
	if !found {
		t.Fatalf("secret keys %+v", doc.Environments["dev"].Config.SecretKeys)
	}
	if _, ok := doc.Environments["dev"].Config.Service["DATABASE_URL"]; ok {
		t.Fatal("secret value leaked")
	}
}

func TestManifestExportUnknownEnv(t *testing.T) {
	h := newManifestHarness(t)
	if _, err := h.projects.CreateProject(h.ctx, CreateProjectInput{
		Name: "yaml-miss", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.manifests.Export(h.ctx, "yaml-miss", "nope")
	if !errors.Is(err, launchpad.ErrNotFound) {
		t.Fatalf("got %v", err)
	}
}

func TestManifestApplyProjectMismatch(t *testing.T) {
	h := newManifestHarness(t)
	_, err := h.manifests.Apply(h.ctx, "app-b", "dev", sampleDoc("app-a"))
	if err == nil || !strings.Contains(err.Error(), "project name does not match") {
		t.Fatalf("got %v", err)
	}
}

func TestManifestApplyPinConflict(t *testing.T) {
	h := newManifestHarness(t)
	if _, err := h.projects.CreateProject(h.ctx, CreateProjectInput{
		Name: "yaml-pin", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.projects.CreateEnvironment(h.ctx, "yaml-pin", CreateEnvironmentInput{
		Name: "staging", Target: TargetInput{Type: "stub"},
	}); err != nil {
		t.Fatal(err)
	}
	v := "1"
	if _, err := h.changesets.StageChanges(h.ctx, "yaml-pin", "staging", StageChangesInput{
		Changes: []StageChangeInput{{Type: "config", Key: "A", Value: &v}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := h.manifests.Apply(h.ctx, "yaml-pin", "dev", sampleDoc("yaml-pin"))
	if err == nil || !errors.Is(err, launchpad.ErrConflict) {
		t.Fatalf("got %v", err)
	}
}
