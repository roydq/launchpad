# Launchpad MCP server Implementation Plan

> **Status: In Progress** — branch `feat/mcp-server`, started 2026-08-23

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md` and `docs/superpowers/specs/2026-08-23-mcp-server-design.md`. Use `/launchpad-dev` for verification (`mise exec --`). Commit after each task with the message specified below. Worktree: `.worktrees/feat-mcp-server`. Do not edit other agents' branches or `main`. Verify from repo root with `mise exec -- go test -C .worktrees/feat-mcp-server …`. Do not `cd` into the worktree without `mise trust` first.

**Goal:** Ship `launchpad mcp` — stdio MCP tools over existing OpenAPI/apiclient, including apply_manifest and deploy-wait.

**Architecture:** `internal/mcp` builds an official go-sdk server; tools call `pkg/apiclient`; cobra `mcp` runs StdioTransport. No new REST, store, or domain types.

**Tech Stack:** Go 1.26, cobra, `github.com/modelcontextprotocol/go-sdk v1.7.0`, existing apiclient, yaml.v3 (CLI already uses it)

**Spec:** `docs/superpowers/specs/2026-08-23-mcp-server-design.md`

**Branch:** `feat/mcp-server`

**DoD:** spec success criteria 1–8; L0; L1 `make e2e-stub`; no OpenAPI change; docs sync; QUEUE `pr-open`.

---

## Task 0 (orchestrator, before Task 1)

- [x] Spec + this plan committed
- [x] QUEUE status `designing`, Branch `feat/mcp-server`
- [ ] Commit: `docs: add MCP server spec and implementation plan`

---

## Task 1: MCP package — server, context, errors, read tools

**Files:**
- Create: `internal/mcp/config.go`, `internal/mcp/errors.go`, `internal/mcp/server.go`, `internal/mcp/tools_read.go`, `internal/mcp/wait.go`
- Create: `internal/mcp/server_test.go`, `internal/mcp/tools_read_test.go`
- Modify: `go.mod` / `go.sum` via `mise exec -- go get github.com/modelcontextprotocol/go-sdk@v1.7.0`

**Allowed paths:** `internal/mcp/**`, `go.mod`, `go.sum`

- [ ] Add `mcp.Config` with `APIURL`, `Token`, `Project`, `Environment`. Helpers: `ResolveProject(explicit string) (string, error)`, `ResolveEnv(explicit string) string` (default `dev`), `RequireToken() error`, `IsSensitiveEnv(name string) bool` (`prod`/`production`, case-insensitive), `Client() *apiclient.Client` setting `Environment` header from resolved env for the upcoming call (handlers set `client.Environment` from resolved env **per call**).
- [ ] `FormatToolError(err error) string` — if `apiclient.APIError`, JSON object with `status`, `code`, `title`, `detail`, `hints`; else `{"detail": err.Error()}`. Never include `Authorization` or raw token.
- [ ] `NewServer(cfg Config) *mcp.Server` — `Implementation{Name: "launchpad", Version: "v0.1.0"}`, set Instructions per spec, register **all v1 tools** (write tools may stub-panic only if Task 2 lands same PR immediately — prefer register write handlers in Task 2; Task 1 must register the **read** tools plus `healthz`). **This task registers read tools listed in the spec.** Write tools in Task 2. After Task 2 the exact set must match the spec. After Task 1, tests assert the read subset is present.
- [ ] Read handlers: `healthz`, `list_projects`, `get_project`, `list_environments`, `get_environment`, `get_config`, `list_processes`, `list_releases`, `get_changeset`, `preview`, `get_manifest`, `get_job`, `get_logs`, `inspect`, `target_capabilities` — input structs with json/jsonschema tags; output JSON objects as spec; `healthz` skips token; others RequireToken + ResolveProject except `list_projects` (token only).
- [ ] `preview`: error if both release-pair and env-pair set; else pending.
- [ ] `get_logs`: process default `web`.
- [ ] `inspect`: compose GetProject, GetChangeset, latest release for env (copy logic from `internal/cli` latestReleaseForEnv — duplicate a small helper in mcp, do not import cli), ListProcesses.
- [ ] Tests with `mcp.NewInMemoryTransports()` + `httptest.NewServer` mux returning canned JSON for the paths used. Cover: tool names for read tools; missing token; project required; preview conflict; healthz without token; 409 maps to isError with code.
- [ ] Verify: `mise exec -- go test -C .worktrees/feat-mcp-server ./internal/mcp/...`
- [ ] Commit: `feat(mcp): add stdio server and read tools`

---

## Task 2: MCP write tools including apply and deploy-wait

**Files:**
- Create: `internal/mcp/tools_write.go`, `internal/mcp/tools_write_test.go`
- Modify: `internal/mcp/server.go` (register write tools), `internal/mcp/wait.go` if not in Task 1, `internal/mcp/server_test.go` (exact full tool set)

**Allowed paths:** `internal/mcp/**`

- [ ] Implement write tools from the spec: `create_project`, `create_environment`, `clone_environment`, `apply_manifest`, `stage_config`, `stage_process`, `stage_image`, `stage_scale`, `unstage_last`, `discard_changeset`, `deploy`, `rollback`, `promote`.
- [ ] `apply_manifest`: require exactly one of `document` (object/`map[string]any` or json.RawMessage) or `yaml` string; parse yaml with `gopkg.in/yaml.v3` into `map[string]any`; call `ApplyManifest`; do not PushChangeset.
- [ ] `stage_*`: build the same change maps as `internal/cli` (`type=config|image|scale|process.set|process.unset`) and `StageChanges`.
- [ ] `deploy`: optional one-shot stage (image / config set / scale); load changeset; if nothing pending → error `nothing to deploy`; `PushChangeset`; if `wait` default true, poll GetJob 500ms until succeeded/failed/dead or `timeout_seconds` (default 300). **No stdout.**
- [ ] Sensitive env: `deploy`/`rollback`/`promote` without `confirm` when env is prod/production → error before API call.
- [ ] Tests: apply yaml vs document vs both; apply does not hit `/releases` or `/changeset/push`; deploy wait pending→succeeded; job failed → isError; production without confirm does not hit push.
- [ ] After registration, test exact tool name set equals the spec union (read+write). No extra tools (`create_token` forbidden).
- [ ] Verify: `mise exec -- go test -C .worktrees/feat-mcp-server ./internal/mcp/...`
- [ ] Commit: `feat(mcp): add apply, stage, and deploy-wait tools`

---

## Task 3: CLI `launchpad mcp`

**Files:**
- Modify: `internal/cli/root.go`
- Create: `internal/cli/mcp_test.go` (help/presence only)

**Allowed paths:** `internal/cli/root.go`, `internal/cli/mcp_test.go`

- [ ] Add cobra command `Use: mcp`, Short mentions stdio MCP server. `RunE`: construct `mcp.Config` from `cfg` (`APIURL`, `Token`, `Project`, `Environment`); `srv := mcpserver.NewServer(...)`; `return srv.Run(cmd.Context(), &mcp.StdioTransport{})`. Import SDK mcp as `mcpsdk` and internal package as `lpagent` or `lpmcp` to avoid name clash.
- [ ] Do not add flags. Help path must not call `Run`.
- [ ] Test: `NewRoot(Config{}).Commands()` includes `mcp`; `cmd.Short` contains `stdio` or `MCP`.
- [ ] Verify: `mise exec -- go test -C .worktrees/feat-mcp-server ./internal/cli/... ./internal/mcp/...` and `mise exec -- go build -C .worktrees/feat-mcp-server -o /tmp/launchpad-mcp-build ./cmd/launchpad`
- [ ] Commit: `feat(cli): launchpad mcp stdio server`

---

## Task 4: e2e-stub MCP path

**Files:**
- Create: `test/e2e/mcp_test.go`

**Allowed paths:** `test/e2e/mcp_test.go`

- [ ] `//go:build e2e` package `e2e`. Use `newAuthedClient` + in-process `internal/mcp.NewServer` with `mcp.NewInMemoryTransports()` **or** `CommandTransport` to the built CLI. Prefer in-process against `LAUNCHPAD_API_URL` so the test does not depend on `bin/` path; if using CommandTransport, `exec.Command` the same `launchpadBin` helper other tests use if one exists — otherwise in-process.
- [ ] Recipe from spec Test strategy e2e bullets 1–7 (create, apply image, no job from apply, deploy wait, inspect/releases, secret needs_value without plaintext).
- [ ] Unique project name via `uniqueProjectName()`.
- [ ] Verify with full e2e at Task 5 (this task adds the file; `mise exec -- go test -C .worktrees/feat-mcp-server -count=1 ./internal/... ./pkg/...` still green).
- [ ] Commit: `test(e2e): MCP apply and deploy-wait on stub`

---

## Task 5: Docs + queue

**Files:**
- Modify: `docs/DOMAIN.md`, `docs/DX-VISION.md`, `docs/DESIGN.md`, `README.md`, `AGENTS.md`, `docs/superpowers/program/QUEUE.md`, this plan (checkboxes + status)

**Allowed paths:** those docs + this plan + spec status line if gate passed

- [ ] DOMAIN: CLI `launchpad mcp`; phase 6 yaml+MCP client; no new REST paths
- [ ] DX-VISION: MCP **Shipped** (this PR) + spec link; Active/next
- [ ] DESIGN: clients + phase 6
- [ ] README: agent one-liner under solo workflow or a short Agents blurb
- [ ] AGENTS.md: MCP off “not yet implemented”
- [ ] QUEUE → `implementing` in this commit if PR not opened; orchestrator sets `pr-open` with link
- [ ] Spec header Status: `Approved (self-approve — ADM)` only after spec-review pass
- [ ] Verify L0 from repo root: `mise exec -- go test -C .worktrees/feat-mcp-server ./...` is too wide if e2e tagged; use `make test` equivalent: `mise exec -- bash -lc 'cd .worktrees/feat-mcp-server && mise trust mise.toml 2>/dev/null; make test && make build && go vet ./...'` **prefer** `mise exec -- go test -C .worktrees/feat-mcp-server $(go list -C .worktrees/feat-mcp-server ./... | grep -v /test/e2e)` plus `go vet -C` and `go build -C`.
- [ ] Commit: `docs: MCP server client surface`

---

## Final verification

```bash
# From trusted repo root
mise exec -- go test -C .worktrees/feat-mcp-server ./internal/... ./pkg/... ./cmd/...
mise exec -- go build -C .worktrees/feat-mcp-server -o /tmp/lp-mcp ./cmd/launchpad
mise exec -- go vet -C .worktrees/feat-mcp-server ./...
mise exec -- make openapi-check
make e2e-stub
```

If e2e must run in the worktree, `make -C .worktrees/feat-mcp-server e2e-stub` with `mise exec --` wrapping go invocations inside the script, or `LAUNCHPAD_E2E_API_ADDR` if 18080 busy.

Persona: MCP S1-equivalent + missing-token/missing-project; write `docs/superpowers/program/feedback/2026-08-23-mcp-server.md`. Scout → `IDEAS.md` only.

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status updated to Completed (ready for PR)
- [ ] Spec linked in PR description
- [ ] No `*.db`, `.env`, or `bin/` committed
- [ ] QUEUE Branch lease `feat/mcp-server`; no merge to main
