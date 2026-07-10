//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func requireE2E(t *testing.T) {
	t.Helper()
	if os.Getenv("LAUNCHPAD_E2E") != "1" {
		t.Skip("set LAUNCHPAD_E2E=1 to run e2e tests (use make e2e-stub or make e2e-kind)")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func e2eConfig(t *testing.T) (apiURL, bootstrap, target, image, namespace string, timeout time.Duration) {
	t.Helper()
	apiURL = envOr("LAUNCHPAD_API_URL", "http://127.0.0.1:18080")
	bootstrap = os.Getenv("LAUNCHPAD_BOOTSTRAP_TOKEN")
	if bootstrap == "" {
		t.Fatal("LAUNCHPAD_BOOTSTRAP_TOKEN is required")
	}
	target = envOr("LAUNCHPAD_E2E_TARGET", "stub")
	image = envOr("LAUNCHPAD_E2E_IMAGE", "nginx:stable")
	if target == "stub" {
		image = envOr("LAUNCHPAD_E2E_IMAGE", "e2e:stub")
	}
	namespace = envOr("LAUNCHPAD_E2E_NAMESPACE", "default")
	timeout = 30 * time.Second
	if target == "kubernetes" {
		timeout = 3 * time.Minute
	}
	if v := os.Getenv("LAUNCHPAD_E2E_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			t.Fatalf("LAUNCHPAD_E2E_TIMEOUT: %v", err)
		}
		timeout = d
	}
	return apiURL, bootstrap, target, image, namespace, timeout
}

func uniqueProjectName() string {
	return fmt.Sprintf("e2e-%d", time.Now().UnixNano()%1_000_000_000)
}

func newAuthedClient(t *testing.T, ctx context.Context, apiURL, bootstrap string) *apiclient.Client {
	t.Helper()
	boot := apiclient.New(apiURL, bootstrap)
	if err := boot.Healthz(ctx); err != nil {
		t.Fatalf("healthz: %v", err)
	}
	tok, err := boot.CreateToken(ctx, "e2e", "default", []string{"admin", "project:read", "project:write", "deploy"})
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	return apiclient.New(apiURL, tok.Token)
}

func pollJobSucceeded(t *testing.T, ctx context.Context, client *apiclient.Client, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		job, err := client.GetJob(ctx, jobID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		t.Logf("job %s status=%s", jobID, job.Status)
		switch job.Status {
		case "succeeded":
			return
		case "failed", "dead":
			t.Fatalf("job terminal failure: status=%s last_error=%s", job.Status, job.LastError)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("job %s did not succeed within %s", jobID, timeout)
}
