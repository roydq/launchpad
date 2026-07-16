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

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
