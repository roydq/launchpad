//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func TestCLIReleasesSmoke(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "stub" {
		t.Skip("CLI smoke is stub-tier only in v1")
	}
	cli := envOr("LAUNCHPAD_E2E_CLI", "./bin/launchpad")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("CLI binary not found at %s", cli)
	}

	ctx := context.Background()
	apiURL, bootstrap, target, image, namespace, timeout := e2eConfig(t)

	boot := apiclient.New(apiURL, bootstrap)
	tok, err := boot.CreateToken(ctx, "e2e-cli", "default", []string{"admin", "project:read", "project:write", "deploy"})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	client := apiclient.New(apiURL, tok.Token)

	name := uniqueProjectName()
	if _, err := client.CreateProject(ctx, name, target, namespace); err != nil {
		t.Fatalf("create project: %v", err)
	}
	result, err := client.Deploy(ctx, name, image, "cli smoke")
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	pollJobSucceeded(t, ctx, client, result.Job.ID, timeout)

	cmd := exec.Command(cli, "releases")
	cmd.Env = append(os.Environ(),
		"LAUNCHPAD_API_URL="+apiURL,
		"LAUNCHPAD_TOKEN="+tok.Token,
		"LAUNCHPAD_PROJECT="+name,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("launchpad releases: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "artifact_ref") && !strings.Contains(string(out), "version") {
		t.Fatalf("unexpected releases output:\n%s", out)
	}
}
