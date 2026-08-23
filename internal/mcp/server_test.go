package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectMCP(t *testing.T, cfg Config) *mcpsdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := NewServer(cfg)
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Wait() })
	cl := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "v0.0.1"}, nil)
	cs, err := cl.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	return res
}

func toolJSON(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatalf("marshal structured: %v", err)
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal structured: %v", err)
		}
		return out
	}
	if len(res.Content) == 0 {
		return map[string]any{}
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content type %T", res.Content[0])
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("unmarshal text %q: %v", tc.Text, err)
	}
	return out
}

func toolErrorText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if !res.IsError {
		t.Fatalf("expected isError, got %#v", res)
	}
	if len(res.Content) == 0 {
		t.Fatal("isError with no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content type %T", res.Content[0])
	}
	return tc.Text
}

func TestRegisteredReadTools(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "t"})
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tool := range list.Tools {
		got[tool.Name] = true
	}
	want := []string{
		"healthz", "list_projects", "get_project", "list_environments", "get_environment",
		"get_config", "list_processes", "list_releases", "get_changeset", "preview",
		"get_manifest", "get_job", "get_logs", "inspect", "target_capabilities",
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool %s", name)
		}
	}
	if got["create_token"] {
		t.Error("create_token must not be registered")
	}
}

func TestHealthzNoToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL})
	res := callTool(t, cs, "healthz", nil)
	if res.IsError {
		t.Fatalf("healthz error: %s", toolErrorText(t, res))
	}
	out := toolJSON(t, res)
	if out["ok"] != true {
		t.Fatalf("ok=%v", out["ok"])
	}
}

func TestMissingTokenListProjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s", r.URL.Path)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL})
	res := callTool(t, cs, "list_projects", nil)
	text := toolErrorText(t, res)
	if !strings.Contains(text, "LAUNCHPAD_TOKEN") {
		t.Fatalf("error %s", text)
	}
	if !strings.Contains(text, "export LAUNCHPAD_TOKEN") {
		t.Fatalf("missing recovery command: %s", text)
	}
}

func TestGetProjectRequiresProject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s", r.URL.Path)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok"})
	res := callTool(t, cs, "get_project", nil)
	text := toolErrorText(t, res)
	if !strings.Contains(text, "project is required") {
		t.Fatalf("error %s", text)
	}
}

func TestGetJobNoProject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/jobs/job-1" {
			t.Errorf("path %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "job-1", "status": "pending"})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok"})
	res := callTool(t, cs, "get_job", map[string]any{"id": "job-1"})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
}

func TestPreviewConflict(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s", r.URL.Path)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "preview", map[string]any{
		"from_release": 1, "to_release": 2, "from_env": "dev", "to_env": "staging",
	})
	text := toolErrorText(t, res)
	if !strings.Contains(text, "not both") {
		t.Fatalf("error %s", text)
	}
}

func TestAPIErrorMapsCodeAndHint(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"title":  "Conflict",
			"detail": "changeset pinned",
			"code":   "changeset_pin",
			"hints":  []map[string]string{{"action": "reset", "command": "launchpad reset"}},
		})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "get_changeset", nil)
	text := toolErrorText(t, res)
	if !strings.Contains(text, "changeset_pin") {
		t.Fatalf("missing code: %s", text)
	}
	if !strings.Contains(text, "launchpad reset") {
		t.Fatalf("missing hint command: %s", text)
	}
}

func TestGetLogsFourOhFourIncludesCode(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"title": "Not Found", "detail": "no logs", "code": "not_found",
		})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "get_logs", nil)
	text := toolErrorText(t, res)
	if !strings.Contains(text, "not_found") {
		t.Fatalf("missing code: %s", text)
	}
}

func TestGetManifestOmitsDefaultEnvFilter(t *testing.T) {
	var gotPath, gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRawQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(map[string]any{
			"version": 1, "project": "p",
			"environments": map[string]any{"dev": map[string]any{}, "staging": map[string]any{}},
		})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p", Environment: "dev"})
	res := callTool(t, cs, "get_manifest", nil)
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if gotPath != "/v1/projects/p/manifest" {
		t.Fatalf("path %s", gotPath)
	}
	if strings.Contains(gotRawQuery, "environment=") {
		t.Fatalf("unexpected query %s", gotRawQuery)
	}
	out := toolJSON(t, res)
	envs, _ := out["environments"].(map[string]any)
	if len(envs) != 2 {
		t.Fatalf("environments=%v", out["environments"])
	}

	res = callTool(t, cs, "get_manifest", map[string]any{"environment": "staging"})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if !strings.Contains(gotRawQuery, "environment=staging") {
		t.Fatalf("expected filter, query %s", gotRawQuery)
	}
}

func TestInspectJSONShape(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects/p") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "p", "status": "ready", "primary_service": "p"})
		case strings.HasSuffix(r.URL.Path, "/changeset"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_ = json.NewEncoder(w).Encode([]any{map[string]any{
				"version": 3, "artifact_ref": "img:v3", "status": "deployed",
				"deployments": []any{map[string]any{"environment": "dev", "status": "running"}},
			}})
		case strings.HasSuffix(r.URL.Path, "/processes"):
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"name": "web", "quantity": 1, "expose": "http"}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "inspect", nil)
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	out := toolJSON(t, res)
	if out["project"] != "p" || out["environment"] != "dev" {
		t.Fatalf("names %v", out)
	}
	if _, ok := out["project"].(string); !ok {
		t.Fatalf("project should be string")
	}
	if out["status"] != "ready" {
		t.Fatalf("status %v", out["status"])
	}
	if out["pending_count"].(float64) != 1 {
		t.Fatalf("pending %v", out["pending_count"])
	}
	ld, ok := out["last_deploy"].(map[string]any)
	if !ok {
		t.Fatalf("last_deploy %T %v", out["last_deploy"], out["last_deploy"])
	}
	if ld["version"].(float64) != 3 || ld["artifact_ref"] != "img:v3" {
		t.Fatalf("last_deploy %v", ld)
	}
}

func TestInspectLastDeployNull(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects/p") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "p", "status": "ready"})
		case strings.HasSuffix(r.URL.Path, "/changeset"):
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]any{"title": "Not Found", "code": "not_found"})
		case strings.HasSuffix(r.URL.Path, "/releases"):
			_ = json.NewEncoder(w).Encode([]any{})
		case strings.HasSuffix(r.URL.Path, "/processes"):
			_ = json.NewEncoder(w).Encode([]any{})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "inspect", nil)
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	out := toolJSON(t, res)
	if out["last_deploy"] != nil {
		t.Fatalf("last_deploy want null, got %v", out["last_deploy"])
	}
}

func TestDefaultEnvIsDevOnAmbientTools(t *testing.T) {
	var envHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		envHeader = r.Header.Get("X-Launchpad-Environment")
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "p", "status": "ready"})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "get_project", nil)
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if envHeader != "dev" {
		t.Fatalf("header %q", envHeader)
	}
}
