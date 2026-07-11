# Multi-Environment (Phase 2a)

| Field | Value |
|-------|-------|
| **Status** | Approved |
| **Date** | 2026-07-11 |
| **Domain spec** | `docs/DOMAIN.md` |
| **DX vision** | `docs/DX-VISION.md` |
| **Branch** | `feat/multi-env` |
| **Scope** | Ambient multi-env context; env CRUD; per-env config/deploy/staging pin; env-honest reads |

---

## Goal

Make **environment** a first-class ambient context so one project name works across `dev`, `staging`, `production`, etc.—without app-per-env renaming and without half-building layered config, promote, or multi-service.

### Success criteria

```bash
launchpad projects create my-api --target stub
launchpad use my-api
launchpad deploy --image my-api:v1 -m "dev"

launchpad env create staging --target stub --namespace launchpad-staging
launchpad env list
launchpad env use staging

launchpad config set LOG_LEVEL=info
launchpad status          # environment: staging + pending
launchpad diff            # vs last release deployed to staging (none → all additions)
launchpad deploy --image my-api:v1 -m "staging"

launchpad config set X=1  # still on staging… or switch:
launchpad env use dev
launchpad config set Y=1
launchpad env use staging # error if pending batch pinned to dev
launchpad reset           # or deploy first
```

HTTP clients omit `X-Launchpad-Environment` → behavior stays **`dev`** (backward compatible).

---

## Approaches considered

### A. Thread env through resolve + header context (recommended)

- Middleware/helper reads `X-Launchpad-Environment` (default `dev`).
- Replace hardcoded `dev` in service resolve paths.
- Changeset gains `environment_id`; pin on first stage; reject cross-env stage/push.
- CLI sticky env + header on all calls.
- Env CRUD under `/v1/projects/{project}/environments`.
- Releases remain global per service; annotate with per-env deployment state.

**Pros:** Matches DOMAIN ambient context; small URL churn; compatible default.  
**Cons:** Schema migration on changesets; careful pin/mismatch errors.

### B. CLI-only env labels without API multi-env

**Reject** — cannot store or deploy per env.

### C. Path-nested `/environments/{env}/…` for all resources

**Reject** — high churn; worse ambient DX; user preference for header scoping.

**Recommendation:** **A**.

---

## Scope

### In scope

- Multiple environments per project; bootstrap still creates `dev`
- `GET/POST/GET-one` environment APIs
- `X-Launchpad-Environment` on existing project-scoped routes (default `dev`)
- Config get/patch, changeset stage/push/discard, immediate release deploy — all for **resolved env**
- Changeset **pinned to environment**; dirty batch blocks env switch and cross-env mutations
- CLI: `env create|list|use`, sticky `environment` in `~/.launchpad/config`, `LAUNCHPAD_ENV`
- Env-honest reads: `status` shows env + running summary; `releases` annotated with per-env deployment info; `diff` baseline = last release **deployed to current env**
- Docs: DOMAIN, README, DX-VISION link

### Out of scope

- Layered config (workspace / shared) — phase 2b
- Promote — phase 5
- Env clone / copy config — wait for secrets vs config
- Env delete / destroy
- Ephemeral TTL automation
- Project-local `.launchpad/` context files
- Multi-service, bindings, ReleaseSet coordination
- `deploy --wait`, logs, SSE
- Path-nested resource URLs

---

## Product model

### Context stack

| Context | Source | Default |
|---------|--------|---------|
| workspace | API token | — |
| project | `use` / `LAUNCHPAD_PROJECT` | required for most cmds |
| environment | `env use` / `LAUNCHPAD_ENV` / file | `dev` |
| service | (implicit) | `primary_service` |

CLI precedence for environment: **`LAUNCHPAD_ENV` > `~/.launchpad/config` > `dev`**.

### Staging vs environment

- At most **one open changeset per project** (unchanged).
- When the first change is staged, the changeset is **pinned** to the request’s environment (`environment_id`).
- Further stages/push require the request env to **match** the pin.
- `env use` to a different env while pending changes exist → **refused** (CLI; API returns conflict on mismatched stage/push).
- `reset` discards batch and pin; `deploy`/push commits and clears open changeset.

### New environment

- Own `target_type` / `target_config` (same shape as project create target).
- Service config for that env starts **empty**.
- Process topology remains on the **service** (shared definitions).
- First deploy to that env needs an image if no prior release exists for redeploy-from-latest rules; if the service already has a succeeded release from another env, push without a new image may reuse latest service artifact (existing push behavior) while resolving **this env’s** config.

### Diff baseline

Staged vs **last release deployed to the current environment** (prefer deployment in `running`, else latest deployment for that service×env with a release). If never deployed to this env → empty baseline (all staged intent as additions).

Not: global latest service version from another env’s deploys (except artifact reuse on push when no image staged — server materialize rules unchanged).

### Env-honest reads

