package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/launchpad/launchpad/internal/auth"
	"github.com/launchpad/launchpad/internal/domain"
	"github.com/launchpad/launchpad/internal/store"
	"github.com/launchpad/launchpad/pkg/launchpad"
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
		ArtifactRef:       "app:v1",
		ConfigResolved:    map[string]string{"A": "1", "B": "2", "SECRET": "s1"},
		ConfigSensitivity: map[string]string{"SECRET": domain.SensitivitySecret},
		ProcessSnapshot:   map[string]domain.ProcessSnapshot{"web": {Quantity: 1}},
	}
	to := &domain.Release{
		ArtifactRef:       "app:v2",
		ConfigResolved:    map[string]string{"A": "9", "C": "3", "SECRET": "s2"},
		ConfigSensitivity: map[string]string{"SECRET": domain.SensitivitySecret},
		ProcessSnapshot:   map[string]domain.ProcessSnapshot{"web": {Quantity: 3}},
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
	if len(diff.Process) != 0 {
		t.Fatalf("quantity-only should not emit process op: %+v", diff.Process)
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

func webOnlyRelease() *domain.Release {
	return &domain.Release{
		ProcessSnapshot: map[string]domain.ProcessSnapshot{
			"web": {Quantity: 1, Expose: "http"},
		},
	}
}

func pendingProcessDiff(t *testing.T, changes []domain.ChangesetChange, baseline *domain.Release) (FoldedPending, EffectiveDiff) {
	t.Helper()
	folded, err := FoldChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	var baselineSnap map[string]domain.ProcessSnapshot
	if baseline != nil {
		baselineSnap = baseline.ProcessSnapshot
	}
	effective := applyProcessOpsToSnapshot(baselineSnap, folded)
	diff := BuildDiff(folded, baseline)
	diff.Process = diffProcessSnapshots(baselineSnap, effective, scaleNameSet(folded.Scales))
	folded.Processes = sparseProcessOverlay(folded, baselineSnap, effective)
	return folded, diff
}

func processOpByName(t *testing.T, diff EffectiveDiff, name string) ProcessDiffOp {
	t.Helper()
	for _, op := range diff.Process {
		if op.Name == name {
			return op
		}
	}
	t.Fatalf("missing process op %q in %+v", name, diff.Process)
	return ProcessDiffOp{}
}

func TestFoldChangesAcceptsProcessTypes(t *testing.T) {
	cmd := "run-worker"
	folded, err := FoldChanges([]domain.ChangesetChange{
		{Type: domain.ChangeTypeProcessSet, Payload: mustJSON(t, domain.ProcessSetPayload{Name: "worker", Command: &cmd})},
		{Type: domain.ChangeTypeProcessUnset, Payload: mustJSON(t, domain.ProcessUnsetPayload{Name: "legacy"})},
		{Type: domain.ChangeTypeProcessApply, Payload: mustJSON(t, domain.ProcessApplyPayload{Procfile: "web: serve\n"})},
		{Type: domain.ChangeTypeProcessApply, Payload: mustJSON(t, domain.ProcessApplyPayload{Procfile: "web: serve\nworker: sidekiq\n"})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if folded.IsEmpty() {
		t.Fatal("expected process ops to make fold non-empty")
	}
	if folded.Processes != nil {
		t.Fatalf("fold must not populate API overlay: %+v", folded.Processes)
	}
	if len(folded.processSets) != 1 || folded.processSets[0].Name != "worker" {
		t.Fatalf("sets: %+v", folded.processSets)
	}
	if len(folded.processUnsets) != 1 || folded.processUnsets[0] != "legacy" {
		t.Fatalf("unsets: %+v", folded.processUnsets)
	}
	if folded.processApply == nil || folded.processApply.Procfile != "web: serve\nworker: sidekiq\n" {
		t.Fatalf("last apply should win: %+v", folded.processApply)
	}

	_, err = FoldChanges([]domain.ChangesetChange{
		{Type: "mystery", Payload: []byte(`{}`)},
	})
	if err == nil || !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("unknown type: %v", err)
	}
}

func TestFoldChangesInvalidProcessPayloads(t *testing.T) {
	cases := []struct {
		name string
		ch   domain.ChangesetChange
	}{
		{"set missing name", domain.ChangesetChange{Type: domain.ChangeTypeProcessSet, Payload: mustJSON(t, domain.ProcessSetPayload{})}},
		{"unset missing name", domain.ChangesetChange{Type: domain.ChangeTypeProcessUnset, Payload: mustJSON(t, domain.ProcessUnsetPayload{})}},
		{"apply empty procfile", domain.ChangesetChange{Type: domain.ChangeTypeProcessApply, Payload: mustJSON(t, domain.ProcessApplyPayload{Procfile: ""})}},
		{"apply invalid procfile", domain.ChangesetChange{Type: domain.ChangeTypeProcessApply, Payload: mustJSON(t, domain.ProcessApplyPayload{Procfile: "not-a-procfile"})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FoldChanges([]domain.ChangesetChange{tc.ch})
			if err == nil || !errors.Is(err, launchpad.ErrBadRequest) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestBuildDiffProcessSetAddsWorker(t *testing.T) {
	cmd := "run-worker"
	_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
		{Type: domain.ChangeTypeProcessSet, Payload: mustJSON(t, domain.ProcessSetPayload{Name: "worker", Command: &cmd})},
	}, webOnlyRelease())
	if len(diff.Process) != 1 {
		t.Fatalf("process: %+v", diff.Process)
	}
	op := diff.Process[0]
	if op.Op != "add" || op.Name != "worker" || op.To == nil || op.To.Command != "run-worker" {
		t.Fatalf("op: %+v", op)
	}
	for _, p := range diff.Process {
		if p.Name == "web" {
			t.Fatalf("web must not appear: %+v", p)
		}
	}
}

func TestBuildDiffProcessFieldChanges(t *testing.T) {
	cmd := "serve"
	exp := "none"
	health := &domain.ProcessHealth{Type: "http"}
	ext := map[string]json.RawMessage{"kubernetes": json.RawMessage(`{"foo":true}`)}
	_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
		{Type: domain.ChangeTypeProcessSet, Payload: mustJSON(t, domain.ProcessSetPayload{
			Name: "web", Command: &cmd, Expose: &exp, Health: health, TargetExtensions: ext,
		})},
	}, webOnlyRelease())
	op := processOpByName(t, diff, "web")
	if op.Op != "change" || op.To == nil || op.To.Command != "serve" || op.To.Expose != "none" {
		t.Fatalf("op: %+v", op)
	}
	if op.To.Health == nil || op.To.Health.Type != "http" || op.To.Health.Path != "/healthz" {
		t.Fatalf("health: %+v", op.To.Health)
	}
	if strings.Join(op.Fields, ",") != "command,expose,health,target_extensions" {
		t.Fatalf("fields: %v", op.Fields)
	}
}

func TestBuildDiffScaleQuantityOnlyNoProcessOp(t *testing.T) {
	_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
		{Type: domain.ChangeTypeScale, Payload: mustJSON(t, domain.ScaleChangePayload{Process: "web", Quantity: 3})},
	}, webOnlyRelease())
	if len(diff.Scale) != 1 || diff.Scale[0].Process != "web" || diff.Scale[0].To != 3 {
		t.Fatalf("scale: %+v", diff.Scale)
	}
	if len(diff.Process) != 0 {
		t.Fatalf("process: %+v", diff.Process)
	}
}

func TestBuildDiffScaleUnknownProcessNoProcessAdd(t *testing.T) {
	_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
		{Type: domain.ChangeTypeScale, Payload: mustJSON(t, domain.ScaleChangePayload{Process: "worker", Quantity: 3})},
	}, webOnlyRelease())
	if len(diff.Scale) != 1 || diff.Scale[0].Process != "worker" || diff.Scale[0].To != 3 {
		t.Fatalf("scale: %+v", diff.Scale)
	}
	for _, op := range diff.Process {
		if op.Name == "worker" {
			t.Fatalf("scale must not invent process: %+v", op)
		}
	}
}

