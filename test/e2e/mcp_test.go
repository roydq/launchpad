//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	lpmcp "github.com/launchpad/launchpad/internal/mcp"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func mcpSession(t *testing.T, apiURL, token string) *mcpsdk.ClientSession {
	t.Helper()
	ctx := context.Background()
	srv := lpmcp.NewServer(lpmcp.Config{APIURL: apiURL, Token: token})
	st, ct := mcpsdk.NewInMemoryTransports()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Wait() })
	cl := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "e2e", Version: "v0.0.1"}, nil)
	cs, err := cl.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func mcpCall(t *testing.T, cs *mcpsdk.ClientSession, name string, args map[string]any) *mcpsdk.CallToolResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("tool %s isError: %s", name, mcpText(t, res))
	}
	return res
}

func mcpText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		return ""
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		return ""
	}
	return tc.Text
}

func mcpJSON(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	if res.StructuredContent != nil {
		b, err := json.Marshal(res.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(mcpText(t, res)), &out); err != nil {
		t.Fatalf("json %s: %v", mcpText(t, res), err)
	}
	return out
}

func TestMCPApplyDeployInspect(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "stub" {
		t.Skip("MCP e2e is stub-tier")
	}
	ctx := context.Background()
	apiURL, bootstrap, _, image, _, timeout := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)
	cs := mcpSession(t, apiURL, client.Token)

	name := uniqueProjectName()
	mcpCall(t, cs, "create_project", map[string]any{"name": name, "target": "stub"})

	apply := mcpCall(t, cs, "apply_manifest", map[string]any{
		"project": name,
		"document": map[string]any{
			"version": 1,
			"project": name,
			"environments": map[string]any{
				"dev": map[string]any{
					"target":    "stub",
					"namespace": "default",
					"image":     image,
				},
			},
		},
	})
	applyOut := mcpJSON(t, apply)
	if staged, _ := applyOut["staged"].([]any); len(staged) == 0 {
		t.Fatalf("expected staged image: %v", applyOut)
	}

	prev := mcpCall(t, cs, "preview", map[string]any{"project": name})
	prevOut := mcpJSON(t, prev)
	if prevOut["summary"] == nil && prevOut["diff"] == nil {
		t.Fatalf("preview empty: %v", prevOut)
	}

	rels, err := client.ListReleases(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 0 {
		t.Fatalf("apply must not create a release, got %d", len(rels))
	}

	deploy := mcpCall(t, cs, "deploy", map[string]any{
		"project":          name,
		"timeout_seconds":  int(timeout.Seconds()),
	})
	depOut := mcpJSON(t, deploy)
	wait, _ := depOut["wait"].(map[string]any)
	if wait["status"] != "succeeded" {
		t.Fatalf("deploy wait %v", depOut)
	}

	insp := mcpJSON(t, mcpCall(t, cs, "inspect", map[string]any{"project": name}))
	ld, ok := insp["last_deploy"].(map[string]any)
	if !ok || ld["version"] == nil {
		t.Fatalf("inspect last_deploy %v", insp)
	}
	if ld["version"].(float64) < 1 {
		t.Fatalf("version %v", ld)
	}

	mcpCall(t, cs, "create_environment", map[string]any{"project": name, "name": "staging", "target": "stub"})
	man := mcpJSON(t, mcpCall(t, cs, "get_manifest", map[string]any{"project": name}))
	envs, _ := man["environments"].(map[string]any)
	if _, ok := envs["dev"]; !ok {
		t.Fatalf("manifest missing dev: %v", man)
	}
	if _, ok := envs["staging"]; !ok {
		t.Fatalf("unfiltered get_manifest should include staging: %v", man)
	}
}

func TestMCPApplySecretNeedsValue(t *testing.T) {
	requireE2E(t)
	if envOr("LAUNCHPAD_E2E_TARGET", "stub") != "stub" {
		t.Skip("MCP e2e is stub-tier")
	}
	ctx := context.Background()
	apiURL, bootstrap, _, image, _, _ := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)
	cs := mcpSession(t, apiURL, client.Token)

	name := uniqueProjectName()
	planted := "postgres://hidden-secret"
	yaml := "version: 1\nproject: " + name + "\nenvironments:\n  dev:\n    target: stub\n    namespace: default\n    image: " + image + "\n    config:\n      secret_keys:\n        service:\n          - DATABASE_URL\n"
	res := mcpCall(t, cs, "apply_manifest", map[string]any{"yaml": yaml})
	text := mcpText(t, res)
	out := mcpJSON(t, res)
	needs, _ := out["needs_value"].([]any)
	found := false
	for _, n := range needs {
		if n == "DATABASE_URL" {
			found = true
		}
	}
	if !found {
		t.Fatalf("needs_value %v text %s", out["needs_value"], text)
	}
	if strings.Contains(text, planted) || strings.Contains(text, "postgres://") {
		t.Fatalf("secret material in output: %s", text)
	}
}

func TestMCPStdioListProjects(t *testing.T) {
	requireE2E(t)
	cli := envOr("LAUNCHPAD_E2E_CLI", "./bin/launchpad")
	if _, err := os.Stat(cli); err != nil {
		t.Skipf("CLI binary not found at %s", cli)
	}
	ctx := context.Background()
	apiURL, bootstrap, _, _, _, _ := e2eConfig(t)
	client := newAuthedClient(t, ctx, apiURL, bootstrap)

	cmd := exec.Command(cli, "mcp")
	cmd.Env = append(os.Environ(),
		"LAUNCHPAD_API_URL="+apiURL,
		"LAUNCHPAD_TOKEN="+client.Token,
	)
	transport := &mcpsdk.CommandTransport{Command: cmd}
	cl := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "e2e-stdio", Version: "v0.0.1"}, nil)
	cs, err := cl.Connect(ctx, transport, nil)
	if err != nil {
		t.Fatalf("stdio connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: "list_projects", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("list_projects: %v", err)
	}
	if res.IsError {
		t.Fatalf("list_projects isError: %s", mcpText(t, res))
	}
}
