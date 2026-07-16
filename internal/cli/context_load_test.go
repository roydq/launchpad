package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFindProjectLocalConfig(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	lpDir := filepath.Join(root, "a", ".launchpad")
	if err := os.MkdirAll(lpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	want := localConfig{Project: "demo", Environment: "staging"}
	data, _ := json.Marshal(want)
	if err := os.WriteFile(filepath.Join(lpDir, "config"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, path, err := findProjectLocalConfig(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "demo" || got.Environment != "staging" {
		t.Fatalf("got %+v", got)
	}
	if path == "" {
		t.Fatal("expected path")
	}

	// Outside tree: no file
	empty, p, err := findProjectLocalConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if empty.Project != "" || p != "" {
		t.Fatalf("expected empty, got %+v path=%q", empty, p)
	}
}

func TestFindProjectLocalConfigSkipsHomeConfig(t *testing.T) {
	// When cwd is under $HOME, walk-up must not treat ~/.launchpad/config as project-local.
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Force UserHomeDir to use HOME on systems that read it.
	nested := filepath.Join(home, "work", "repo")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	lpDir := filepath.Join(home, ".launchpad")
	if err := os.MkdirAll(lpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(localConfig{Project: "from-home"})
	if err := os.WriteFile(filepath.Join(lpDir, "config"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	got, path, err := findProjectLocalConfig(nested)
	if err != nil {
		t.Fatal(err)
	}
	if got.Project != "" || path != "" {
		t.Fatalf("home config must not be project-local: got %+v path=%q", got, path)
	}
}

func TestMergeConfigLayersPrecedence(t *testing.T) {
	cfg := mergeConfigLayers(
		localConfig{Project: "global-proj", Environment: "dev"},
		localConfig{Project: "local-proj", Environment: "staging"},
		"",
		"",
	)
	if cfg.Project != "local-proj" || cfg.Environment != "staging" {
		t.Fatalf("local should win: %+v", cfg)
	}
	cfg = mergeConfigLayers(
		localConfig{Project: "global-proj", Environment: "dev"},
		localConfig{Project: "local-proj", Environment: "staging"},
		"env-proj",
		"prod",
	)
	if cfg.Project != "env-proj" || cfg.Environment != "prod" {
		t.Fatalf("env vars should win: %+v", cfg)
	}
}
