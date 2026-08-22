# launchpad.yaml v1 (project manifest)

| Field | Value |
|-------|-------|
| **Status** | Draft (ADM self-approve pending spec review) |
| **Date** | 2026-08-22 |
| **Domain spec** | `docs/DOMAIN.md` (phase 6 v1) |
| **Scope** | Import/export of the **shipped** single-service model as `launchpad.yaml`; stage via existing changeset; no GitOps; no multi-service |
| **Queue** | `launchpad-yaml` |
| **Branch** | `feat/launchpad-yaml` |
| **Mode** | ADM single-feature (1 PR, open only, no merge) |

---

## Goal

Give a Launchpad project a mise.toml-shaped file: check it into git, recreate or converge **declared** desired state with one command, without deploying and without putting secrets in the repo.

```bash
# From an existing project@env
launchpad export
# → writes ./launchpad.yaml (plain config, processes, env targets, image refs; secret keys listed, values omitted)

# Apply stages; never pushes a release
launchpad apply
# created environment: (none)
# staged: process.set web, config.service.PORT, image
# needs_value: DATABASE_URL
# next: launchpad diff && launchpad deploy --wait

# New project from a file that includes environments.dev
launchpad apply -f launchpad.yaml
# created project my-api (target stub)
# context: my-api @ dev
# staged: ...
```

**Success criteria:**

1. `launchpad export` writes a v1 document that round-trips the shipped model for the current project: processes (command, quantity, expose, health, target_extensions), each environment's target type + namespace/cluster, plain shared/service config, secret **keys** (no values), and last-deployed image per env (when any).
2. `launchpad apply -f launchpad.yaml` creates the project if missing (**requires `environments.dev`**), creates missing environments, and **stages** process/config/image diffs for the **selected environment** only. It does **not** deploy, prune undeclared keys/processes, patch an existing env's target, or write secret values.
3. `GET /v1/projects/{project}/manifest` and `POST /v1/projects/{project}/manifest/apply` are the API; CLI is YAML over those endpoints.
4. Applying a document that contains a secret **value**, `services:`, `bindings:`, or `version != 1` fails closed with problem+json.
5. Re-export after apply (before deploy) matches the declared fields (round-trip). e2e-stub covers export → apply-to-new-project → diff → deploy --wait.
6. OpenAPI lists the new routes; `make openapi-check` passes.

---

## Approaches considered

### A. Domain document + service apply/export + REST + CLI (recommended)

Canonical `domain.Manifest` type. Service **export** reads live tables; service **apply** creates project/env as needed and stages changeset rows via existing `ChangesetService`. API is JSON; CLI reads/writes YAML.

**Pros:** One apply path for CLI, future MCP, and other clients; validation lives in domain; no new tables; matches phase 6 “API, store, worker, CLI together” with store/worker as N/A.  
**Cons:** More layers than a CLI script.

### B. CLI-only (like `launchpad new` recipes)

CLI parses YAML and calls CreateProject / StageChanges / CreateEnvironment in a loop.

**Pros:** Smaller PR.  
**Cons:** MCP and TUI would reimplement apply; secrets handling in the client is easier to get wrong; no single server-side report.

### C. Persist the file and reconcile (GitOps)

Store last-applied hash; worker converges cluster to the file.

**Pros:** Hands-off.  
**Cons:** Explicit DOMAIN non-goal; “file is not continuously reconciled.”

**Recommendation:** A.

---

## Scope

### In scope

- `domain.Manifest` + validation (version, project name, unknown keys, secret values, deferred-field rejection).
- Service `ExportManifest` / `ApplyManifest` with a structured report.
- `GET /v1/projects/{project}/manifest` (optional `?environment=` to filter env map).
- `POST /v1/projects/{project}/manifest/apply` (create project if 404 and name matches document; stage only).
- CLI `launchpad export` / `launchpad apply`.
- File format `launchpad.yaml` (also `launchpad.yml` as `-f` alias).
- OpenAPI, DOMAIN phase 6 notes, README one-liner, QUEUE/DX-VISION.
- Unit tests (domain + service) and e2e-stub CLI path.

### Out of scope (this feature)