func TestBuildDiffProcessSetQuantityOnly(t *testing.T) {
	qty := 3
	_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
		{Type: domain.ChangeTypeProcessSet, Payload: mustJSON(t, domain.ProcessSetPayload{Name: "web", Quantity: &qty})},
	}, webOnlyRelease())
	if len(diff.Scale) != 0 {
		t.Fatalf("scale: %+v", diff.Scale)
	}
	op := processOpByName(t, diff, "web")
	if op.Op != "change" || len(op.Fields) != 1 || op.Fields[0] != "quantity" {
		t.Fatalf("op: %+v", op)
	}
}

func TestBuildDiffProcessUnsetAndApply(t *testing.T) {
	baseline := &domain.Release{
		ProcessSnapshot: map[string]domain.ProcessSnapshot{
			"web":    {Quantity: 1, Expose: "http"},
			"worker": {Command: "run-worker", Quantity: 1, Expose: "none"},
		},
	}
	t.Run("unset", func(t *testing.T) {
		_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
			{Type: domain.ChangeTypeProcessUnset, Payload: mustJSON(t, domain.ProcessUnsetPayload{Name: "worker"})},
		}, baseline)
		op := processOpByName(t, diff, "worker")
		if op.Op != "remove" || op.From == nil {
			t.Fatalf("op: %+v", op)
		}
		for _, p := range diff.Process {
			if p.Name == "web" {
				t.Fatalf("web must not appear: %+v", p)
			}
		}
	})
	t.Run("apply", func(t *testing.T) {
		_, diff := pendingProcessDiff(t, []domain.ChangesetChange{
			{Type: domain.ChangeTypeProcessApply, Payload: mustJSON(t, domain.ProcessApplyPayload{Procfile: "web: serve\n"})},
		}, baseline)
		rm := processOpByName(t, diff, "worker")
		if rm.Op != "remove" {
			t.Fatalf("worker: %+v", rm)
		}
		web := processOpByName(t, diff, "web")
		if web.Op != "change" || web.To == nil || web.To.Command != "serve" {
			t.Fatalf("web: %+v", web)
		}
	})
}

