# Implicit Changeset CLI (Stage by Default)

| Field | Value |
|-------|-------|
| **Status** | Approved |
| **Date** | 2026-07-09 |
| **Domain spec** | `docs/DOMAIN.md` |
| **Scope** | CLI-only DX: auto-stage mutations, `diff`/`status`/`reset`/`deploy`; API keeps changeset |

---

## Goal

Make changeset **batching implicit** in the CLI. Users make changes, review a diff against the last release, and submit with `deploy`. They should not need a `changeset` subcommand on the happy path.

The control-plane **API and domain entity remain “changeset.”** This feature is presentation and composition in the CLI (and `pkg/apiclient` helpers as needed).

### Success criteria

```bash
launchpad projects create my-api --target stub
launchpad use my-api

# Stage by default (no changeset command)
launchpad config set PORT=3000
launchpad image my-api:v1
launchpad scale web=2

launchpad status          # pending summary
launchpad diff            # staged vs last release

launchpad deploy -m "initial"
launchpad releases

# One-shot: append mutations then submit
launchpad deploy --image my-api:v2 PORT=8080 -m "bump"

# Immediate (clean staging only)
launchpad config set DEBUG=true --now -m "debug on"

# Dirty staging blocks --now
launchpad config set A=1
launchpad config set B=2 --now   # error: staging not clean
launchpad reset
```

**Remove** the `launchpad changeset` command group entirely (project is greenfield; no aliases).

---

## Approaches Considered

### A. Thin CLI composer over existing changeset API (recommended)

Keep store, service, worker, and REST paths unchanged. Rewrite the CLI command map:

- Mutation commands → `POST …/changeset/changes`
- `deploy` → optional stage + `POST …/changeset/push`
- `diff` → client-side delta from `GET …/changeset` + latest release
- `reset` → `DELETE …/changeset`
- `--now` → require clean staging, then stage-these + push

Shared helpers (`stage`, `loadPending`, `requireCleanStaging`, `push`, `diffPending`) keep one-shot and multi-step on one path.

**Pros:** Small blast radius; reuses atomic push; API vocabulary stays accurate; easy to test.  
**Cons:** Diff logic lives in CLI until a future preview API.

### B. CLI + server-side preview/diff API

Same as A, plus `GET …/changeset/diff` (or equivalent) computed in the service layer.

**Pros:** Reusable by TUI/MCP/agents.  
**Cons:** Out of “CLI only” for this slice; more surface area.

### C. Live tables as working tree; drop CLI use of changeset

`config set` writes live rows; `diff` is live vs last release; `deploy` snapshots live state.

**Pros:** Matches one DOMAIN phrasing of “live tables as staging ground.”  
**Cons:** Undercuts the shipped changeset engine and atomic push story; higher risk. Reject for this feature.

**Recommendation:** **A.** Optionally revisit **B** later for reuse.

---

## Scope

### In scope

- CLI command map: stage-by-default mutations, `diff`, `status`, `reset`, `deploy` with optional one-shot mutations
- `--now` on mutation commands only (not on `deploy`)
- Block `--now` when any pending staged changes exist
- Client-side `diff` (staged vs last release)
- Shared CLI helpers for stage → push
- Remove `changeset` CLI subcommands (no deprecation aliases)
- Update README and DOMAIN CLI examples (entity “Changeset” remains)

### Out of scope (this feature)

- Renaming API paths or domain entity “Changeset”
- Server-side diff/preview endpoint
- Worker, target, or release snapshot behavior changes
- Multi-service / multi-environment deploy UX
- TUI or MCP surface
- JSON output flags unless already trivial to add

### Deferred (future — do not half-build)

- Preview/diff API for agents and non-CLI clients
- Multi-service staging awareness in `diff`/`status`
- Separate “working tree” model that collapses live tables and changeset

---

## Product model

### Staging model (B)

