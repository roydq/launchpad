//go:build e2e

package e2e

import (
	"context"
	"errors"
	"testing"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func TestDeployConflictWhileActive(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, _ := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// First deploy leaves a pending/deploying job; second should conflict.
	if _, err := client.Deploy(ctx, name, image, "first"); err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	_, err := client.Deploy(ctx, name, image+"-2", "second while active")
	if err == nil {
		t.Fatal("expected conflict on second deploy")
	}
	var ae *apiclient.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.Status != 409 {
		t.Fatalf("status %d want 409 detail=%s", ae.Status, ae.Detail)
	}
	if ae.Code != "" && ae.Code != "deployment_in_progress" && ae.Code != "conflict" {
		// Prefer catalog code when present
		t.Logf("code=%q (detail=%s)", ae.Code, ae.Detail)
	}
	if ae.Code == "deployment_in_progress" && len(ae.Hints) == 0 {
		t.Fatal("expected recovery hints for deployment_in_progress")
	}
}

func TestChangesetPinBlocksOtherEnvironment(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, timeout := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := client.CreateEnvironment(ctx, name, "staging", target, namespace, false); err != nil {
		t.Fatalf("staging: %v", err)
	}

	// Quiet deploy so we aren't fighting active deploy on pin test.
	dep, err := client.Deploy(ctx, name, image, "baseline")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	pollJobSucceeded(t, ctx, client, dep.Job.ID, timeout)

	client.Environment = "dev"
	if _, err := client.StageChanges(ctx, name, []map[string]any{
		{"type": "config", "key": "PINNED", "value": "yes"},
	}); err != nil {
		t.Fatalf("stage dev: %v", err)
	}

	client.Environment = "staging"
	_, err = client.StageChanges(ctx, name, []map[string]any{
		{"type": "config", "key": "OTHER", "value": "no"},
	})
	if err == nil {
		t.Fatal("expected pin conflict staging into")
	}
	var ae *apiclient.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	if ae.Status != 409 {
		t.Fatalf("status %d want 409: %s", ae.Status, ae.Detail)
	}
	if ae.Code != "" && ae.Code != "changeset_env_mismatch" && ae.Code != "conflict" {
		t.Logf("code=%q detail=%s", ae.Code, ae.Detail)
	}
}

func TestEmptyPushRejected(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, _, namespace, _ := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := client.PushChangeset(ctx, name, "empty")
	if err == nil {
		t.Fatal("expected empty push to fail")
	}
	var ae *apiclient.APIError
	if !errors.As(err, &ae) {
		t.Fatalf("want *APIError, got %T %v", err, err)
	}
	// Empty / missing open changeset: 404 (no open) or 400 (empty pin).
	if ae.Status != 400 && ae.Status != 404 {
		t.Fatalf("status %d want 400 or 404: %s", ae.Status, ae.Detail)
	}
}

func TestPreviewEmptyPending(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, _, namespace, _ := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create: %v", err)
	}
	prev, err := client.PreviewPending(ctx, name)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if prev.HasPending {
		t.Fatalf("expected no pending: %+v", prev)
	}
	if prev.Summary == "" {
		t.Fatal("expected summary text")
	}
}
