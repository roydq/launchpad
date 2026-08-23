# Launchpad MCP server

| Field | Value |
|-------|-------|
| **Status** | Draft |
| **Date** | 2026-08-23 |
| **Domain spec** | `docs/DOMAIN.md` (phase 6 integrations; no new entities) |
| **Scope** | stdio MCP tools over existing OpenAPI + `apiclient`, including `apply_manifest`; token auth; no new domain entities |
| **Queue** | `mcp-server` |
| **Branch** | `feat/mcp-server` |
| **Mode** | ADM single-feature (1 PR, open only, no merge) |

---

## Goal

Give agents a first-class Launchpad surface: call the control plane without shelling `curl` or scraping CLI text. v1 is a **thin client** of the shipped REST API (same tokens, same ambient env, same apply-then-diff-then-deploy loop as humans).

```bash
# Host config (Grok / Claude / Cursor) — stdio, same env as the CLI
# [mcp_servers.launchpad]
# command = "launchpad"
# args = ["mcp"]
# env = { LAUNCHPAD_TOKEN = "${LAUNCHPAD_TOKEN}", LAUNCHPAD_API_URL = "${LAUNCHPAD_API_URL}" }

launchpad mcp   # blocks on stdin/stdout JSON-RPC; logs on stderr only
```

**Success criteria:**

1. `launchpad mcp` starts an MCP server over **stdio** using `github.com/modelcontextprotocol/go-sdk` (pin **v1.7.0**). Implementation name `launchpad`. Tools-only (no resources, no prompts).
2. Every tool is a wrapper of an existing `pkg/apiclient` method (or a documented composition of those methods). **No new REST routes, tables, or domain types.**
3. Auth is the CLI story: `Authorization: Bearer $LAUNCHPAD_TOKEN` against `LAUNCHPAD_API_URL` (default `http://localhost:8080`). Missing token → tool `isError` with a recovery sentence **except `healthz`** (no token required). Token material is never echoed in tool output or logs.
4. `apply_manifest` is a first-class tool: stages via `POST /v1/projects/{project}/manifest/apply`; **never deploys**; accepts a JSON `document` object **or** a YAML string (not both); **does not read the filesystem**. Document/yaml xor is an MCP `isError` (not an HTTP 400 — yaml never hits OpenAPI).
5. `deploy` pushes the open changeset (`POST …/changeset/push`), optionally stages `image` / config / scale first (same shapes as CLI `deploy`), and **`wait` defaults to true when the field is omitted** (JSON `wait` is `*bool`; explicit `false` skips polling). Poll `GET /v1/jobs/{id}` until `succeeded` / `failed` / `dead` or timeout.
6. API `problem+json` failures surface as MCP tool errors (`isError=true`) including `code`, `detail`, and `hints`. `GetLogs` 4xx must parse as `apiclient.APIError` (narrow change in `pkg/apiclient`) so `get_logs` meets this rule.
7. In-process MCP tests cover tool registration, context resolution, apply, deploy-wait (omit `wait` ⇒ default true; timeout `isError` with `job_id` + `last_status`), and error mapping. e2e-stub covers apply → preview → deploy-wait in-process **and** a stdio smoke (`mcp.CommandTransport` to `$LAUNCHPAD_E2E_CLI mcp`: initialize + `healthz`).
8. Docs: DOMAIN CLI table + phase 6 “MCP shipped”; DX-VISION Track A MCP row; DESIGN clients diagram; README one-liner; QUEUE `pr-open`; AGENTS.md “suggested future tooling” MCP row → implemented.

---

## Approaches considered

### A. stdio MCP + official Go SDK + apiclient (recommended)

New package `internal/mcp` builds an `mcp.Server`, registers typed tools, and calls `*apiclient.Client`. Cobra command `launchpad mcp` loads the same `cli.Config` as the rest of the CLI and runs `StdioTransport`. Tests use `mcp.NewInMemoryTransports()` plus `httptest` (or a fake backend).

**Pros:** One binary; structured JSON in/out; reuse auth, env header, and apply API; Grok/Claude stdio is the local host path; no new listen port.  
**Cons:** Extra dependency (`go-sdk`); tool count must be curated.

### B. Shell-out to the CLI

Each tool `exec`s `launchpad <verb>` and returns stdout.

