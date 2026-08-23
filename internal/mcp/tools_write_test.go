package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestExactToolSet(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "t"})
	list, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"healthz": true, "list_projects": true, "get_project": true, "list_environments": true,
		"get_environment": true, "get_config": true, "list_processes": true, "list_releases": true,
		"get_changeset": true, "preview": true, "get_manifest": true, "get_job": true, "get_logs": true,
		"inspect": true, "target_capabilities": true, "create_project": true, "create_environment": true,
		"clone_environment": true, "apply_manifest": true, "stage_config": true, "stage_process": true,
		"stage_image": true, "stage_scale": true, "unstage_last": true, "discard_changeset": true,
		"deploy": true, "rollback": true, "promote": true,
	}
	got := map[string]bool{}
	for _, tool := range list.Tools {
		got[tool.Name] = true
		if !want[tool.Name] {
			t.Errorf("unexpected tool %s", tool.Name)
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("missing tool %s", name)
		}
	}
	if got["create_token"] {
		t.Error("create_token must not be registered")
	}
}

func TestApplyManifestXorAndNoPush(t *testing.T) {
	var hitPush, hitReleases bool
	var applyBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/manifest/apply"):
			applyBody = body
			_ = json.NewEncoder(w).Encode(map[string]any{"project": "p", "environment": "dev", "staged": []string{"image"}})
		case strings.Contains(r.URL.Path, "/changeset/push"):
			hitPush = true
			w.WriteHeader(http.StatusInternalServerError)
		case strings.Contains(r.URL.Path, "/releases"):
			hitReleases = true
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})

	both := callTool(t, cs, "apply_manifest", map[string]any{
		"document": map[string]any{"version": 1, "project": "p"},
		"yaml":     "version: 1\nproject: p\n",
	})
	if text := toolErrorText(t, both); !strings.Contains(text, "exactly one") {
		t.Fatalf("xor: %s", text)
	}

	res := callTool(t, cs, "apply_manifest", map[string]any{
		"yaml": "version: 1\nproject: p\nenvironments:\n  dev:\n    config:\n      service:\n        PORT: 8080\n",
	})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if hitPush || hitReleases {
		t.Fatalf("apply must not push/releases")
	}
	if !strings.Contains(string(applyBody), `"document"`) {
		t.Fatalf("apply body %s", applyBody)
	}
}

func TestCreateProjectNoContext(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/projects" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "newp", "status": "ready"})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok"})
	res := callTool(t, cs, "create_project", map[string]any{"name": "newp"})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
}

func TestDeploySensitiveEnvRequiresConfirm(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("must not hit API: %s", r.URL.Path)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p", Environment: "production"})
	res := callTool(t, cs, "deploy", map[string]any{"image": "img:v1"})
	text := toolErrorText(t, res)
	if !strings.Contains(text, "sensitive environment") {
		t.Fatalf("%s", text)
	}
}

func TestDeploySensitiveEnvConfirmReachesBackend(t *testing.T) {
	var hitPush bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/changeset/push"):
			hitPush = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{"id": "d1", "status": "pending", "release": map[string]any{"version": 1}},
				"job":        map[string]any{"id": "j1", "status": "pending"},
			})
		case strings.Contains(r.URL.Path, "/changeset"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p", Environment: "production"})
	res := callTool(t, cs, "deploy", map[string]any{"image": "img:v1", "confirm": true, "wait": false})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if !hitPush {
		t.Fatal("confirm=true must reach push")
	}
}

func TestDeployWaitOmitDefaultTrue(t *testing.T) {
	var jobGets atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/changeset/changes") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		case strings.HasSuffix(r.URL.Path, "/changeset") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		case strings.HasSuffix(r.URL.Path, "/changeset/push"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{"id": "d1", "status": "pending", "release": map[string]any{"version": 1}},
				"job":        map[string]any{"id": "j1", "type": "deploy", "status": "pending"},
			})
		case r.URL.Path == "/v1/jobs/j1":
			n := jobGets.Add(1)
			status := "pending"
			if n >= 2 {
				status = "succeeded"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j1", "type": "deploy", "status": status})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	// Omit wait entirely — default true.
	res := callTool(t, cs, "deploy", map[string]any{"image": "img:v1"})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	out := toolJSON(t, res)
	wait, _ := out["wait"].(map[string]any)
	if wait["status"] != "succeeded" {
		t.Fatalf("wait %v", out)
	}
	if jobGets.Load() < 2 {
		t.Fatalf("expected poll, gets=%d", jobGets.Load())
	}
}

func TestDeployWaitFalseSkipsPoll(t *testing.T) {
	var jobGets atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/changeset/push"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{"id": "d1", "status": "pending", "release": map[string]any{"version": 1}},
				"job":        map[string]any{"id": "j1", "status": "pending"},
			})
		case strings.Contains(r.URL.Path, "/changeset"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		case strings.HasPrefix(r.URL.Path, "/v1/jobs/"):
			jobGets.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j1", "status": "pending"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	waitFalse := false
	res := callTool(t, cs, "deploy", map[string]any{"image": "img:v1", "wait": waitFalse})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if jobGets.Load() != 0 {
		t.Fatalf("GetJob called %d times", jobGets.Load())
	}
}