func TestPreviewPendingProcessSet(t *testing.T) {
	ctx, st, csSvc, releaseSvc := setupPreviewProject(t, "prev-proc-set")
	r1, err := releaseSvc.CreateRelease(ctx, "prev-proc-set", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, r1.Deployment)

	cmd := "run-worker"
	if _, err := csSvc.StageChanges(ctx, "prev-proc-set", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.set", Name: "worker", Command: &cmd},
		},
	}); err != nil {
		t.Fatal(err)
	}
	prev, err := csSvc.PreviewPending(ctx, "prev-proc-set", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if !prev.HasPending {
		t.Fatalf("%+v", prev)
	}
	op := processOpByName(t, prev.Diff, "worker")
	if op.Op != "add" || op.To == nil || op.To.Command != "run-worker" {
		t.Fatalf("op: %+v", op)
	}
	if !strings.Contains(prev.Summary, "## Process") {
		t.Fatalf("summary: %q", prev.Summary)
	}
	if prev.Pending == nil {
		t.Fatal("pending overlay missing")
	}
	w := prev.Pending.Processes["worker"]
	if w == nil || w.Command != "run-worker" {
		t.Fatalf("overlay worker: %+v", prev.Pending.Processes)
	}
	if _, ok := prev.Pending.Processes["web"]; ok {
		t.Fatalf("web must stay out of overlay: %+v", prev.Pending.Processes)
	}
}

