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

func TestFoldChangesLastWriteWins(t *testing.T) {
	v1, v2 := "1", "2"
	changes := []domain.ChangesetChange{
		{Type: domain.ChangeTypeConfig, Payload: mustJSON(t, domain.ConfigChangePayload{Key: "PORT", Value: &v1})},
		{Type: domain.ChangeTypeConfig, Payload: mustJSON(t, domain.ConfigChangePayload{Key: "PORT", Value: &v2})},
		{Type: domain.ChangeTypeImage, Payload: mustJSON(t, domain.ImageChangePayload{ArtifactRef: "img:a"})},
		{Type: domain.ChangeTypeImage, Payload: mustJSON(t, domain.ImageChangePayload{ArtifactRef: "img:b"})},
		{Type: domain.ChangeTypeScale, Payload: mustJSON(t, domain.ScaleChangePayload{Process: "web", Quantity: 1})},
		{Type: domain.ChangeTypeScale, Payload: mustJSON(t, domain.ScaleChangePayload{Process: "web", Quantity: 3})},
	}
	folded, err := FoldChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	if folded.Image != "img:b" {
		t.Fatalf("image %q", folded.Image)
	}
	if folded.Config["PORT"] == nil || *folded.Config["PORT"] != "2" {
		t.Fatalf("PORT %+v", folded.Config["PORT"])
	}
	if folded.Scales["web"] != 3 {
		t.Fatalf("scale %d", folded.Scales["web"])
	}
}

func TestBuildDiffConfigOps(t *testing.T) {
	nine, three := "9", "3"
	pending := FoldedPending{
		Config: map[string]*string{"A": &nine, "B": nil, "C": &three},
	}
	baseline := &domain.Release{ConfigResolved: map[string]string{"A": "1", "B": "2"}}
	diff := BuildDiff(pending, baseline)
	ops := map[string]ConfigDiffOp{}
	for _, c := range diff.Config {
		ops[c.Key] = c
	}
	if ops["A"].Op != "change" || *ops["A"].From != "1" || *ops["A"].To != "9" {
		t.Fatalf("A: %+v", ops["A"])
	}
	if ops["B"].Op != "remove" {
		t.Fatalf("B: %+v", ops["B"])
	}
	if ops["C"].Op != "add" || *ops["C"].To != "3" {
		t.Fatalf("C: %+v", ops["C"])
	}
}

func TestPreviewPendingService(t *testing.T) {
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
	ws := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, auth.ContextTeamID, ws)
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	csSvc := NewChangesetService(st, projectSvc, releaseSvc)

	if err := st.CreateProject(ctx, &domain.Project{WorkspaceID: ws, Name: "prev-app"}, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	// baseline release
	r1, err := releaseSvc.CreateRelease(ctx, "prev-app", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, r1.Deployment)

	port := "9999"
	if _, err := csSvc.StageChanges(ctx, "prev-app", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "image", Image: "app:v2"},
			{Type: "config", Key: "PORT", Value: &port},
		},
	}); err != nil {
		t.Fatal(err)
	}

	prev, err := csSvc.PreviewPending(ctx, "prev-app", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if prev.Mode != "pending" || !prev.HasPending {
		t.Fatalf("%+v", prev)
	}
	if prev.Pending == nil || prev.Pending.Image != "app:v2" {
		t.Fatalf("pending: %+v", prev.Pending)
	}
	if prev.Diff.Image == nil || prev.Diff.Image.To != "app:v2" {
		t.Fatalf("diff image: %+v", prev.Diff.Image)
	}
	if prev.BaselineVersion == nil || *prev.BaselineVersion != 1 {
		t.Fatalf("baseline: %+v", prev.BaselineVersion)
	}
	if prev.Summary == "" || prev.Summary == "No pending changes\n" {
		t.Fatalf("summary: %q", prev.Summary)
	}
}

func TestBuildSnapshotDiffFullUnion(t *testing.T) {
	from := &domain.Release{
		ArtifactRef:     "app:v1",
		ConfigResolved:  map[string]string{"A": "1", "B": "2", "SECRET": "s1"},
		ConfigSensitivity: map[string]string{"SECRET": domain.SensitivitySecret},
		ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 1}},
	}
	to := &domain.Release{
		ArtifactRef:     "app:v2",
		ConfigResolved:  map[string]string{"A": "9", "C": "3", "SECRET": "s2"},
		ConfigSensitivity: map[string]string{"SECRET": domain.SensitivitySecret},
		ProcessSnapshot: map[string]domain.ProcessSnapshot{"web": {Quantity: 3}},
	}
	diff := BuildSnapshotDiff(from, to)
	if diff.Image == nil || diff.Image.From != "app:v1" || diff.Image.To != "app:v2" {
		t.Fatalf("image: %+v", diff.Image)
	}
	ops := map[string]ConfigDiffOp{}
	for _, c := range diff.Config {
		ops[c.Key] = c
	}
	if ops["A"].Op != "change" || *ops["A"].From != "1" || *ops["A"].To != "9" {
		t.Fatalf("A: %+v", ops["A"])
	}
	if ops["B"].Op != "remove" || *ops["B"].From != "2" {
		t.Fatalf("B: %+v", ops["B"])
	}
	if ops["C"].Op != "add" || *ops["C"].To != "3" {
		t.Fatalf("C: %+v", ops["C"])
	}
	if ops["SECRET"].Op != "change" || *ops["SECRET"].From != domain.SecretSentinel || *ops["SECRET"].To != domain.SecretSentinel {
		t.Fatalf("SECRET redaction: %+v", ops["SECRET"])
	}
	if len(diff.Scale) != 1 || diff.Scale[0].Process != "web" || diff.Scale[0].From == nil || *diff.Scale[0].From != 1 || diff.Scale[0].To != 3 {
		t.Fatalf("scale: %+v", diff.Scale)
	}
	if FormatSnapshotDiffSummary(EffectiveDiff{}) != "No differences\n" {
		t.Fatal("empty summary")
	}
}