- **Default:** every normal mutation only updates the open project changeset.
- **Submit:** `launchpad deploy` materializes via existing push (release + deployment + job).
- **Immediate:** `--now` on mutations creates a release immediately **only if** staging is empty; implemented as stage-these-changes + push (same materialize path).
- **No `deploy --now`:** `deploy` already means “make it real.”

### Vocabulary

| User-facing | Meaning |
|-------------|---------|
| Pending / staged | Open changeset changes |
| `diff` | Staged vs **last release** |
| `status` | Short pending summary |
| `deploy` | Submit (push) |
| `reset` | Discard open changeset |
| Changeset | API/domain term only |

### State layers

| State | Storage | CLI readers |
|-------|---------|-------------|
| Pending batch | Changeset + ChangesetChange | `diff`, `status`, `deploy` |
| Live tables | config_vars / processes | Updated on push; `config get` |
| Last release | releases snapshot | `diff` baseline; worker source of truth |

CLI does **not** treat live tables as the staging area. Staging always goes through the changeset API.

---

## CLI command map

| Command | Behavior |
|---------|----------|
| `config set KEY=VAL…` | Stage config changes |
| `config unset KEY…` | Stage config deletes (`value: null` on changeset change payload) |
| `config` / `config get` | Show **live** (applied) config — not pending |
| `scale PROC=N…` | Stage scale changes |
| `image <ref>` | Stage image change only |
| `diff` | Print staged vs last release |
| `status` | Pending change count/summary + hints |
| `deploy` | Optional one-shot mutations, then push |
| `reset` | Discard open changeset |
| `* --now` on mutations | Immediate: require clean staging → stage these → push |
| **Removed** | `changeset add\|status\|push\|reset` |

### `deploy` flags and args

| Input | Meaning |
|-------|---------|
| `-m, --message` | Release description (push description) |
| `--image <ref>` | Append image change, then push |
| `--scale proc=n` | Append scale change(s), then push |
| `KEY=VAL` args | Append config changes, then push |

**Algorithm:**

```
if any mutation flags/args:
  stage(mutations)
pending = loadPending()
if pending is empty:
  fail "nothing to deploy"
push(message)
```

### `--now` on mutations

Applies to: `config set`, `config unset`, `scale`, `image`.

```
if loadPending() is non-empty:
  fail with guidance: diff | deploy | reset
stage(only the mutations from this command)
push(message)
```

### Edge cases

| Situation | Behavior |
|-----------|----------|
| `deploy` with empty staging and no mutation inputs | Error: nothing to deploy |
| `--now` while pending exists | Error: staging not clean |
| First release (no prior release) | `diff` shows staged intent as additions; deploy still works |
| Open changeset with zero changes | Treat as nothing pending |
| Stage same value as last release | Allowed; diff may show no effective delta or annotate unchanged |
| Multiple stages of same key | Last write wins (existing server accumulation) |
| `reset` with nothing open | Friendly no-op success |
| Active deploy conflict on push | Surface existing API 409 |

---

## Architecture

```
CLI (cmd/launchpad)
  shared helpers: stage, loadPending, requireCleanStaging, push, diffPending
        │
        ▼
pkg/apiclient  →  existing REST
  GET    /v1/projects/{project}/changeset
  POST   /v1/projects/{project}/changeset/changes
  DELETE /v1/projects/{project}/changeset
  POST   /v1/projects/{project}/changeset/push
  GET    /v1/projects/{project}/releases  (or equivalent latest)
  GET    config as needed for `config get`
```

**No** service/worker/schema changes required for the core design.

`PATCH /config` (if present) is not used by the stage-by-default CLI mutation path. Immediate and batched releases both go through changeset stage + push so materialization stays single-pathed.

### Shared helpers (maintainability)

One internal CLI module (name flexible) owns:

1. **stage(project, []Change)** — POST changes  
2. **loadPending(project)** — GET changeset; empty if none/open-with-zero  
3. **requireCleanStaging(project)** — error if pending non-empty  
4. **push(project, message)** — POST push  
5. **diffPending(project)** — pending + last release → printable delta  