**Pros:** Tiny wrapper.  
**Cons:** Parses human text; loses problem+json; `mcp` stdout would collide if nested; secrets more likely to leak in captured output; MCP and TUI still cannot share a client.

### C. Streamable HTTP MCP inside `cmd/api`

Serve MCP next to REST.

**Pros:** Remote agents without a local binary.  
**Cons:** New public surface, session/OAuth questions, ops (TLS, bind address); local hosts already launch stdio. Premature vs OIDC parked.

**Recommendation:** **A**. Reject B (text scraping). Reject C for v1 (later slice if hosted agents appear; do not half-build HTTP MCP).

---

## Scope

### In scope

- `internal/mcp` server factory + tool handlers + stderr logging.
- `launchpad mcp` cobra command (stdio; process exits when the client disconnects).
- Tool list in [Tools (v1)](#tools-v1) — frozen for this PR.
- Context: `project` / `environment` tool args override `cli.Config` (env vars > project-local `.launchpad/config` > `~/.launchpad/config` > default env `dev`).
- Sensitive-env confirm: `deploy` / `rollback` / `promote` require `confirm=true` when the target env name is `prod` or `production` (same set as CLI `--yes`). Other tools do not.
- `apply_manifest` YAML parse via existing `gopkg.in/yaml.v3` (same stringify rules as CLI apply).
- Structured tool errors from `apiclient.APIError`.
- Unit/in-process tests + e2e-stub MCP path.
- Docs listed in success criterion 8. **No OpenAPI change** (no new HTTP).

### Out of scope (this feature)

- Streamable HTTP / SSE MCP transport.
- MCP resources, prompts, sampling, elicitation.
- `create_token` tool (plaintext secrets in agent transcripts).
- Reading `launchpad.yaml` from a filesystem `path` argument.
- `launchpad new` / recipes, `use`, `env use`, `completion`, `prompt`, `shell-init`, `doctor` (local/CLI-only).
- `process.apply` Procfile file path (agents can `stage_process` or pass procfile text later — **not this PR**).
- Changing apply/deploy/changeset semantics.
- TUI, docs site, dashboard.

### Deferred (do not half-build)

- Multi-service / ReleaseSet / bindings / workspace config layer.
- OIDC login for MCP or REST.
- Idempotency keys, SSE job events, scale-as-job API.
- GitOps reconcile, Helm, builds.
- Remote MCP auth (OAuth resource server).

---

## Domain impact

No new entities. MCP is another **client** of Project, Environment, Service, Process, Changeset, Release, Deployment, Job — same as the CLI.

| Entity | Change |
|--------|--------|
| Project / Environment / Service / Process / Changeset / Release / Deployment / Job | Unchanged |
| Manifest document | Unchanged (yaml v1 apply/export already shipped) |
| MCP | **New client surface** (not stored) |

**Invariants to preserve:**

- Releases stay immutable; MCP `deploy` is changeset push (or rollback/promote), not in-place edit.
- `apply_manifest` only stages; live/release tables update on a later push.
- Secret values never appear on control-plane reads; MCP must not add an include-secrets flag.
- Single primary service; no `services:` / bindings tools.
- Ambient env via `X-Launchpad-Environment` (tool `environment` or config; default `dev`) on tools that use the header. **`get_manifest` is the exception** (DOMAIN: GET `/manifest` ignores the header; query `environment` only).
- At most one open changeset, pinned to one environment — 409s pass through as tool errors.
- Sensitive env names `prod` / `production` still need explicit confirm on deploy/rollback/promote.

**Invariants to add:**

1. MCP v1 is stdio + tools only; it does not persist sessions in Launchpad.
2. MCP never logs or returns bearer tokens or secret config values.
3. MCP does not read host files for apply (no path argument).
4. Tool names in this spec are the v1 contract; adding tools later is a new slice, not silent expansion in this PR.
5. `get_manifest` `environment` is an **optional export filter taken only from the tool argument**. Omitted argument → unfiltered export (all envs), even if config/default env is `dev`. Never pass `ResolveEnv()` as `GetManifest` envFilter.

DOMAIN updates in this PR: phase 6 “MCP follows” → shipped client; CLI table `launchpad mcp`; integrations paragraph. No new REST paths.

---

## Auth story

| Layer | Behavior |
|-------|----------|
| MCP protocol | No Launchpad login. Host starts `launchpad mcp` as a subprocess. |
| REST | Every API call (except `healthz`) sends `Authorization: Bearer <token>` and optional `X-Launchpad-Environment`. |
| Token source | `LAUNCHPAD_TOKEN` (process env). Same as CLI. Optional inherit from host MCP `env`. |
| API URL | `LAUNCHPAD_API_URL`, default `http://localhost:8080`. |
| Project default | Tool `project` arg, else `LAUNCHPAD_PROJECT`, else project-local `.launchpad/config`, else `~/.launchpad/config`. If still empty → tool error: set `project` or `LAUNCHPAD_PROJECT`. |
| Env default | Tool `environment` arg, else `LAUNCHPAD_ENV` / config layers, else `dev`. Sent as `X-Launchpad-Environment` on project-scoped mutating/read tools **except** `get_manifest` (see Tools). |
| Bootstrap token | Accepted if the operator puts it in `LAUNCHPAD_TOKEN` (API already allows it). Docs tell agents to mint a workspace token via CLI/`POST /v1/tokens` **outside** MCP. |
| Token minting | **Not a tool.** |
| Missing token | `healthz` still runs. All other tools: `isError`, detail `LAUNCHPAD_TOKEN is not set`. |
| 401/403 | Pass through as tool errors with problem+json fields. |

Host example (Grok project or user config):

```toml
[mcp_servers.launchpad]
command = "launchpad"
args = ["mcp"]
env = { LAUNCHPAD_TOKEN = "${LAUNCHPAD_TOKEN}", LAUNCHPAD_API_URL = "${LAUNCHPAD_API_URL}" }
```

`${VAR}` expansion is the host’s job, not Launchpad’s.

---

## Architecture

```
MCP host (Grok / Claude / Cursor)
    │  stdio JSON-RPC
    ▼
launchpad mcp          # cobra; logs → stderr
    │
internal/mcp.Server    # go-sdk mcp.NewServer
    │  tools
    ▼
pkg/apiclient.Client   # existing HTTP + Bearer + env header
    ▼
cmd/api REST           # unchanged OpenAPI
```

**Package layout:**

| Path | Responsibility |
|------|----------------|
| `internal/mcp/server.go` | `NewServer(cfg Config) *mcp.Server`; instructions; register tools |
| `internal/mcp/config.go` | Config (APIURL, Token, Project, Environment); resolve project/env; sensitive-env check |
| `internal/mcp/errors.go` | Map `error` / `apiclient.APIError` → tool failure text (JSON with status, code, title, detail, hints) |
| `internal/mcp/tools_read.go` | Read tools |
| `internal/mcp/tools_write.go` | Write tools including apply + deploy-wait |
| `internal/mcp/*_test.go` | In-memory MCP client + httptest backend |
| `internal/cli/root.go` | `mcp` subcommand: `internalmcp.NewServer` + `StdioTransport` |

CLI `Config` may be copied into `mcp.Config` (strings only) so `internal/mcp` does not import cobra. `internal/cli` may import `internal/mcp`.

**Logging:** `log/slog` to **stderr**. Never write non-protocol bytes to stdout.

**MCP instructions** (server instructions string, ~3 sentences): Launchpad control plane for one primary service. `apply_manifest` stages desired state and does not deploy. Call `preview` (or inspect pending) then `deploy` with wait (default true). Secret values are omitted by the API. Default environment is `dev`.

**SDK:** `github.com/modelcontextprotocol/go-sdk v1.7.0` — `mcp.AddTool` with typed input structs (`json` + `jsonschema` tags). Do not use `mark3labs/mcp-go`.

---

## Tools (v1)

Frozen list. Names are MCP tool names (host will qualify as `launchpad__<name>`). Optional fields omitted unless noted.

### Scoping

**Project-scoped** tools take optional `project` / `environment` and resolve them as in Auth (env default `dev`). They set `client.Environment` for that call.

| Tool | Token | Project | Ambient env header | Notes |
|------|-------|---------|--------------------|-------|
| `healthz` | no | no | no | |
| `list_projects` | yes | no | no | |
| `create_project` | yes | no | no | Input `name` is the new project |
| `get_job` | yes | no | no | `id` only |
| `target_capabilities` | yes | only if `type` omitted | only if `type` omitted | If `type` set, GET `/v1/targets/{type}/capabilities` only |
| `get_manifest` | yes | **yes** | **no** | `environment` is an optional **query filter**, not ambient default |
| All other v1 tools | yes | yes | yes | |

Shared optional args on **project-scoped** tools (not the exceptions above, except `get_manifest` which takes `project` plus optional filter `environment`):

| Arg | Type | Default |
|-----|------|---------|
| `project` | string | resolved context (error if empty) |
| `environment` | string | resolved context / `dev` (ambient header). **`get_manifest`:** no default; omitted = all envs |

### Read

| Tool | apiclient / composition | Input beyond shared | Output |
|------|-------------------------|---------------------|--------|
| `healthz` | `Healthz` | (none; no token required) | `{ "ok": true }` |
| `list_projects` | `ListProjects` | — | `{ "projects": [...] }` |
| `get_project` | `GetProject` | — | project object (apiclient JSON) |
| `list_environments` | `ListEnvironments` | — | `{ "environments": [...] }` |
| `get_environment` | `GetEnvironment` | `name` (default: resolved env) | environment object |
| `get_config` | `GetConfig` / `GetConfigLayer` | `layer` optional `shared`\|`service` | map (secrets redacted by API) |
| `list_processes` | `ListProcesses` | — | `{ "processes": [...] }` |
| `list_releases` | `ListReleases` | — | `{ "releases": [...] }` |
| `get_changeset` | `GetChangeset` | — | changeset or empty pending |
| `preview` | `PreviewPending` / `PreviewReleases` / `PreviewEnvironments` | `from_release`,`to_release` **or** `from_env`,`to_env`; if neither, pending vs last deploy | preview object |
| `get_manifest` | `GetManifest` | optional `environment` **argument only** (no ResolveEnv fallback) | unfiltered manifest JSON if omitted; `environments` map contains only that env if set |
| `get_job` | `GetJob` | `id` required | job object |
| `get_logs` | `GetLogs` | `process` default `web` | `{ "logs": "<text>" }` |
| `inspect` | composition: `GetProject` + `GetChangeset` + latest release-in-env + `ListProcesses` | — | see below |
| `target_capabilities` | `TargetCapabilities` | `type` optional; if empty, `GetEnvironment` then use its `target_type` | capabilities object |

`preview` is invalid if both release-pair and env-pair are set.

**`inspect` output (pinned):**

```json
{
  "project": "<name string>",
  "environment": "<name string>",
  "status": "<GetProject.Status string>",
  "pending_count": 0,
  "last_deploy": null,
  "processes": [{ "name": "web", "quantity": 1, "expose": "http" }]
}
```

`last_deploy` is JSON `null` when the env has no release deployment; otherwise `{ "version": <int>, "artifact_ref": "<string>", "status": "<string>" }` from the latest release-in-env (same selection as CLI `inspect`). `pending_count` is `len(changeset.Changes)` or `0`. `project` / `environment` are **names**, not objects.

### Write

Passthrough tools return the apiclient JSON for that method (`CreateProject` → project object, `StageChanges` → changeset, `ApplyManifest` → `ApplyReport`, `UnstageLastChange` → unstage result, `DiscardChangeset` → `{ "ok": true }`).

| Tool | apiclient | Input beyond shared | Output | Notes |
|------|-----------|---------------------|--------|-------|
| `create_project` | `CreateProject` | `name` required; `target` default `stub`; `namespace` default `default` | project object | Bootstraps `dev` + primary service + `web` |
| `create_environment` | `CreateEnvironment` | `name` required; `target` default `stub`; `namespace` default `default`; `ephemeral` default false | environment object | |
| `clone_environment` | `CloneEnvironment` | `from` + `name` required; optional `target`, `namespace`, `ephemeral` | clone result | Secrets → `needs_value` only |
| `apply_manifest` | `ApplyManifest` | `document` object **xor** `yaml` string | `ApplyReport` | Stage only. Both/neither → MCP `isError` (not HTTP 400). Yaml parsed to object then POSTed as `document`. Project arg must match document `project` when both present (API also enforces). |
| `stage_config` | `StageChanges` type `config` | `set` object and/or `unset` string[]; `layer` `service` (default) or `shared`; `secret` bool default false | changeset | At least one set or unset; `secret=true` → `sensitivity=secret` |
| `stage_process` | `StageChanges` `process.set` or `process.unset` | `name` required; `unset` bool; optional `command`, `quantity`, `expose`, `health` object | changeset | `unset=true` ignores other process fields |
| `stage_image` | `StageChanges` type `image` | `image` required | changeset | |
| `stage_scale` | `StageChanges` type `scale` | `process` + `quantity` required | changeset | Quantity staging only; not the deferred scale-job API |
| `unstage_last` | `UnstageLastChange` | — | unstage result | |
| `discard_changeset` | `DiscardChangeset` | — | `{ "ok": true }` | Same as CLI `reset` |
| `deploy` | optional stage then `PushChangeset` | `image`, `set` (config map), `scale_process`+`scale_quantity`, `message`, `wait` `*bool` omit=true, `timeout_seconds` default 300, `confirm` bool | wait payload below | Empty changeset and no one-shot fields → `isError` `nothing to deploy` |
| `rollback` | `Rollback` | `version` int required; `message` optional; `wait` `*bool` omit=true; `timeout_seconds` 300; `confirm` | wait payload below | |
| `promote` | `Promote` | `from` required; `to` default resolved env; `version` optional **0 = running release in `from`** (not highest service version); `message`; `wait` `*bool` omit=true; `timeout_seconds` 300; `confirm` | wait payload below | Matches DOMAIN/CLI: omit/`0` → running in `--from`; if none running, API error to pass version explicitly |

**Sensitive env:** if resolved target env (for `promote`, the `to` env) is `prod` or `production` (case-insensitive), `deploy` / `rollback` / `promote` fail unless `confirm=true`. Error text matches CLI intent: refusing to modify sensitive environment without confirm.

**Wait field:** JSON `wait` is `*bool`. Omitted/`null` → **true**. Explicit `false` → return immediately after enqueue. Do not use a non-pointer `bool` (zero value would invert the default).

**Wait helper:** poll `GetJob` every 500ms. Terminal success = `succeeded`. Terminal failure = `failed` or `dead`. Timeout → `isError` text JSON `{ "detail": "timed out waiting for job", "job_id": "<id>", "last_status": "<status>" }`. Wait must not write to stdout.

**Wait success/failure output (`deploy` / `rollback` / `promote`):**

- `wait=false`: apiclient `DeployResult` (`deployment` + `job` as returned by push/rollback/promote).
- `wait=true` and job `succeeded`: `{ "deployment": <DeployResult.deployment>, "job": <final GetJob object>, "wait": { "status": "succeeded" } }`.
- `wait=true` and job `failed`/`dead`: `isError` text JSON `{ "detail": "deploy failed", "job_id": "<id>", "status": "<status>", "last_error": "<job.LastError>" }` (plus problem fields if the GetJob call itself failed as APIError).

**Not tools:** `CreateToken`, `PatchConfig` (live `--now`; agents stage then deploy), doctor, recipes, filesystem apply.

---

## API sketch

No new HTTP. Existing paths used by tools:

| Method | Path | Tools |
|--------|------|-------|
| `GET` | `/healthz` | `healthz` |
| `GET`/`POST` | `/v1/projects` | `list_projects`, `create_project` |
| `GET` | `/v1/projects/{project}` | `get_project`, `inspect` |
| `GET` | `/v1/projects/{project}/config` | `get_config` |
| `GET` | `/v1/projects/{project}/processes` | `list_processes`, `inspect` |
| `GET` | `/v1/projects/{project}/logs` | `get_logs` |
| `GET`/`POST` | `/v1/projects/{project}/environments` | `list_environments`, `create_environment` |
| `GET` | `/v1/projects/{project}/environments/{name}` | `get_environment`, `target_capabilities` fallback |
| `POST` | `/v1/projects/{project}/environments/{name}/clone` | `clone_environment` |
| `GET` | `/v1/projects/{project}/releases` | `list_releases`, `inspect` |
| `POST` | `/v1/projects/{project}/rollback` | `rollback` |
| `POST` | `/v1/projects/{project}/promote` | `promote` |
| `GET` | `/v1/projects/{project}/changeset` | `get_changeset`, `inspect` |
| `POST` | `/v1/projects/{project}/changeset/changes` | `stage_*`, `deploy` one-shot |
| `DELETE` | `/v1/projects/{project}/changeset/changes/last` | `unstage_last` |
| `DELETE` | `/v1/projects/{project}/changeset` | `discard_changeset` |
| `POST` | `/v1/projects/{project}/changeset/push` | `deploy` |
| `GET` | `/v1/projects/{project}/preview` | `preview` |
| `GET` | `/v1/projects/{project}/manifest` | `get_manifest` (query `environment` **only** when the tool arg is set; never from ResolveEnv) |
| `POST` | `/v1/projects/{project}/manifest/apply` | `apply_manifest` |
| `GET` | `/v1/jobs/{id}` | `get_job`, wait |
| `GET` | `/v1/targets/{type}/capabilities` | `target_capabilities` |

RFC 7807 errors unchanged; MCP wraps them.

---

## Schema sketch

None. No migrations.

---

## Target / worker impact

None. Worker and targets are unchanged. MCP wait only **observes** job status already written by the worker.

---

## CLI

```text
launchpad mcp
```

Short: `Run the Launchpad MCP server on stdio`. Long: points at README/DOMAIN; notes stdout is protocol; token via `LAUNCHPAD_TOKEN`.

No flags in v1 (config is env + existing CLI config files). Process runs until stdin closes / transport error; then returns that error to cobra.

`launchpad mcp --help` must not start the server (cobra help path).

---

## Test strategy

- **Unit (`internal/mcp`):**  
  - Registered tool names equal the v1 table (exact set; no extras).  
  - Context: explicit `project` wins; empty project + empty config → error on **project-scoped** tools; default ambient env `dev`. `get_job` / `list_projects` / `create_project` succeed without a project.  
  - Missing token: `healthz` ok; `list_projects` isError mentioning `LAUNCHPAD_TOKEN`.  
  - Sensitive env: `deploy` to `production` without `confirm` fails; with `confirm=true` reaches the backend.  
  - `apply_manifest`: both document+yaml → MCP `isError`; yaml-only parses and POSTs JSON document; apply does not call push/releases.  
  - `preview`: both pairs set → error.  
  - API 409 problem+json → isError body contains `code` and first hint `command` when present.  
  - `get_logs` 4xx problem+json → isError with `code` (requires `GetLogs` to return `APIError`).  
  - `get_manifest`: Config.Environment=`dev` and **omitted** tool arg → GET `/manifest` **without** `?environment=`; explicit `environment=staging` → query set. Assert `environments` map is unfiltered vs single-key.  
  - `inspect`: pin JSON types (project/environment strings, `pending_count` int, `last_deploy` null vs `{version, artifact_ref, status}`).  
  - `deploy` wait: **omit** `wait` in arguments (not `wait=true`) → polls; httptest job `pending` then `succeeded`; output includes `wait.status=succeeded` and final `job`. Job `failed` → isError with `job_id`. Explicit `wait=false` → no second GetJob poll after push. Timeout: job stays `pending`, `timeout_seconds=1` → isError with `job_id` + `last_status`.  
- **CLI:** `mcp` command exists on the root cobra command; help string mentions stdio. Do not run stdio in unit tests.  
- **e2e-stub (`test/e2e/mcp_test.go`, `//go:build e2e`):**  
  1. In-process MCP against `LAUNCHPAD_API_URL` for the apply/deploy recipe.  
  2. `create_project` unique name, target stub (no pre-set project context required).  
  3. `apply_manifest` with a document that sets `image` (and optional plain config).  
  4. `preview` or `get_changeset` shows pending; **no** succeeded job from apply.  
  5. `deploy` **omitting** `wait` succeeds (default true).  
  6. `inspect` has `last_deploy.version` set; `list_releases` non-empty.  
  7. Separate case: `apply_manifest` with `secret_keys` / secret placeholder → `needs_value`; output must not contain a planted secret string.  
  8. `get_manifest` without `environment` after a second env exists → `environments` contains more than one key (or at least is not forced to `dev` only).  
  9. **Stdio smoke (required):** `mcp.CommandTransport` with `exec.Command(os.Getenv("LAUNCHPAD_E2E_CLI"), "mcp")` (default `./bin/launchpad`), env `LAUNCHPAD_TOKEN` + `LAUNCHPAD_API_URL`; `initialize` + `healthz` `{ok: true}`. Does not replace the in-process apply/deploy recipe.  
- **OpenAPI:** no change; `make openapi-check` still green.  
- **L0:** `mise exec -- make test && make build && go vet ./...`  
- **L1:** `make e2e-stub` required (deploy path via MCP).

---

## Docs sync

| Doc | Change |
|-----|--------|
| `docs/DOMAIN.md` | Phase 6: yaml **and** MCP client shipped; CLI `launchpad mcp`; note no new REST |
| `docs/DX-VISION.md` | Track A / Active / next / P4: MCP **Shipped** (this PR) with spec link |
| `docs/DESIGN.md` | Clients include MCP; phase 6 integrations yaml+MCP |
| `README.md` | Agent one-liner: `launchpad mcp` + token env |
| `AGENTS.md` | Move MCP off “suggested future”; still no DOMAIN fork |
| `QUEUE.md` | `designing` → `implementing` → `pr-open` |
| `docs/openapi.yaml` | **Unchanged** |

Persona: MCP is agent UX. After e2e, run a stub-backed tool path equivalent to S1 (create → apply → preview → deploy wait → inspect) and S4-ish mistakes (missing token, missing project, apply+yaml together). Write `docs/superpowers/program/feedback/2026-08-23-mcp-server.md`. Human CLI S1 is unchanged; do not require a full PERSONA S1 unless the CLI happy path regresses.

---

## Recommended path (recorded)

**A** — stdio MCP, official go-sdk v1.7.0, apiclient wrappers, `launchpad mcp`. Rejected B (CLI scrape) and C (HTTP MCP in API).

---

## Spec self-review (ADM)

| Check | Result |
|-------|--------|
| 1. No placeholders | Pass — no TBD; tool table complete |
| 2. Internal consistency | Pass — tools map to existing apiclient/OpenAPI; apply never deploys; wait polls jobs; healthz token exception in SC3; wait is `*bool` |
| 3. Single plan scope | Pass — stdio tools only; HTTP MCP deferred |
| 4. No DOMAIN contradiction | Pass — client only; yaml apply unchanged; `get_manifest` omit = all envs (query only, no ambient filter) |
| 5. MVP boundary | Pass — no multi-service, bindings, OIDC, scale job, SSE |
| 6. Recommended path recorded | A |
| 7. Test strategy | Unit + in-process MCP + e2e-stub + CommandTransport stdio smoke |
| 8. DoD / acceptance | Success criteria 1–8 |

**Status:** Draft until `adm-spec-review` pass=true, then `Approved (self-approve — ADM)`.

Gate history: `adm-spec-review` `wf_01a02f62dc5a777089b07e73f906c6e6` **pass=false** (blocker `manifest-env-filter` + 9 warnings). This revision folds the blocker and warnings as implementer rules.

---

## Open questions

None blocking. Resolved in this spec:

- Transport is **stdio** only. HTTP MCP is a future row, not a hidden flag.
- Apply does **not** read disk paths; agents pass `document` or `yaml`. Document/yaml xor is MCP `isError`, not HTTP 400.
- `wait` on deploy/rollback/promote defaults **true when omitted** (`*bool`); CLI `--wait` default stays false.
- Confirm flag only on the same three verbs as CLI `--yes`.
- No `create_token` tool.
- `inspect` is an allowed composition of existing GETs with a pinned JSON shape (names, not objects).
- `stage_scale` is changeset quantity, not the deferred scale API.
- SDK is official `go-sdk` v1.7.0, not community mcp-go.
- OpenAPI unchanged.
- `get_manifest` environment is an optional query filter; omitted = all envs (DOMAIN / `launchpad export`).
- `promote` version 0 = **running** release in `from`, not highest version.
- `get_job` / `list_projects` / `create_project` / `healthz` are not project-scoped; `target_capabilities` only needs project when `type` is omitted.
- `GetLogs` 4xx parsed as `APIError` (narrow apiclient change).

---

## Approval

- [ ] `adm-spec-review` pass=true (self-approve — ADM)
- [ ] Design reviewed and approved (ADM self-approve after gate)
