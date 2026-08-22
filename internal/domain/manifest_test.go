package domain

import (
	"errors"
	"strings"
	"testing"

	"github.com/launchpad/launchpad/pkg/launchpad"
)

func goodManifest() *Manifest {
	q := 1
	return &Manifest{
		Version: 1,
		Project: "my-api",
		Processes: map[string]ManifestProcess{
			"web": {Command: "", Quantity: &q, Expose: "http"},
		},
		Environments: map[string]ManifestEnvironment{
			"dev": {
				Target:    "stub",
				Namespace: "default",
				Image:     "hello:v1",
				Config: ManifestConfig{
					Service:    map[string]string{"PORT": "8080"},
					SecretKeys: ManifestSecretKeys{Service: []string{"DATABASE_URL"}},
				},
			},
		},
	}
}

func TestValidateManifestOK(t *testing.T) {
	if err := ValidateManifest(goodManifest()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifestVersion(t *testing.T) {
	m := goodManifest()
	m.Version = 2
	err := ValidateManifest(m)
	if !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("want bad request, got %v", err)
	}
	if err == nil || !contains(err.Error(), "unsupported manifest version") {
		t.Fatalf("phrase: %v", err)
	}
}

func TestValidateManifestMapUnknownAndDeferred(t *testing.T) {
	err := ValidateManifestMap(map[string]any{"version": 1, "project": "my-api", "kind": "Project"})
	if !errors.Is(err, launchpad.ErrBadRequest) || err == nil || !contains(err.Error(), "unknown manifest field") {
		t.Fatalf("unknown: %v", err)
	}
	err = ValidateManifestMap(map[string]any{"version": 1, "project": "my-api", "services": map[string]any{}})
	if err == nil || !contains(err.Error(), "manifest field is deferred") {
		t.Fatalf("services: %v", err)
	}
	err = ValidateManifestMap(map[string]any{"version": 1, "project": "my-api", "bindings": map[string]any{}})
	if err == nil || !contains(err.Error(), "manifest field is deferred") {
		t.Fatalf("bindings: %v", err)
	}
}

func TestValidateManifestSecretOverlap(t *testing.T) {
	m := goodManifest()
	m.Environments["dev"].Config.Service["DATABASE_URL"] = "postgres://x"
	err := ValidateManifest(m)
	if err == nil || !contains(err.Error(), "manifest must not contain a secret value") {
		t.Fatalf("overlap: %v", err)
	}
}

func TestValidateManifestMapSecretObject(t *testing.T) {
	err := ValidateManifestMap(map[string]any{
		"version": 1, "project": "my-api",
		"environments": map[string]any{
			"dev": map[string]any{
				"config": map[string]any{
					"service": map[string]any{
						"DATABASE_URL": map[string]any{"secret": true, "value": "x"},
					},
				},
			},
		},
	})
	if err == nil || !contains(err.Error(), "manifest must not contain a secret value") {
		t.Fatalf("secret object: %v", err)
	}
}

func TestValidateManifestMapBinding(t *testing.T) {
	err := ValidateManifestMap(map[string]any{
		"version": 1, "project": "my-api",
		"environments": map[string]any{
			"dev": map[string]any{
				"config": map[string]any{
					"service": map[string]any{"URL": "${{ services.db.config.URL }}"},
				},
			},
		},
	})
	if err == nil || !contains(err.Error(), "manifest field is deferred") {
		t.Fatalf("binding: %v", err)
	}
}

func TestValidateManifestEmptyProject(t *testing.T) {
	m := goodManifest()
	m.Project = ""
	if err := ValidateManifest(m); !errors.Is(err, launchpad.ErrBadRequest) {
		t.Fatalf("got %v", err)
	}
}

func TestApplyProcessDefaults(t *testing.T) {
	web := ApplyProcessDefaults("web", ManifestProcess{})
	if web.Expose != "http" || web.Quantity == nil || *web.Quantity != 1 {
		t.Fatalf("web defaults %+v qty=%v", web, web.Quantity)
	}
	worker := ApplyProcessDefaults("worker", ManifestProcess{})
	if worker.Expose != "none" {
		t.Fatalf("worker expose %s", worker.Expose)
	}
	zero := 0
	rel := ApplyProcessDefaults("release", ManifestProcess{Quantity: &zero, Expose: "none"})
	if *rel.Quantity != 0 {
		t.Fatalf("quantity 0 must stick, got %d", *rel.Quantity)
	}
}

func TestRequireDevAndSelected(t *testing.T) {
	m := goodManifest()
	if err := RequireDevEnvironment(m); err != nil {
		t.Fatal(err)
	}
	if err := RequireSelectedEnvironment(m, "staging"); err == nil || !contains(err.Error(), "selected environment") {
		t.Fatalf("missing selected: %v", err)
	}
	onlyStaging := goodManifest()
	onlyStaging.Environments = map[string]ManifestEnvironment{"staging": onlyStaging.Environments["dev"]}
	if err := RequireDevEnvironment(onlyStaging); err == nil || !contains(err.Error(), "environments.dev") {
		t.Fatalf("dev required: %v", err)
	}
}

func TestStringifyConfigMaps(t *testing.T) {
	raw := map[string]any{
		"environments": map[string]any{
			"dev": map[string]any{
				"config": map[string]any{
					"service": map[string]any{"PORT": float64(8080), "DEBUG": true},
				},
			},
		},
	}
	StringifyConfigMaps(raw)
	svc := raw["environments"].(map[string]any)["dev"].(map[string]any)["config"].(map[string]any)["service"].(map[string]any)
	if svc["PORT"] != "8080" || svc["DEBUG"] != "true" {
		t.Fatalf("%v", svc)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