`deploy` and `--now` only compose these helpers.

### `diff` algorithm (client-side)

**Baseline:** latest release for the MVP primary service (prefer last **succeeded** if the API distinguishes; otherwise latest by version). If none, baseline is empty.

**Pending:** fold changeset changes in order (last write wins per config key / process scale / image).

**Sections** (omit empty):

1. **Image** — `old → new` when staged image differs from release `artifact_ref`  
2. **Config** — keys added, changed, or removed vs release `config_resolved`  
3. **Scale / process** — quantity (and other staged process fields if present) vs release `process_snapshot`

**Empty pending:** print “No pending changes” (exit 0).

### `status` content

- Current project (and env if shown elsewhere)  
- Pending: change count and simple type breakdown when cheap  
- Hint: `diff` to review, `deploy` to apply, `reset` to discard  

Do not overload with full deploy health (`ps` / releases remain separate).

### Output tone

- Stage commands: brief confirmation (`Staged config PORT`, `Staged image my-api:v2`)  
- `deploy` / `--now`: release version + deployment/job identifiers from push response  
- `diff`: human-readable text (default)

---

## Domain impact

| Entity | Change |
|--------|--------|
| Changeset | Unchanged (API/store) |
| ChangesetChange | Unchanged |
| Release / Deployment | Unchanged materialize path |
| CLI surface | Primary change |

**Invariants to preserve:**

- At most one open changeset per project  
- Push materialization remains one transaction (existing release-invariants work)  
- Deploy worker reads release snapshot only  
- Releases are immutable  

**Docs-only domain note:**

- Update DOMAIN “Changeset workflow” **CLI examples** to implicit staging + `deploy`  
- State explicitly that the HTTP API still uses `/changeset*`  

---

## API sketch

**No new routes.** Existing:

| Method | Path | CLI use |
|--------|------|---------|
| `GET` | `/v1/projects/{project}/changeset` | status, diff, requireClean, deploy guard |
| `POST` | `/v1/projects/{project}/changeset/changes` | config/scale/image stage |
| `DELETE` | `/v1/projects/{project}/changeset` | reset |
| `POST` | `/v1/projects/{project}/changeset/push` | deploy, `--now` |
| `GET` | releases listing/detail | diff baseline |
| `GET` | config | `config get` live view |

User-facing CLI errors map from existing problem+json where applicable; add clear CLI messages for dirty `--now` and empty `deploy`.

---

## Schema sketch

None.

---

## Target / worker impact

None.

---

## Test strategy

- **Service/store:** Existing changeset push/atomicity tests remain; no required changes  
- **apiclient:** Cover get/stage/push/delete as needed for CLI  
- **CLI:**  
  - `config set` stages (GET changeset shows change)  
  - `diff` output against fixture last release + staged ops  
  - `deploy` with flags stages then pushes  
  - empty `deploy` fails  
  - `--now` succeeds when clean  
  - `--now` fails when dirty  
  - `reset` clears pending  
  - `changeset` subcommand not registered  
- **Smoke / README:** Replace `changeset add/push` with `config set` / `image` / `deploy`  

---

## Future work (not this feature)

- Server-side `GET …/changeset/diff` (or preview) for MCP/TUI reuse  
- Richer `status` integrating deployment health  
- Multi-service pending view and coordination flags on `deploy`

---

## Open questions

None remaining — resolved in brainstorming:

- Stage by default with `--now` escape hatch  
- Git-flavored review verbs; `deploy` as submit  
- Diff vs last release  
- One-shot mutations on `deploy` via shared helpers  
- CLI-only; API keeps changeset; no CLI aliases  
- Block `--now` when pending  

---

## Approval

- [x] Design reviewed and approved in brainstorming (2026-07-09)
- [ ] Spec file reviewed by user before implementation plan
