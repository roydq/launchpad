//go:build e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

// TestCLIRecipesNew exercises launchpad new web-stub day-one path.
func TestCLIRecipesNew(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "stub" {
		t.Skip("recipe smoke is stub-tier only")
	}
	cli := envOr("LAUNCHPAD_E2E_CLI", "./bin/launchpad")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("CLI binary not found at %s", cli)
	}

	ctx := context.Background()
	apiURL, bootstrap, _, _, _, timeout := e2eConfig(t)
	boot := apiclient.New(apiURL, bootstrap)
	tok, err := boot.CreateToken(ctx, "e2e-recipe", "default", []string{"admin", "project:read", "project:write", "deploy"})
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	dir := t.TempDir()
	name := uniqueProjectName()
	env := append(os.Environ(),
		"LAUNCHPAD_API_URL="+apiURL,
		"LAUNCHPAD_TOKEN="+tok.Token,
		"HOME="+dir,
	)

	list := exec.Command(cli, "new", "list")
	list.Env = env
	out, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("new list: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "hello-stub") || !strings.Contains(string(out), "web-stub") {
		t.Fatalf("new list output:\n%s", out)
	}

	create := exec.Command(cli, "new", "web-stub", name, "--dir", dir)
	create.Env = env
	create.Dir = dir
	out, err = create.CombinedOutput()
	if err != nil {
		t.Fatalf("new web-stub: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "created project") {
		t.Fatalf("unexpected new output:\n%s", out)
	}

	if _, err := os.Stat(filepath.Join(dir, ".launchpad", "config")); err != nil {
		t.Fatalf("project-local config: %v", err)
	}

	deploy := exec.Command(cli, "deploy", "--wait", "--timeout", timeout.String())
	deploy.Env = append(env, "LAUNCHPAD_PROJECT="+name)
	deploy.Dir = dir
	out, err = deploy.CombinedOutput()
	if err != nil {
		t.Fatalf("deploy: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "succeeded") {
		t.Fatalf("expected deploy succeeded:\n%s", out)
	}
}