- Deploy / `--now` / `--wait` on apply (user runs `diff` then `deploy`).
- Prune / `--prune` of processes or config keys not in the file.
- PATCH of `target_type` / `target_config` on an **existing** environment (warn on mismatch; create-only uses yaml target).
- `launchpad new` writing a yaml; `init` skeleton without an API.
- Walk-up file discovery (cwd + `-f` only).
- `application/yaml` on the HTTP API (JSON only).
- Man pages, TUI, docs site.
- MCP server (next QUEUE row; apply API is the hook).

### Deferred (do not half-build)

- Multi-service (`services:` map, non-primary `service:`, ReleaseSet, coordination).
- Bindings (`${{ }}` in the file as a first-class type).
- Workspace config layer.
- GitOps reconcile, Helm, builds, OIDC.
- Per-env process topology (processes remain **service-scoped**).
- Writing secret values into or out of the file (`secret_ref` / external SM).

---

## Domain impact

No new entities. Manifest is an **import/export document** over existing Project, Environment, Service (primary only), Process, and layered config.

| Entity | Change |
|--------|--------|
| Project / Environment / Service / Process / Changeset / Release | Unchanged |
| Manifest | **New document type** (not stored) |

**Invariants to preserve:**

- Releases stay immutable; apply only stages the next snapshot.
- Config still resolves at release creation, not at apply time.
- Bootstrap: `CreateProject` still creates `dev` + primary service + `web`.
- At most one open changeset, pinned to one environment.
- Secret values never appear on control-plane reads; export uses the same rule.
- Single primary service; project name = primary service name.

**Invariants to add:**

1. Manifest `version` must be `1`.
2. `document.project` must equal the URL `{project}` on apply.
3. Creating a project via apply **requires** `environments.dev` in the document (bootstrap).
4. Secret values in the document are a **400**; secret keys may be listed without values.
5. Apply is **upsert** of declared fields; it does not prune undeclared processes or config keys.
6. Apply does not push a release.
7. File is not stored or reconciled by the worker.

DOMAIN updates in this PR: phase 6 v1 description (already noted as next); CLI table row for `apply` / `export`; open question 5 marked shipped-as-designed for v1.

---

## Document schema (v1)

YAML on disk; JSON on the wire. Field names are snake_case to match the API.

```yaml
version: 1
project: my-api
processes:
  web:
    command: ""
    quantity: 1
    expose: http
    health:
      type: http
      path: /healthz
    target_extensions:
      kubernetes:
        resources:
          requests:
            memory: 256Mi
  worker:
    command: run-worker
    quantity: 1
    expose: none
environments:
  dev:
    target: stub
    namespace: default
    cluster: ""
    ephemeral: false
    image: hello:v1
    config:
      shared:
        LOG_LEVEL: info
      service:
        PORT: "8080"
      secret_keys:
        shared: []
        service:
          - DATABASE_URL
```

### Field rules

| Field | Export | Apply |
|-------|--------|-------|
| `version` | always `1` | required; other values → 400 `manifest_version` |
| `project` | project name | required; must match URL |
| `processes` | all live processes | omitted → do not touch processes; each **declared** name is a full upsert (command, quantity, expose, health, extensions). Omitted `command` → empty (image default). Omitted `quantity` → `1`. Omitted `expose` → `http` if name is `web`, else `none`. Omitted health → no probe. Omitted extensions → empty map. |
| `environments` | all envs, or one if `?environment=` | required; must contain the **selected** env |
| `environments.*.target` | `target_type` | required when **creating** that env; if env exists and differs → **warning**, no mutation |
| `namespace` / `cluster` | from `target_config` | used only on create |
| `ephemeral` | live flag | used only on create |
| `image` | artifact of latest meaningful release in that env (same rule as inspect); omit if never deployed | omitted → do not stage image; present and different from pending image or last release → stage `image` |
| `config.shared` / `config.service` | plain keys with values; secret keys **absent** from these maps | upsert plain values when different from live; **reject** if a listed value's live sensitivity is `secret` |
| `config.secret_keys` | names of secret keys per layer | do not write values; if live key missing or empty placeholder → add name to `needs_value[]`; if live secret already set → no-op |

