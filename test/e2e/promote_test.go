//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

// TestPromoteReResolvesTargetConfig is the automated dogfood path for cross-env promote:
// distinct config layers per env → deploy source → promote → target release config
// equals target ResolveConfig, not source snapshot.
func TestPromoteReResolvesTargetConfig(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, timeout := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := client.CreateEnvironment(ctx, name, "staging", target, namespace, false); err != nil {
		t.Fatalf("create staging: %v", err)
	}
	if _, err := client.CreateEnvironment(ctx, name, "production", target, namespace, false); err != nil {
		t.Fatalf("create production: %v", err)
	}

	debug := "debug"
	port3000 := "3000"
	info := "info"
	port8080 := "8080"

	client.Environment = "staging"
	if _, err := client.PatchConfig(ctx, name, map[string]*string{
		"LOG_LEVEL": &debug,
		"PORT":      &port3000,
	}); err != nil {
		t.Fatalf("patch staging config: %v", err)
	}

	client.Environment = "production"
	if _, err := client.PatchConfig(ctx, name, map[string]*string{
		"LOG_LEVEL": &info,
		"PORT":      &port8080,
	}); err != nil {
		t.Fatalf("patch production config: %v", err)
	}

	client.Environment = "staging"
	dep, err := client.Deploy(ctx, name, image, "e2e staging ship")
	if err != nil {
		t.Fatalf("deploy staging: %v", err)
	}
	pollJobSucceeded(t, ctx, client, dep.Job.ID, timeout)

	src := findReleaseByVersion(t, ctx, client, name, 1)
	if src.ArtifactRef != image {
		t.Fatalf("source artifact: got %q want %q", src.ArtifactRef, image)
	}
	if src.ConfigResolved["LOG_LEVEL"] != "debug" || src.ConfigResolved["PORT"] != "3000" {
		t.Fatalf("source config_resolved: %+v", src.ConfigResolved)
	}
	if src.Status != "succeeded" {
		t.Fatalf("source status: %s", src.Status)
	}

	promoted, err := client.Promote(ctx, name, "staging", "production", 1, "e2e promote")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	pollJobSucceeded(t, ctx, client, promoted.Job.ID, timeout)

	client.Environment = "production"
	tgt := findReleaseByVersion(t, ctx, client, name, 2)
	if tgt.ArtifactRef != image {
		t.Fatalf("promoted artifact: got %q want %q", tgt.ArtifactRef, image)
	}
	if tgt.ConfigResolved["LOG_LEVEL"] != "info" || tgt.ConfigResolved["PORT"] != "8080" {
		t.Fatalf("promoted config_resolved not re-resolved to production: %+v", tgt.ConfigResolved)
	}
	if tgt.ConfigResolved["LOG_LEVEL"] == src.ConfigResolved["LOG_LEVEL"] {
		t.Fatal("promoted config must not equal source staging config")
	}
	if tgt.Status != "succeeded" {
		t.Fatalf("promoted status: %s", tgt.Status)
	}
	foundProd := false
	for _, d := range tgt.Deployments {
		if d.Environment == "production" {
			foundProd = true
			break
		}
	}
	if !foundProd {
		t.Fatalf("expected production deployment annotation: %+v", tgt.Deployments)
	}
}

func findReleaseByVersion(t *testing.T, ctx context.Context, client *apiclient.Client, project string, version int) apiclient.Release {
	t.Helper()
	releases, err := client.ListReleases(ctx, project)
	if err != nil {
		t.Fatalf("list releases: %v", err)
	}
	for _, r := range releases {
		if r.Version == version {
			return r
		}
	}
	t.Fatalf("release version %d not found (%d releases)", version, len(releases))
	return apiclient.Release{}
}
