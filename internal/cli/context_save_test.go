package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveActiveContextUpdatesProjectLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := t.TempDir()
	// Chdir so findProjectLocalConfig walks from cwd.
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	// Stale local env=dev after recipe-style bootstrap.
	if err := saveProjectLocalConfig(cwd, localConfig{Project: "app", Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	if err := saveActiveContext("app", "staging"); err != nil {
		t.Fatal(err)
	}

	// Global
	g, err := loadLocalConfig()
	if err != nil {
		t.Fatal(err)
	}
	if g.Project != "app" || g.Environment != "staging" {
		t.Fatalf("global %+v", g)
	}
	// Project-local
	data, err := os.ReadFile(filepath.Join(cwd, ".launchpad", "config"))
	if err != nil {
		t.Fatal(err)
	}
	var pl localConfig
	if err := json.Unmarshal(data, &pl); err != nil {
		t.Fatal(err)
	}
	if pl.Project != "app" || pl.Environment != "staging" {
		t.Fatalf("project-local %+v", pl)
	}
}
