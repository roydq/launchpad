package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// expectedOpenAPIPaths must stay in sync with Server.Routes() in handlers.go.
// When you add a route, update docs/openapi.yaml and this list.
var expectedOpenAPIPaths = []string{
	"/healthz",
	"/v1/tokens",
	"/v1/audit",
	"/v1/jobs/{id}",
	"/v1/projects",
	"/v1/projects/{project}",
	"/v1/projects/{project}/config",
	"/v1/projects/{project}/processes",
	"/v1/projects/{project}/logs",
	"/v1/projects/{project}/environments",
	"/v1/projects/{project}/environments/{name}",
	"/v1/projects/{project}/releases",
	"/v1/projects/{project}/rollback",
	"/v1/projects/{project}/promote",
	"/v1/projects/{project}/changeset",
	"/v1/projects/{project}/changeset/changes",
	"/v1/projects/{project}/changeset/changes/last",
	"/v1/projects/{project}/changeset/push",
	"/v1/projects/{project}/preview",
}

func TestOpenAPIContractCoversRoutes(t *testing.T) {
	docPath := openAPIPath(t)
	data, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read openapi: %v", err)
	}
	var doc struct {
		OpenAPI string                 `yaml:"openapi"`
		Paths   map[string]interface{} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi yaml: %v", err)
	}
	if !strings.HasPrefix(doc.OpenAPI, "3.") {
		t.Fatalf("openapi version %q want 3.x", doc.OpenAPI)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("openapi paths empty")
	}
	for _, p := range expectedOpenAPIPaths {
		if _, ok := doc.Paths[p]; !ok {
			t.Errorf("docs/openapi.yaml missing path %s (update contract when adding routes)", p)
		}
	}
	// Fail if openapi has unknown paths not in the allowlist (catch typos / dead docs).
	allowed := map[string]struct{}{}
	for _, p := range expectedOpenAPIPaths {
		allowed[p] = struct{}{}
	}
	for p := range doc.Paths {
		if _, ok := allowed[p]; !ok {
			t.Errorf("docs/openapi.yaml has unexpected path %s (add to expectedOpenAPIPaths or remove)", p)
		}
	}
}

func TestHandlersRoutesMentionedInOpenAPIList(t *testing.T) {
	// Structural: handlers.go should mention each path segment we care about.
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	handlers := filepath.Join(filepath.Dir(file), "handlers.go")
	src, err := os.ReadFile(handlers)
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	needles := []string{
		`"/tokens"`,
		`"/audit"`,
		`"/jobs/{id}"`,
		`"/projects"`,
		`"/projects/{project}"`,
		`"/projects/{project}/config"`,
		`"/projects/{project}/processes"`,
		`"/projects/{project}/logs"`,
		`"/projects/{project}/environments"`,
		`"/projects/{project}/environments/{name}"`,
		`"/projects/{project}/releases"`,
		`"/projects/{project}/rollback"`,
		`"/projects/{project}/promote"`,
		`"/projects/{project}/changeset"`,
		`"/projects/{project}/changeset/changes"`,
		`"/projects/{project}/changeset/changes/last"`,
		`"/projects/{project}/changeset/push"`,
		`"/projects/{project}/preview"`,
		`"/healthz"`,
	}
	for _, n := range needles {
		if !strings.Contains(body, n) {
			t.Errorf("handlers.go missing route string %s", n)
		}
	}
}

func openAPIPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	// internal/api -> repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	return filepath.Join(root, "docs", "openapi.yaml")
}