func TestPreviewPendingProcessUnsetOverlay(t *testing.T) {
	ctx, st, csSvc, _ := setupPreviewProject(t, "prev-proc-unset")
	cmd := "run-worker"
	if _, err := csSvc.StageChanges(ctx, "prev-proc-unset", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.set", Name: "worker", Command: &cmd},
			{Type: "image", Image: "app:v1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	pushed, err := csSvc.PushChangeset(ctx, "prev-proc-unset", "dev", PushChangesetInput{})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, pushed.Deployment)

	if _, err := csSvc.StageChanges(ctx, "prev-proc-unset", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.unset", Name: "worker"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	prev, err := csSvc.PreviewPending(ctx, "prev-proc-unset", "dev")
	if err != nil {
		t.Fatal(err)
	}
	op := processOpByName(t, prev.Diff, "worker")
	if op.Op != "remove" {
		t.Fatalf("op: %+v", op)
	}
	if prev.Pending == nil {
		t.Fatal("pending overlay missing")
	}
	v, ok := prev.Pending.Processes["worker"]
	if !ok {
		t.Fatalf("worker key missing: %+v", prev.Pending.Processes)
	}
	if v != nil {
		t.Fatalf("want nil overlay, got %+v", v)
	}
}

func TestPreviewPendingProcessApplyOverlay(t *testing.T) {
	ctx, st, csSvc, _ := setupPreviewProject(t, "prev-proc-apply")
	cmd := "run-worker"
	if _, err := csSvc.StageChanges(ctx, "prev-proc-apply", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.set", Name: "worker", Command: &cmd},
			{Type: "image", Image: "app:v1"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	pushed, err := csSvc.PushChangeset(ctx, "prev-proc-apply", "dev", PushChangesetInput{})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, pushed.Deployment)

	if _, err := csSvc.StageChanges(ctx, "prev-proc-apply", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.apply", Procfile: "web: serve\n"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	prev, err := csSvc.PreviewPending(ctx, "prev-proc-apply", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if prev.Pending == nil {
		t.Fatal("pending overlay missing")
	}
	if v, ok := prev.Pending.Processes["worker"]; !ok || v != nil {
		t.Fatalf("want worker overlay nil tombstone, got ok=%v v=%+v map=%+v", ok, v, prev.Pending.Processes)
	}
	web, ok := prev.Pending.Processes["web"]
	if !ok || web == nil || web.Command != "serve" {
		t.Fatalf("web overlay: ok=%v %+v", ok, web)
	}
}

func TestPreviewReleasesProcessCommandChange(t *testing.T) {
	ctx, st, csSvc, releaseSvc := setupPreviewProject(t, "prev-rel-cmd")
	r1, err := releaseSvc.CreateRelease(ctx, "prev-rel-cmd", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, r1.Deployment)

	cmd := "serve"
	if _, err := csSvc.StageChanges(ctx, "prev-rel-cmd", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "process.set", Name: "web", Command: &cmd},
			{Type: "image", Image: "app:v2"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	pushed, err := csSvc.PushChangeset(ctx, "prev-rel-cmd", "dev", PushChangesetInput{})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, pushed.Deployment)

	prev, err := csSvc.PreviewReleases(ctx, "prev-rel-cmd", "dev", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	op := processOpByName(t, prev.Diff, "web")
	if op.Op != "change" {
		t.Fatalf("op: %+v", op)
	}
	foundCmd := false
	for _, f := range op.Fields {
		if f == "command" {
			foundCmd = true
		}
	}
	if !foundCmd {
		t.Fatalf("fields: %v", op.Fields)
	}
	if !strings.Contains(prev.Summary, "## Process") {
		t.Fatalf("summary: %q", prev.Summary)
	}
	if strings.Contains(prev.Summary, "no effective delta") {
		t.Fatalf("summary dropped process change: %q", prev.Summary)
	}
}

func TestPreviewReleasesIdenticalCompare(t *testing.T) {
	ctx, st, csSvc, releaseSvc := setupPreviewProject(t, "prev-rel-same")
	r1, err := releaseSvc.CreateRelease(ctx, "prev-rel-same", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, r1.Deployment)

	prev, err := csSvc.PreviewReleases(ctx, "prev-rel-same", "dev", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !prev.Diff.IsEmpty() {
		t.Fatalf("expected empty diff: %+v", prev.Diff)
	}
	if prev.Summary == "" {
		t.Fatal("identical release preview must not drop summary")
	}
	if !strings.Contains(prev.Summary, "No differences") {
		t.Fatalf("summary: %q", prev.Summary)
	}
}

func TestPreviewPendingScaleQuantityOnly(t *testing.T) {
	ctx, st, csSvc, releaseSvc := setupPreviewProject(t, "prev-scale-only")
	r1, err := releaseSvc.CreateRelease(ctx, "prev-scale-only", "dev", CreateReleaseInput{
		Source: SourceInput{Type: "image", Image: "app:v1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	markDeploySucceeded(t, ctx, st, r1.Deployment)

	qty := 3
	if _, err := csSvc.StageChanges(ctx, "prev-scale-only", "dev", StageChangesInput{
		Changes: []StageChangeInput{
			{Type: "scale", Process: "web", Quantity: &qty},
		},
	}); err != nil {
		t.Fatal(err)
	}
	prev, err := csSvc.PreviewPending(ctx, "prev-scale-only", "dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(prev.Diff.Process) != 0 {
		t.Fatalf("process: %+v", prev.Diff.Process)
	}
	if len(prev.Diff.Scale) != 1 || prev.Diff.Scale[0].Process != "web" || prev.Diff.Scale[0].To != 3 {
		t.Fatalf("scale: %+v", prev.Diff.Scale)
	}
}

func TestBuildSnapshotDiffProcessCommand(t *testing.T) {
	from := &domain.Release{
		ProcessSnapshot: map[string]domain.ProcessSnapshot{
			"web": {Quantity: 1, Expose: "http"},
		},
	}
	to := &domain.Release{
		ProcessSnapshot: map[string]domain.ProcessSnapshot{
			"web": {Command: "serve", Quantity: 1, Expose: "http"},
		},
	}
	diff := BuildSnapshotDiff(from, to)
	if len(diff.Scale) != 0 {
		t.Fatalf("scale: %+v", diff.Scale)
	}
	op := processOpByName(t, diff, "web")
	if op.Op != "change" || op.To == nil || op.To.Command != "serve" {
		t.Fatalf("op: %+v", op)
	}
	foundCmd := false
	for _, f := range op.Fields {
		if f == "command" {
			foundCmd = true
		}
	}
	if !foundCmd {
		t.Fatalf("fields: %v", op.Fields)
	}
}

func TestBuildSnapshotDiffProcessRemoveKeepsScaleToZero(t *testing.T) {
	from := &domain.Release{
		ProcessSnapshot: map[string]domain.ProcessSnapshot{
			"web":    {Quantity: 1, Expose: "http"},
			"worker": {Command: "run-worker", Quantity: 1, Expose: "none"},
		},
	}
	to := &domain.Release{
		ProcessSnapshot: map[string]domain.ProcessSnapshot{
			"web": {Quantity: 1, Expose: "http"},
		},
	}
	diff := BuildSnapshotDiff(from, to)
	foundScale := false
	for _, s := range diff.Scale {
		if s.Process == "worker" && s.To == 0 && s.From != nil && *s.From == 1 {
			foundScale = true
		}
	}
	if !foundScale {
		t.Fatalf("want scale To:0 for worker: %+v", diff.Scale)
	}
	op := processOpByName(t, diff, "worker")
	if op.Op != "remove" {
		t.Fatalf("op: %+v", op)
	}
}

func TestProcessSnapshotEqualityAliases(t *testing.T) {
	from := map[string]domain.ProcessSnapshot{
		"web": {Quantity: 1, Expose: "http"},
	}
	toNone := map[string]domain.ProcessSnapshot{
		"web": {Quantity: 1, Expose: "http", Health: &domain.ProcessHealth{Type: "none"}, TargetExtensions: map[string]json.RawMessage{}},
	}
	if ops := diffProcessSnapshots(from, toNone, nil); len(ops) != 0 {
		t.Fatalf("nil vs type=none/empty map: %+v", ops)
	}
	toEmptyType := map[string]domain.ProcessSnapshot{
		"web": {Quantity: 1, Expose: "http", Health: &domain.ProcessHealth{Type: ""}},
	}
	if ops := diffProcessSnapshots(from, toEmptyType, nil); len(ops) != 0 {
		t.Fatalf("nil vs empty type: %+v", ops)
	}
}

func setupPreviewProject(t *testing.T, name string) (context.Context, *store.Store, *ChangesetService, *ReleaseService) {
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
	ws := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ctx = context.WithValue(ctx, auth.ContextTeamID, ws)
	projectSvc := NewProjectService(st)
	releaseSvc := NewReleaseService(st, projectSvc)
	csSvc := NewChangesetService(st, projectSvc, releaseSvc)
	if err := st.CreateProject(ctx, &domain.Project{WorkspaceID: ws, Name: name}, &domain.Environment{TargetType: "stub"}); err != nil {
		t.Fatal(err)
	}
	return ctx, st, csSvc, releaseSvc
}
