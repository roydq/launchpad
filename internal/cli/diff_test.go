package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/launchpad/launchpad/pkg/apiclient"
)

func mustPayload(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFoldChangesLastWriteWins(t *testing.T) {
	v1, v2 := "1", "2"
	changes := []apiclient.ChangesetChange{
		{Type: "config", Payload: mustPayload(t, map[string]any{"key": "PORT", "value": v1})},
		{Type: "config", Payload: mustPayload(t, map[string]any{"key": "PORT", "value": v2})},
		{Type: "image", Payload: mustPayload(t, map[string]any{"artifact_ref": "img:a"})},
		{Type: "image", Payload: mustPayload(t, map[string]any{"artifact_ref": "img:b"})},
		{Type: "scale", Payload: mustPayload(t, map[string]any{"process": "web", "quantity": 1})},
		{Type: "scale", Payload: mustPayload(t, map[string]any{"process": "web", "quantity": 3})},
	}
	folded, err := foldChanges(changes)
	if err != nil {
		t.Fatal(err)
	}
	if folded.Image != "img:b" {
		t.Fatalf("image: got %q want img:b", folded.Image)
	}
	if folded.Config["PORT"] == nil || *folded.Config["PORT"] != "2" {
		t.Fatalf("PORT: got %v want 2", folded.Config["PORT"])
	}
	if folded.Scales["web"] != 3 {
		t.Fatalf("web scale: got %d want 3", folded.Scales["web"])
	}
}

func TestFormatDiffConfigAddChangeRemove(t *testing.T) {
	nine := "9"
	three := "3"
	pending := FoldedPending{
		Config: map[string]*string{
			"A": &nine,
			"B": nil,
			"C": &three,
		},
	}
	baseline := &apiclient.Release{
		ConfigResolved: map[string]string{"A": "1", "B": "2"},
	}
	out := formatDiff(pending, baseline)
	if !strings.Contains(out, "## Config") {
		t.Fatalf("missing Config section:\n%s", out)
	}
	if !strings.Contains(out, "~ A: 1 → 9") && !strings.Contains(out, "~ A:") {
		t.Fatalf("expected change for A:\n%s", out)
	}
	if !strings.Contains(out, "- B") {
		t.Fatalf("expected remove B:\n%s", out)
	}
	if !strings.Contains(out, "+ C=3") && !strings.Contains(out, "+ C") {
		t.Fatalf("expected add C:\n%s", out)
	}
}

func TestFormatDiffNoPending(t *testing.T) {
	out := formatDiff(FoldedPending{
		Config: map[string]*string{},
		Scales: map[string]int{},
	}, nil)
	if !strings.Contains(out, "No pending changes") {
		t.Fatalf("got %q", out)
	}
}

func TestFormatDiffFirstRelease(t *testing.T) {
	port := "3000"
	pending := FoldedPending{
		Image:  "my-api:v1",
		Config: map[string]*string{"PORT": &port},
		Scales: map[string]int{"web": 2},
	}
	out := formatDiff(pending, nil)
	if !strings.Contains(out, "## Image") {
		t.Fatalf("missing Image:\n%s", out)
	}
	if !strings.Contains(out, "my-api:v1") {
		t.Fatalf("missing image ref:\n%s", out)
	}
	if !strings.Contains(out, "## Config") || !strings.Contains(out, "PORT") {
		t.Fatalf("missing config:\n%s", out)
	}
	if !strings.Contains(out, "## Scale") || !strings.Contains(out, "web") {
		t.Fatalf("missing scale:\n%s", out)
	}
}

func TestReleaseToFoldedAndDiff(t *testing.T) {
	from := &apiclient.Release{
		Version:        1,
		ArtifactRef:    "app:v1",
		ConfigResolved: map[string]string{"PORT": "3000"},
		ProcessSnapshot: map[string]apiclient.ProcessSnapshot{
			"web": {Quantity: 1},
		},
	}
	to := &apiclient.Release{
		Version:        2,
		ArtifactRef:    "app:v2",
		ConfigResolved: map[string]string{"PORT": "8080"},
		ProcessSnapshot: map[string]apiclient.ProcessSnapshot{
			"web": {Quantity: 2},
		},
	}
	out := formatDiff(releaseToFolded(to), from)
	if !strings.Contains(out, "## Image") || !strings.Contains(out, "app:v2") {
		t.Fatalf("image: %s", out)
	}
	if !strings.Contains(out, "PORT") {
		t.Fatalf("config: %s", out)
	}
	if !strings.Contains(out, "## Scale") {
		t.Fatalf("scale: %s", out)
	}
}

func TestFormatDiffNoopVsBaseline(t *testing.T) {
	port := "3000"
	pending := FoldedPending{
		Image:  "img:v1",
		Config: map[string]*string{"PORT": &port},
		Scales: map[string]int{"web": 1},
	}
	baseline := &apiclient.Release{
		ArtifactRef:    "img:v1",
		ConfigResolved: map[string]string{"PORT": "3000"},
		ProcessSnapshot: map[string]apiclient.ProcessSnapshot{
			"web": {Quantity: 1},
		},
	}
	out := formatDiff(pending, baseline)
	if !strings.Contains(out, "no effective delta") && !strings.Contains(out, "No effective") {
		t.Fatalf("expected no-op message:\n%s", out)
	}
}