### Fail closed (400)

- Unknown **top-level** keys (`services`, `bindings`, `kind`, …).
- `version` ≠ 1.
- Invalid project / env / process names (existing DNS-label rules).
- Secret **value** anywhere: a key listed under `config.shared`/`service` **and** `secret_keys` for that layer; or a non-string config value object with `secret: true` and a value (not supported — use `secret_keys` only).
- `primary_service` or `service` field not equal to `project` (multi-service leak).
- Selected apply environment missing from `environments`.
- Create-project path without `environments.dev`.
- Empty document / missing `project`.

### Selected environment (apply)

1. Request `environment` field if set, else `X-Launchpad-Environment`, else `dev`.
2. That name must exist in `document.environments`.
3. Process upserts are **service-scoped** (they affect the next deploy in every env). Config and image apply only to the selected env. Document this in CLI help.

### Create vs update

| Situation | Behavior |
|-----------|----------|
| Project missing | `CreateProject` using `environments.dev` target/namespace/cluster; then continue apply |
| Env missing | `CreateEnvironment` from that env's target block |
| Env exists, target differs | warning `target_mismatch`; continue |
| Declared process vs live | `process.set` when any portable field differs |
| Live process not in file | warning `undeclared_process`; **do not** unset |
| Plain config differs | `config` change with `layer` |
| Live config key not in file | ignore (no prune) |
| Nothing to stage | `staged: []`, changeset unchanged/empty; still 200 |

Apply never calls push.

---

## API sketch

| Method | Path | Scope | Purpose |
|--------|------|-------|---------|
| `GET` | `/v1/projects/{project}/manifest` | `project:read` | Export live document. Query `environment` optional (filters `environments` map). |
| `POST` | `/v1/projects/{project}/manifest/apply` | `project:write` | Create-if-missing + stage. Body below. Ambient env header unless overridden. |

`GET` 404 if project does not exist.

### POST body

```json
{
  "document": { "version": 1, "project": "my-api", "processes": {}, "environments": {} },
  "environment": "dev"
}
```

`environment` optional (header / default `dev`).

### POST 200 body

```json
{
  "project": "my-api",
  "environment": "dev",
  "created_project": false,
  "created_environment": false,
  "staged": ["process.set web", "config.service.PORT", "image"],
  "unchanged": ["process.worker"],
  "warnings": [
    "live process 'release' is not in the manifest (not pruned)",
    "environment dev target is stub; manifest says kubernetes (not changed)"
  ],
  "needs_value": ["DATABASE_URL"],
  "changeset": { "id": "...", "change_count": 3 }
}
```

`changeset` is null when nothing was staged.

### Errors (problem+json)

| HTTP | code | When |
|------|------|------|
| 400 | `manifest_version` | version ≠ 1 |
| 400 | `manifest_invalid` | schema / unknown keys / name rules |
| 400 | `manifest_secret_value` | secret value in document |
| 400 | `manifest_deferred` | `services` / bindings / non-primary service |
| 400 | `manifest_env` | selected env not in document; or create without `dev` |
| 400 | `manifest_project_mismatch` | URL name ≠ `document.project` |
| 409 | `changeset_env_mismatch` | existing open changeset pinned to another env (reuse existing hint) |
| 404 | `not_found` | GET missing project |

Hints: secret value → remove values, list keys under `secret_keys`; deferred → single primary service only; env → `launchpad env list` / add the env block.

---

## Schema sketch

No new tables or columns. No worker jobs.

---

## Target / worker impact

None. Apply does not enqueue deploys. Export does not read targets.

---

## CLI sketch

| Command | Behavior |
|---------|----------|
| `launchpad export` | GET manifest for current project (all envs). Write `./launchpad.yaml`. Refuse overwrite unless `--force`. |
| `launchpad export --stdout` | Print YAML to stdout. |
| `launchpad export -f PATH` | Write PATH (`-` = stdout). |
| `launchpad export --env NAME` | GET with `?environment=NAME`. |
| `launchpad apply` | Read `./launchpad.yaml` or `./launchpad.yml` (yaml wins if both). POST apply for `document.project`. Selected env = ambient / `--env`. |
| `launchpad apply -f PATH` | Read PATH. |
| `launchpad apply --env NAME` | Override selected env. |

