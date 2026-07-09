//go:build e2e

package e2e

import (
	"context"
	"testing"
)

func TestHappyPathDeploy(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, timeout := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	project, err := client.CreateProject(ctx, name, target, namespace)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if project.Name != name {
		t.Fatalf("project name: got %q want %q", project.Name, name)
	}

	result, err := client.Deploy(ctx, name, image, "e2e happy path")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if result.Job.ID == "" {
		t.Fatal("deploy returned empty job id")
	}

	pollJobSucceeded(t, ctx, client, result.Job.ID, timeout)

	releases, err := client.ListReleases(ctx, name)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	if len(releases) == 0 {
		t.Fatal("expected at least one release")
	}
	if releases[0].Status != "succeeded" {
		t.Fatalf("expected latest release succeeded, got %s", releases[0].Status)
	}
}