| Command / API | Behavior |
|---------------|----------|
| `config get` | Current env only |
| `status` | project, **environment**, pending (if any), running deploy/release summary for env |
| `diff` | Pending vs baseline for **current env** |
| `releases` | Full service version timeline + annotations of deployments per env |
| `ps` | Process **definitions** (service-level); not env-specific runtime rows |

---

## Domain impact

| Entity | Change |
|--------|--------|
| Environment | CRUD beyond bootstrap `dev`; unchanged columns |
| Changeset | Add `environment_id` (nullable until pin / set on first stage) |
| Config / Deployment | Already per env — wire resolve path |
| Release | Unchanged versioning; list DTO gains deployment annotations |

**Invariants to preserve**

- One open changeset per project  
- One active deploy per (service, environment)  
- Atomic push TX; snapshot-only worker deploy  
- Release immutability  

**Invariants to add**

- Open changeset with changes has a pinned `environment_id`  
- Stage/push request env must match pin  
- Environment names unique per project; valid name pattern (align with project DNS-label rules)

---

## Schema sketch

Migration (SQLite + Postgres):

```sql
ALTER TABLE changesets ADD COLUMN environment_id /* TEXT/UUID */ REFERENCES environments(id);
-- Backfill: open changesets → project's dev environment id
```

Index: existing unique open-per-project remains.

---

## API sketch

### Header

| Header | Default | Meaning |
|--------|---------|---------|
| `X-Launchpad-Environment` | `dev` | Target env for project-scoped routes |

Unknown env name → **404** problem+json.

### Env CRUD

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/projects/{project}/environments` | List |
| `POST` | `/v1/projects/{project}/environments` | Create |
| `GET` | `/v1/projects/{project}/environments/{name}` | Get |

Create body:

```json
{
  "name": "staging",
  "target": { "type": "stub", "namespace": "default", "cluster": "" },
  "ephemeral": false
}
```

No `DELETE` in 2a.

### Existing routes (behavior change)

| Method | Path | Env effect |
|--------|------|------------|
| GET/PATCH | `…/config` | Current env config |
| GET/POST/DELETE | `…/changeset*` | Pin/match; push deploys to pinned env |
| POST | `…/releases` | Deploy to header env |
| GET | `…/releases` | Timeline + per-env deployment annotations |
| GET | `…/processes` | Definitions only |

### DTO additions

Changeset:

```json
{
  "environment": "staging",
  "changes": []
}
```

Release list item (sketch): include `deployments` array or map of `{ "environment", "status", "deployment_id" }` for known deployments of that release (or latest per env)—implementation may choose compact form as long as CLI can show where a version ran.

### Errors

| Case | Code |
|------|------|
| Unknown environment | 404 |
| Stage/push env ≠ pin | 409 Conflict |
| Duplicate env name | 409 |
| Invalid env name | 400 |

---

## CLI sketch

```bash
launchpad env list
launchpad env create staging --target stub --namespace launchpad-staging
launchpad env use staging
# optional: launchpad env          # print current
```

`~/.launchpad/config`:

```json
{ "project": "my-api", "environment": "staging" }
```

All API calls send `X-Launchpad-Environment`.

Dirty switch message (family of dirty `--now`):

```text
cannot switch environment: N pending change(s) for dev; run "launchpad deploy", "launchpad diff", or "launchpad reset"
```

---

## Architecture

```
CLI (sticky env, header)
    → apiclient
        → API middleware (resolve env name)
            → service.resolvePrimary(project, envName)
                → store (config/deploy/changeset by env)
```

Worker: no intentional change if jobs already load deployment → environment → target config. Verify and fix only if hardcoded `dev`.

### Shared resolve

```text
resolvePrimary(ctx, projectName, envName) → (Project, Service, Environment)
```

Used by config, changeset, release, process list (project/service only need env where applicable).

---

## Target / worker impact

- **None expected** if deploy path already uses `deployment.EnvironmentID` for target config.
- Verify K8s namespace comes from env’s `target_config`, not project-global assumptions.

---

## Test strategy

- **Store:** second env; unique constraint; changeset pin column + backfill  
- **Service:** stage/push on staging only touches staging config; header/pin mismatch → conflict; create env  
- **API:** default header `dev`; unknown env 404  
- **CLI:** env precedence unit tests; dirty `env use` guard  
- **Diff:** baseline helper with per-env last deployment  
- **e2e-stub (optional but valuable):** create staging, deploy dev + staging  

---

## Docs updates (with implementation)

- `docs/DOMAIN.md` — multi-env workflow; CLI; phase table note 2a  
- `README.md` — second env examples  
- `docs/DX-VISION.md` — link this spec; secrets-before-clone note  
- `docs/DESIGN.md` — header + env routes  

---

## Open questions

None remaining for 2a — resolved in brainstorming:

- Scope B (writes + env-honest reads)  
- Sticky global env (project-local later)  
- Changeset pin + block switch  
- Header API scoping  
- Empty config on create; no clone until secrets  
- Dual-view releases; diff baseline per env  

---

## Approval

- [x] Design reviewed and approved in brainstorming (2026-07-11)
- [ ] Spec file reviewed by user before implementation plan
