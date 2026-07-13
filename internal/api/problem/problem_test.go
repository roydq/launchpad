package problem

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/launchpad/launchpad/pkg/launchpad"
)

func TestWriteDetailIncludesHints(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteDetail(rec, Detail{
		Status: http.StatusConflict,
		Title:  "Conflict",
		Detail: "deployment already in progress",
		Code:   "deployment_in_progress",
		Hints: []Hint{
			{Action: "wait", Message: "wait", Command: "launchpad deploy --wait"},
		},
	})
	if rec.Code != 409 {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("content-type: %s", ct)
	}
	var d Detail
	if err := json.NewDecoder(rec.Body).Decode(&d); err != nil {
		t.Fatal(err)
	}
	if d.Code != "deployment_in_progress" || len(d.Hints) != 1 {
		t.Fatalf("decoded: %+v", d)
	}
	if d.Hints[0].Command != "launchpad deploy --wait" {
		t.Fatalf("command: %s", d.Hints[0].Command)
	}
}

func TestWriteOmitsEmptyHints(t *testing.T) {
	rec := httptest.NewRecorder()
	Write(rec, http.StatusInternalServerError, "Internal Server Error", "boom", "")
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["hints"]; ok {
		t.Fatalf("expected no hints key, body=%v", m)
	}
	if _, ok := m["code"]; ok {
		t.Fatalf("expected no code key, body=%v", m)
	}
}

func TestHintsForCatalog(t *testing.T) {
	cases := []struct {
		err      error
		wantCode string
		wantCmd  string
	}{
		{
			err:      fmt.Errorf("%w: changeset is pinned to environment %q; current context is %q", launchpad.ErrConflict, "dev", "staging"),
			wantCode: "changeset_env_mismatch",
			wantCmd:  "launchpad diff",
		},
		{
			err:      fmt.Errorf("%w: changeset is empty", launchpad.ErrBadRequest),
			wantCode: "changeset_empty",
			wantCmd:  "launchpad config set KEY=value",
		},
		{
			err:      fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict),
			wantCode: "deployment_in_progress",
			wantCmd:  "launchpad deploy --wait",
		},
		{
			err:      fmt.Errorf("%w: artifact is required", launchpad.ErrBadRequest),
			wantCode: "artifact_required",
			wantCmd:  "launchpad deploy --image <ref>",
		},
		{
			err:      fmt.Errorf("%w: release v1 is not succeeded", launchpad.ErrBadRequest),
			wantCode: "promote_invalid_source",
			wantCmd:  "launchpad releases",
		},
		{
			err:      fmt.Errorf("%w: release v1 was never successfully deployed to staging", launchpad.ErrBadRequest),
			wantCode: "promote_invalid_source",
			wantCmd:  "launchpad releases",
		},
		{
			err:      launchpad.ErrNotFound,
			wantCode: "not_found",
			wantCmd:  "launchpad doctor",
		},
		{
			err:      fmt.Errorf("%w: from and to environments must differ", launchpad.ErrBadRequest),
			wantCode: "promote_same_env",
			wantCmd:  "launchpad promote --from staging --to production",
		},
		{
			err:      fmt.Errorf("%w: no running release in staging; pass version explicitly", launchpad.ErrBadRequest),
			wantCode: "promote_no_running",
			wantCmd:  "launchpad promote --from <env> --release <n>",
		},
	}
	for _, tc := range cases {
		code, hints := HintsFor(tc.err)
		if code != tc.wantCode {
			t.Fatalf("err=%v code=%q want %q", tc.err, code, tc.wantCode)
		}
		if tc.wantCmd != "" {
			if len(hints) == 0 {
				t.Fatalf("err=%v expected hints", tc.err)
			}
			if hints[0].Command != tc.wantCmd {
				t.Fatalf("err=%v first command %q want %q", tc.err, hints[0].Command, tc.wantCmd)
			}
		}
	}
}

func TestHintsForUsesErrorsIsOverSubstring(t *testing.T) {
	// Detail mentions "not found" but sentinel is conflict — must not mis-label.
	err := fmt.Errorf("%w: resource not found in cache but conflict on write", launchpad.ErrConflict)
	code, _ := HintsFor(err)
	if code != "conflict" {
		t.Fatalf("code=%q want conflict (errors.Is wins over substring)", code)
	}
}

func TestTypeURISlug(t *testing.T) {
	if got := typeURI(http.StatusNotFound); got != "https://launchpad.dev/errors/not-found" {
		t.Fatalf("got %s", got)
	}
	if got := typeURI(http.StatusBadRequest); got != "https://launchpad.dev/errors/bad-request" {
		t.Fatalf("got %s", got)
	}
}

func TestWriteErrorUsesCatalog(t *testing.T) {
	rec := httptest.NewRecorder()
	err := fmt.Errorf("%w: deployment already in progress", launchpad.ErrConflict)
	WriteError(rec, err)
	if rec.Code != 409 {
		t.Fatalf("status %d", rec.Code)
	}
	var d Detail
	if err := json.NewDecoder(rec.Body).Decode(&d); err != nil {
		t.Fatal(err)
	}
	if d.Code != "deployment_in_progress" {
		t.Fatalf("code: %s", d.Code)
	}
	if len(d.Hints) == 0 || d.Hints[0].Action != "wait" {
		t.Fatalf("hints: %+v", d.Hints)
	}
}
