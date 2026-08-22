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

func TestCLIManifestExportApply(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "stub" {
		t.Skip("manifest yaml smoke is stub-tier only")
	}
	cli := envOr("LAUNCHPAD_E2E_CLI", "./bin/launchpad")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("CLI binary not found at %s", cli)
	}

	ctx := context.Background()
	apiURL, bootstrap, _, _, _, timeout := e2eConfig(t)
	boot := apiclient.New(apiURL, bootstrap)
	tok, err := boot.CreateToken(ctx, "e2e-yaml", "default", []string{"admin", "project:read", "project:write", "deploy"})
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	dir := t.TempDir()
	src := uniqueProjectName()
	dst := uniqueProjectName()
	env := append(os.Environ(),
		"LAUNCHPAD_API_URL="+apiURL,
		"LAUNCHPAD_TOKEN="+tok.Token,
		"HOME="+dir,
	)

	create := exec.Command(cli, "new", "web-stub", src, "--dir", dir)
	create.Env = env
	create.Dir = dir
	if out, err := create.CombinedOutput(); err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}

	deploySrc := exec.Command(cli, "deploy", "--wait", "--timeout", timeout.String())
	deploySrc.Env = append(env, "LAUNCHPAD_PROJECT="+src)
	deploySrc.Dir = dir
	if out, err := deploySrc.CombinedOutput(); err != nil {
		t.Fatalf("deploy src: %v\n%s", err, out)
	}

	mf := filepath.Join(dir, "launchpad.yaml")
	export := exec.Command(cli, "export", "-f", mf, "--force")
	export.Env = append(env, "LAUNCHPAD_PROJECT="+src)
	export.Dir = dir
	if out, err := export.CombinedOutput(); err != nil {
		t.Fatalf("export: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(mf)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "postgres://") {
		t.Fatalf("secret plaintext in yaml:\n%s", body)
	}
	if !strings.Contains(body, "image:") {
		t.Fatalf("expected image after deploy:\n%s", body)
	}

	rewritten := strings.Replace(body, "project: "+src, "project: "+dst, 1)
	if rewritten == body {
		t.Fatalf("did not rewrite project name in:\n%s", body)
	}
	dstFile := filepath.Join(dir, "dst.yaml")
	if err := os.WriteFile(dstFile, []byte(rewritten), 0o600); err != nil {
		t.Fatal(err)
	}

	apply := exec.Command(cli, "apply", "-f", dstFile)
	apply.Env = env
	apply.Dir = dir
	applyOut, err := apply.CombinedOutput()
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, applyOut)
	}
	if !strings.Contains(string(applyOut), "created project") {
		t.Fatalf("expected created project:\n%s", applyOut)
	}
	if !strings.Contains(string(applyOut), "image") {
		t.Fatalf("expected image staged:\n%s", applyOut)
	}
	dstClient := apiclient.New(apiURL, tok.Token)
	dstClient.Environment = "dev"
	rels, err := dstClient.ListReleases(ctx, dst)
	if err != nil {
		t.Fatalf("list dest releases after apply: %v", err)
	}
	if len(rels) != 0 {
		t.Fatalf("apply must not create a release; got %d", len(rels))
	}

	diff := exec.Command(cli, "diff")
	diff.Env = append(env, "LAUNCHPAD_PROJECT="+dst)
	diff.Dir = dir
	diffOut, err := diff.CombinedOutput()
	if err != nil {
		t.Fatalf("diff: %v\n%s", err, diffOut)
	}
	if strings.TrimSpace(string(diffOut)) == "" || strings.Contains(string(diffOut), "no pending") {
		t.Fatalf("expected pending diff:\n%s", diffOut)
	}

	deployDst := exec.Command(cli, "deploy", "--wait", "--timeout", timeout.String())
	deployDst.Env = append(env, "LAUNCHPAD_PROJECT="+dst)
	deployDst.Dir = dir
	if out, err := deployDst.CombinedOutput(); err != nil {
		t.Fatalf("deploy dst: %v\n%s", err, out)
	} else if !strings.Contains(string(out), "succeeded") {
		t.Fatalf("expected dest deploy succeeded:\n%s", out)
	}

	exportDst := exec.Command(cli, "export", "--stdout")
	exportDst.Env = append(env, "LAUNCHPAD_PROJECT="+dst)
	exportDst.Dir = dir
	out, err := exportDst.CombinedOutput()
	if err != nil {
		t.Fatalf("export dst: %v\n%s", err, out)
	}
	got := string(out)
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "image:") && !strings.Contains(got, trim) {
			t.Fatalf("dst export missing %q:\n%s", trim, got)
		}
	}
	if strings.Contains(body, "8080") && !strings.Contains(got, "8080") {
		t.Fatalf("dst export missing PORT from src:\n%s", got)
	}
	if !strings.Contains(got, "stub") {
		t.Fatalf("dst export missing stub target:\n%s", got)
	}
}

func TestCLIManifestSecretKeysNeedsValue(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "stub" {
		t.Skip("manifest yaml smoke is stub-tier only")
	}
	cli := envOr("LAUNCHPAD_E2E_CLI", "./bin/launchpad")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("CLI binary not found at %s", cli)
	}

	ctx := context.Background()
	apiURL, bootstrap, _, _, _, _ := e2eConfig(t)
	boot := apiclient.New(apiURL, bootstrap)
	tok, err := boot.CreateToken(ctx, "e2e-yaml-secret", "default", []string{"admin", "project:read", "project:write", "deploy"})
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	dir := t.TempDir()
	name := uniqueProjectName()
	body := "version: 1\nproject: " + name + "\nenvironments:\n  dev:\n    target: stub\n    namespace: default\n    image: hello:v1\n    config:\n      secret_keys:\n        service:\n          - DATABASE_URL\n"
	path := filepath.Join(dir, "launchpad.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "postgres://") {
		t.Fatal("fixture leaked")
	}

	env := append(os.Environ(),
		"LAUNCHPAD_API_URL="+apiURL,
		"LAUNCHPAD_TOKEN="+tok.Token,
		"HOME="+dir,
	)
	apply := exec.Command(cli, "apply", "-f", path)
	apply.Env = env
	apply.Dir = dir
	out, err := apply.CombinedOutput()
	if err != nil {
		t.Fatalf("apply: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "DATABASE_URL") {
		t.Fatalf("expected needs_value DATABASE_URL:\n%s", out)
	}
	if strings.Contains(string(out), "postgres://") {
		t.Fatalf("secret material in CLI output:\n%s", out)
	}
}
