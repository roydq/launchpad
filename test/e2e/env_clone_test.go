//go:build e2e

package e2e

import (
	"context"
	"testing"
)

// TestEnvClonePlainAndSecrets: plain config is copied; secrets appear only in needs_value.
func TestEnvClonePlainAndSecrets(t *testing.T) {
	requireE2E(t)
	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, timeout := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create project: %v", err)
	}
	client.Environment = "dev"

	// Stage plain + secret config and image, then push.
	sec := "secret"
	plain := "plain"
	if _, err := client.StageChanges(ctx, name, []map[string]any{
		{"type": "config", "key": "PORT", "value": "8080", "sensitivity": plain},
		{"type": "config", "key": "DATABASE_URL", "value": "postgres://hidden", "sensitivity": sec},
		{"type": "image", "image": image},
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	dep, err := client.PushChangeset(ctx, name, "e2e clone seed")
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	pollJobSucceeded(t, ctx, client, dep.Job.ID, timeout)

	result, err := client.CloneEnvironment(ctx, name, "dev", "staging", "", "", false)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if result.Environment.Name != "staging" {
		t.Fatalf("name %s", result.Environment.Name)
	}
	if result.From != "dev" {
		t.Fatalf("from %s", result.From)
	}
	foundPort := false
	for _, k := range result.ClonedPlain {
		if k == "PORT" {
			foundPort = true
		}
		if k == "DATABASE_URL" {
			t.Fatal("DATABASE_URL must not appear in cloned_plain")
		}
	}
	if !foundPort {
		t.Fatalf("expected PORT in cloned_plain: %+v", result.ClonedPlain)
	}
	foundSecret := false
	for _, k := range result.NeedsValue {
		if k == "DATABASE_URL" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Fatalf("expected DATABASE_URL in needs_value: %+v", result.NeedsValue)
	}

	// Staging config has PORT, not real secret material for DATABASE_URL.
	client.Environment = "staging"
	cfg, err := client.GetConfig(ctx, name)
	if err != nil {
		t.Fatalf("get staging config: %v", err)
	}
	if cfg["PORT"] != "8080" {
		t.Fatalf("PORT on staging: %+v", cfg)
	}
	if v, ok := cfg["DATABASE_URL"]; ok && v != "" && v != "***" && v != "[secret]" {
		// Redacted sentinel is ok if placeholder exists; plaintext is not.
		if v == "postgres://hidden" {
			t.Fatalf("secret material leaked to staging: %q", v)
		}
	}
}