If apply **creates** a project, write project-local `.launchpad/config` (same as `launchpad new`) and print context. If the project already existed, do not change context unless `--use`.

Print the report (created_*, staged, warnings, needs_value) and `next: launchpad diff && launchpad deploy --wait` when `staged` is non-empty.

`--deploy` is **not** accepted (out of scope). Collides with `process apply` only by nesting; top-level `apply` is the manifest verb.

Default file: **cwd only** (no walk-up).

---

## Service sketch

`ManifestService` (new) depends on `ProjectService`, `ConfigService`, `ChangesetService`, `ReleaseService`, store.

- `Export(ctx, project, envFilter string) (*domain.Manifest, error)`
  - List processes → manifest processes (including health + extensions).
  - List environments (or one) → target, namespace, cluster, ephemeral.
  - Typed config per env, per layer: plains into maps; secrets into `secret_keys`.
  - Image: latest meaningful release for that env (running deploy, else any deploy in env) — same as inspect.
- `Apply(ctx, projectName, selectedEnv string, doc domain.Manifest) (*ApplyReport, error)`
  - `ValidateManifest(doc)` first.
  - Create project / env as specified.
  - Diff and `StageChanges` in one call (one pin).

Do **not** bypass changeset staging (no direct store writes for process/config/image except CreateProject / CreateEnvironment).

---

## Test strategy

- **Unit (domain):** validate version, unknown keys, secret value, deferred keys, process defaults, name rules.
- **Unit (service):** in-memory store — export shape (redacted secrets); apply creates project; apply stages process/config/image diffs; apply no-op when in sync; apply rejects secret values; apply does not prune extra process; apply missing env creates it; apply without `dev` cannot create project; changeset pin conflict.
- **API:** OpenAPI contract includes the two routes; handler tests optional if service tests cover apply/export (prefer 1–2 httptest cases if cheap; otherwise service + e2e is enough).
- **CLI:** parse `-f` / `--stdout` / default filename; YAML marshal round-trip of a fixture (no API).
- **e2e-stub:** CLI export after `launchpad new` + config/process; apply `-f` into a **new** project name; `diff` non-empty; `deploy --wait` succeeds; secret key listed without value in file → `needs_value` and no plaintext in the yaml file.

---

## Docs sync

| Doc | Change |
|-----|--------|
| `docs/DOMAIN.md` | Phase 6 v1 shipped-as-this-PR; CLI table; open question 5 |
| `docs/openapi.yaml` | Two routes + Manifest / ApplyReport schemas |
| `docs/DX-VISION.md` | Active/next; Track A yaml status |
| `docs/DESIGN.md` | Phase 6 next → this PR |
| `README.md` | Solo workflow: export/apply one-liner |
| `QUEUE.md` | `designing` → `implementing` → `pr-open` |

---

## Recommended path (recorded)

**A** — domain document + service + API + CLI. Rejected B (CLI-only) and C (GitOps).

---

## Spec self-review (ADM)

| Check | Result |
|-------|--------|
| 1. No placeholders | Pass — no TBD |
| 2. Internal consistency | Pass — schema, API, CLI, and success criteria agree |
| 3. Single plan scope | Pass — yaml v1 only; MCP is the next QUEUE row |
| 4. No DOMAIN contradiction | Pass — import sets desired state once; not reconciled; v1 current model |
| 5. MVP boundary | Pass — reject `services` / bindings; no OIDC / Helm / scale job |
| 6. Recommended path recorded | A |
| 7. Test strategy | Unit + e2e-stub |
| 8. DoD / acceptance | Success criteria 1–6 |

**Status:** Draft until `adm-spec-review` passes; then `Approved (self-approve — ADM)`.

---

## Open questions

None blocking. Resolved in this spec:

- Apply does not prune, deploy, or patch existing env targets.
- Secret values never in the file; keys only.
- Creating a project via apply requires `environments.dev`.
- Processes are service-scoped; config/image are env-scoped.