func TestDeployJobFailed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/changeset/push"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{"id": "d1", "status": "pending", "release": map[string]any{"version": 1}},
				"job":        map[string]any{"id": "j1", "status": "pending"},
			})
		case strings.Contains(r.URL.Path, "/changeset"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		case r.URL.Path == "/v1/jobs/j1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j1", "status": "failed", "last_error": "boom"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "deploy", map[string]any{"image": "img:v1"})
	text := toolErrorText(t, res)
	if !strings.Contains(text, "j1") {
		t.Fatalf("missing job_id: %s", text)
	}
}

func TestDeployWaitTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/changeset/push"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"deployment": map[string]any{"id": "d1", "status": "pending", "release": map[string]any{"version": 1}},
				"job":        map[string]any{"id": "j1", "status": "pending"},
			})
		case strings.Contains(r.URL.Path, "/changeset"):
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{map[string]any{"id": "1"}}})
		case r.URL.Path == "/v1/jobs/j1":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "j1", "status": "pending"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "deploy", map[string]any{"image": "img:v1", "timeout_seconds": 1})
	text := toolErrorText(t, res)
	if !strings.Contains(text, "j1") || !strings.Contains(text, "pending") {
		t.Fatalf("timeout error %s", text)
	}
}

func TestStageConfigOmitsSensitivityWhenNotSecret(t *testing.T) {
	var body []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "cs", "changes": []any{}})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "stage_config", map[string]any{"set": map[string]any{"PORT": "8080"}})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if strings.Contains(string(body), "sensitivity") {
		t.Fatalf("should omit sensitivity: %s", body)
	}
}

func TestSecretConfigNotEchoedInChangesetTools(t *testing.T) {
	planted := "super-secret-db-url"
	secretCS := map[string]any{
		"id": "cs",
		"changes": []any{map[string]any{
			"id":   "1",
			"type": "config",
			"payload": map[string]any{
				"key":          "DATABASE_URL",
				"value":        planted,
				"sensitivity":  "secret",
			},
		}},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(secretCS)
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	stage := callTool(t, cs, "stage_config", map[string]any{
		"set":    map[string]any{"DATABASE_URL": planted},
		"secret": true,
	})
	if stage.IsError {
		t.Fatalf("stage: %s", toolErrorText(t, stage))
	}
	got := callTool(t, cs, "get_changeset", nil)
	if got.IsError {
		t.Fatalf("get_changeset: %s", toolErrorText(t, got))
	}
	body := mcpDump(t, stage) + mcpDump(t, got)
	if strings.Contains(body, planted) {
		t.Fatalf("planted secret leaked: %s", body)
	}
	out := toolJSON(t, got)
	changes, _ := out["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("changes %v", out)
	}
	ch := changes[0].(map[string]any)
	payload := ch["payload"].(map[string]any)
	if payload["value"] != "***" {
		t.Fatalf("payload %v", payload)
	}
}

func mcpDump(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	b, _ := json.Marshal(toolJSON(t, res))
	return string(b)
}

func TestEnvironmentTargetConfigIsObject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "e1",
			"name":          "dev",
			"target_type":   "stub",
			"target_config": map[string]any{"namespace": "default", "cluster": ""},
			"ephemeral":     false,
		})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "get_environment", map[string]any{"name": "dev"})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	out := toolJSON(t, res)
	tc, ok := out["target_config"].(map[string]any)
	if !ok {
		t.Fatalf("target_config want object, got %T %v", out["target_config"], out["target_config"])
	}
	if tc["namespace"] != "default" {
		t.Fatalf("target_config %v", tc)
	}
}

func TestStageConfigPayloadIsObject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cs",
			"changes": []any{map[string]any{
				"id":   "1",
				"type": "config",
				"payload": map[string]any{
					"key":   "PORT",
					"value": "8080",
				},
			}},
		})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "stage_config", map[string]any{"set": map[string]any{"PORT": "8080"}})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	out := toolJSON(t, res)
	changes, _ := out["changes"].([]any)
	payload, ok := changes[0].(map[string]any)["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload want object: %v", out)
	}
	if payload["key"] != "PORT" {
		t.Fatalf("payload %v", payload)
	}
}

func TestCloneOmitsTargetByDefault(t *testing.T) {
	var body []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"environment": map[string]any{"name": "staging", "target_type": "stub"},
			"from":        "dev",
		})
	}))
	t.Cleanup(ts.Close)
	cs := connectMCP(t, Config{APIURL: ts.URL, Token: "tok", Project: "p"})
	res := callTool(t, cs, "clone_environment", map[string]any{"from": "dev", "name": "staging"})
	if res.IsError {
		t.Fatalf("%s", toolErrorText(t, res))
	}
	if strings.Contains(string(body), `"target"`) {
		t.Fatalf("clone must not default target: %s", body)
	}
}
