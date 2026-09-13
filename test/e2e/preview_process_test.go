//go:build e2e

package e2e

import (
	"context"
	"testing"
)

func TestPreviewPendingProcessSet(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, _, namespace, _ := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := client.StageChanges(ctx, name, []map[string]any{
		{"type": "process.set", "name": "worker", "command": "run-worker"},
	}); err != nil {
		t.Fatalf("stage process.set: %v", err)
	}

	prev, err := client.PreviewPending(ctx, name)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !prev.HasPending {
		t.Fatalf("expected pending: %+v", prev)
	}

	var found bool
	for _, op := range prev.Diff.Process {
		if op.Op == "add" && op.Name == "worker" && op.To != nil && op.To.Command == "run-worker" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("want Diff.Process add worker command=run-worker, got %+v", prev.Diff.Process)
	}
}
