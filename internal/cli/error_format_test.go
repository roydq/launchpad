package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func TestFormatCLIErrorIncludesHints(t *testing.T) {
	err := &apiclient.APIError{
		Method: "POST",
		Path:   "/v1/projects/x/promote",
		Status: 409,
		Detail: "conflict: deployment already in progress",
		Code:   "deployment_in_progress",
		Hints: []apiclient.RecoveryHint{
			{Action: "wait", Message: "A deploy is already running.", Command: "launchpad deploy --wait"},
			{Action: "inspect", Message: "Check status.", Command: "launchpad inspect"},
		},
	}
	out := formatCLIError(err)
	if !strings.Contains(out, "deployment_in_progress") {
		t.Fatalf("missing code in: %s", out)
	}
	if !strings.Contains(out, "recovery:") {
		t.Fatalf("missing recovery section: %s", out)
	}
	if !strings.Contains(out, "launchpad deploy --wait") {
		t.Fatalf("missing first hint command: %s", out)
	}
	if !strings.Contains(out, "launchpad inspect") {
		t.Fatalf("missing second hint: %s", out)
	}
}

func TestFormatCLIErrorPlain(t *testing.T) {
	out := formatCLIError(errors.New("boom"))
	if out != "error: boom\n" {
		t.Fatalf("got %q", out)
	}
}

func TestConfirmSensitiveEnv(t *testing.T) {
	if err := confirmSensitiveEnv("dev", false); err != nil {
		t.Fatalf("dev should not require yes: %v", err)
	}
	if err := confirmSensitiveEnv("production", true); err != nil {
		t.Fatalf("yes should allow production: %v", err)
	}
	if err := confirmSensitiveEnv("production", false); err == nil {
		t.Fatal("expected refuse without --yes")
	}
	if err := confirmSensitiveEnv("prod", false); err == nil {
		t.Fatal("expected refuse for prod alias")
	}
}
