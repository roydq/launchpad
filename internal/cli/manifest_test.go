package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeManifestYAMLStringifyAndUnknown(t *testing.T) {
	doc, err := decodeManifestYAML([]byte(`
version: 1
project: my-api
environments:
  dev:
    target: stub
    config:
      service:
        PORT: 8080
`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Environments["dev"].Config.Service["PORT"] != "8080" {
		t.Fatalf("stringify: %+v", doc.Environments["dev"].Config.Service)
	}

	_, err = decodeManifestYAML([]byte("version: 1\nproject: my-api\nkind: Project\nenvironments:\n  dev:\n    target: stub\n"))
	if err == nil || !strings.Contains(err.Error(), "unknown manifest field") {
		t.Fatalf("unknown: %v", err)
	}

	_, err = decodeManifestYAML([]byte("version: 1\nproject: my-api\nservices: {}\nenvironments:\n  dev:\n    target: stub\n"))
	if err == nil || !strings.Contains(err.Error(), "deferred") {
		t.Fatalf("deferred: %v", err)
	}
}

func TestDefaultApplyPath(t *testing.T) {
	dir := t.TempDir()
	_, err := defaultApplyPath(dir)
	if err == nil {
		t.Fatal("expected missing")
	}
	p := filepath.Join(dir, "launchpad.yaml")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := defaultApplyPath(dir)
	if err != nil || got != p {
		t.Fatalf("got %s %v", got, err)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	in, err := decodeManifestYAML([]byte(`
version: 1
project: my-api
processes:
  web:
    command: serve
    quantity: 1
    expose: http
environments:
  dev:
    target: stub
    image: hello:v1
    config:
      service:
        PORT: "8080"
`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := encodeManifestYAML(in)
	if err != nil {
		t.Fatal(err)
	}
	again, err := decodeManifestYAML(out)
	if err != nil {
		t.Fatal(err)
	}
	if again.Project != "my-api" || again.Environments["dev"].Image != "hello:v1" {
		t.Fatalf("%+v", again)
	}
}