func TestPreviewEnvironments(t *testing.T) {
	ctx, st, releaseSvc, projectSvc, workspaceID := setupPromoteFixture(t)
	if err := st.CreateProject(ctx, &domain.Project{WorkspaceID: workspaceID, Name: "envdiff"}, &domain.Environment{Name: "dev", TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "envdiff", CreateEnvironmentInput{Name: "staging", Target: TargetInput{Type: "stub"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := projectSvc.CreateEnvironment(ctx, "envdiff", CreateEnvironmentInput{Name: "production", Target: TargetInput{Type: "stub"}}); err != nil {
		t.Fatal(err)
	}
	project, err := projectSvc.GetProject(ctx, "envdiff")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := st.GetServiceByProjectAndName(ctx, project.ID, project.PrimaryService)
	if err != nil {
		t.Fatal(err)
	}
	staging, err := projectSvc.GetEnvironment(ctx, "envdiff", "staging")
	if err != nil {
		t.Fatal(err)
	}
	prod, err := projectSvc.GetEnvironment(ctx, "envdiff", "production")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Transact(ctx, func(tx *sql.Tx) error {
		if err := st.MergeConfigVarsTx(ctx, tx, svc.ID, staging.ID, map[string]*string{"PORT": strPtr("3000")}); err != nil {
			return err
		}
		return st.MergeConfigVarsTx(ctx, tx, svc.ID, prod.ID, map[string]*string{"PORT": strPtr("8080")})
	}); err != nil {
		t.Fatal(err)
	}

	src, err := releaseSvc.CreateRelease(ctx, "envdiff", "staging", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:stage"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, src.Deployment)

	dst, err := releaseSvc.CreateRelease(ctx, "envdiff", "production", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:prod"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, dst.Deployment)

	csSvc := NewChangesetService(st, projectSvc, releaseSvc)
	prev, err := csSvc.PreviewEnvironments(ctx, "envdiff", "staging", "production")
	if err != nil {
		t.Fatal(err)
	}
	if prev.Mode != "environments" || prev.FromEnvironment != "staging" || prev.ToEnvironment != "production" {
		t.Fatalf("%+v", prev)
	}
	if prev.FromVersion == nil || *prev.FromVersion != 1 || prev.ToVersion == nil || *prev.ToVersion != 2 {
		t.Fatalf("versions from=%v to=%v", prev.FromVersion, prev.ToVersion)
	}
	if prev.Diff.Image == nil || prev.Diff.Image.From != "app:stage" || prev.Diff.Image.To != "app:prod" {
		t.Fatalf("image: %+v", prev.Diff.Image)
	}
	foundPort := false
	for _, c := range prev.Diff.Config {
		if c.Key == "PORT" && c.Op == "change" && c.From != nil && *c.From == "3000" && c.To != nil && *c.To == "8080" {
			foundPort = true
		}
	}
	if !foundPort {
		t.Fatalf("PORT op missing: %+v", prev.Diff.Config)
	}
	if prev.MatchesBaseline {
		t.Fatal("expected differences")
	}

	// Same env
	if _, err := csSvc.PreviewEnvironments(ctx, "envdiff", "staging", "staging"); err == nil {
		t.Fatal("expected same-env error")
	}
	// Never deployed side: empty snapshot
	if _, err := projectSvc.CreateEnvironment(ctx, "envdiff", CreateEnvironmentInput{Name: "empty", Target: TargetInput{Type: "stub"}}); err != nil {
		t.Fatal(err)
	}
	emptyPrev, err := csSvc.PreviewEnvironments(ctx, "envdiff", "staging", "empty")
	if err != nil {
		t.Fatal(err)
	}
	if emptyPrev.ToVersion != nil {
		t.Fatalf("empty to version: %v", emptyPrev.ToVersion)
	}
	if emptyPrev.Diff.IsEmpty() {
		t.Fatal("expected removals vs empty env")
	}

	// Unknown env
	if _, err := csSvc.PreviewEnvironments(ctx, "envdiff", "staging", "nope"); err == nil {
		t.Fatal("expected 404 for unknown env")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
