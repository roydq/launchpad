# Release Invariants & Control-Plane Correctness

| Field | Value |
|-------|-------|
| **Status** | Draft (awaiting approval) |
| **Date** | 2026-07-09 |
| **Domain spec** | `docs/DOMAIN.md` |
| **Scope** | Close MVP invariant debt: snapshot-driven deploy, atomic push, API wire shape, target surface |

---

## Goal

Make the **implemented** control plane match the domain's core promises:

1. A release is the **only** desired-state input for deploy.
2. Push and immediate release share one **atomic** materialize → snapshot → job path.
3. HTTP clients see a **stable snake_case** contract (no PascalCase domain leaks).
4. Docs describe MVP honestly (what is done vs. deferred).

### Success criteria

```bash
# Snapshot is what deploys (regression test, not only CLI)
# 1. Stage config + image, push
# 2. Mutate live config *after* job is queued but *before* worker runs
# 3. Deployed release still uses config snapshotted at enqueue time

# Atomic push (regression test)
# 1. Force release enqueue to fail after config would have been applied
# 2. Open changeset still open; live config/process rows unchanged

# Wire format
curl -s .../changeset | jq 'has("changes") and (has("Changes") | not)'
```

Behavioral CLI loop unchanged:

```bash
launchpad projects create my-api --target stub
launchpad use my-api
launchpad config set PORT=3000
launchpad changeset add --image my-api:v1
launchpad changeset push
launchpad releases
```

---

## Approaches Considered

### A. Enforce invariants in service + worker; docs + API DTOs (recommended)

- Expand `ProcessSnapshot` to full deployable process state (`command`, `quantity`, `expose`).
- Worker builds `DeployRequest` **only** from release (+ identity entities).
- Single transaction for push: apply config/scale → create release/deployment/job → commit changeset.
- Immediate `POST /releases` already one TX; share enqueue helper with push.
- Introduce API response DTOs / json tags at the edge; type `apiclient`.
- Narrow documented MVP target surface to deploy-centric; keep unused methods as stubs until phase work.

**Pros:** Deletes dual source of truth; matches DOMAIN principles; small surface area.  
**Cons:** Slightly larger process snapshot JSON; requires worker + service tests.

### B. Keep live reload; document “best effort snapshot”

Treat snapshot as audit-only; worker always reads live tables.

**Pros:** Minimal code.  
**Cons:** Violates stated product model; breaks rollback/promote story; reject.

### C. Event-sourced desired state only (no live process/config tables)

All desired state only on releases; no mutable process/config rows.

**Pros:** Pure model.  
**Cons:** Out of MVP scope; loses easy “current desired config” without scanning releases.

**Recommendation:** A.

---

## Scope

### In scope

- Full process snapshot fields needed for deploy
- Worker deploy uses release snapshot only
- Atomic push (same TX as release enqueue)
- Active-deploy concurrency: enforce inside TX; map unique-index conflicts to `ErrConflict`
- Supersede previous `running` deployment when a new one enters `deploying` (same worker TX as status advance)
- API wire: snake_case DTOs for list/get of processes, releases, jobs, changesets
- Typed `pkg/apiclient` responses for those endpoints; drop CLI `Changes`/`changes` dual-path
- Doc updates: `DOMAIN.md`, `DESIGN.md`, this spec + plan
- Tests proving snapshot isolation and atomic push
- Cleanups: `errors.Is`, remove pass-through wrappers, auth problem helper reuse where cheap

### Out of scope (this feature)

- Multi-environment, multi-service, ReleaseSet, bindings
- Immediate `scale` / `rollback` / `promote` APIs
- Removing Scale/Destroy/Rollback/Logs from Go `Target` interface (document MVP uses Deploy only; implementations may keep no-ops)
- Encrypting config at rest, OpenAPI, SSE, idempotency keys
- Renaming all auth `TeamID` → `WorkspaceID` (optional small rename if touched; not required for merge)

### Deferred (do not half-build)

- Layered config resolution
- Coordination modes / atomic multi-service
- Target method split into optional interfaces (can follow once Scale/Logs APIs exist)

---

## Domain impact

| Entity | Change |
|--------|--------|
| `ProcessSnapshot` | Expand: `command`, `quantity`, `expose` (not quantity-only) |
| `Release` | Unchanged columns; snapshot JSON richer; **deploy must use it** |
| `Deployment` | Supersede previous running on new `deploying` (behavior) |
| `Changeset` | Commit only in same TX as release materialization |
| `Project.status` | Document as **cached aggregate** (stored), not pure derived query |
| Target `DeployRequest` | Conceptually: identity + release; `Processes`/`Config` are **derived from release** for target convenience |

**Invariants to preserve:**

- One open changeset per project
- One active (`pending`/`deploying`) deployment per service×env (partial unique index already exists)
- Release versions monotonic per service

**Invariants to enforce in code (already claimed in DOMAIN):**

- Releases are immutable after create
- Deploy does not re-resolve or re-read live config/process tables
- Push materialization is atomic with release + job creation

**Product decisions locked:**

1. **Deploy source of truth:** release snapshot only.  
2. **Runtime mutations that change what runs:** go through a **new release** (changeset push or immediate deploy). Live config/process tables are the **staging ground for the next snapshot**, not what the worker applies.  
3. **Scale without image:** still creates a release (artifact from latest succeeded release) — same as current push behavior; no `Target.Scale` from control plane in MVP.  
4. **Project status:** stored cache updated on enqueue and deploy terminal transitions.  
5. **Release status:** reflects the deployment created with that release (MVP: one deploy attempt per release creation). Multi-env semantics deferred; document for phase 2.  
6. **Promotion (future):** copies artifact + process_snapshot only; re-resolves all config layers in target env (no copy of `config_resolved`).

---

## API sketch

No new paths. Behavior and wire format only.

| Method | Path | Change |
|--------|------|--------|
| `POST` | `.../releases` | Unchanged path; already atomic; uses full process snapshot |
| `POST` | `.../changeset/push` | Atomic with materialize; 409 if active deploy |
| `GET` | `.../changeset` | Response DTO: snake_case (`id`, `project_id`, `status`, `changes`, …) |
| `GET` | `.../releases` | snake_case release DTO |
| `GET` | `.../processes` | snake_case process DTO |
| `GET` | `.../jobs/{id}` | snake_case job DTO |

Errors remain RFC 7807. Map unique-index active-deploy conflicts to **409 Conflict**.

---

## Schema sketch

No migration required if `process_snapshot` / `config_resolved` remain JSON blobs (they do).  
Optional: none for this feature.

---

## Target / worker impact

### Worker

1. Load deployment + release + project/service/environment identity.  
2. Build `Processes` from `release.ProcessSnapshot` (not `ListProcesses`).  
3. Build `Config` from `release.ConfigResolved` (not `ListConfigVars`).  
4. `pending → deploying`; supersede any previous `running` for same service×env.  
5. `Target.Deploy`.  
6. Terminal success/fail updates (existing).

### Target

- `DeployRequest` may keep `Processes` and `Config` fields so backends stay simple, but **callers must populate them from the release**. Document that contract.  
- Prefer not to break k8s/stub signatures in this PR beyond using command from snapshot when present.

---

## Test strategy

- **Unit / service:**  
  - `PushChangeset` rolls back config and leaves changeset open if release enqueue fails (inject via forcing active deploy already in TX, or store-level failure).  
  - `buildProcessSnapshot` includes command/expose.  
- **Worker:**  
  - After enqueue, mutate live config; deploy result / target request still sees snapshotted config (stub target inspection or deployContext assertion).  
  - Supersede: running v1 → new deploy → v1 `superseded`, v2 `running`.  
- **Store:** unique index still enforced; conflict mapping if added.  
- **API/client:** JSON keys snake_case for changeset list.  
- **Smoke:** `scripts/smoke-stub.sh` still passes.

---

## Open questions

- [x] Deploy from live tables? **No — release only.**  
- [x] Scale creates a release? **Yes (via push / future --now); no live-only scale in MVP.**  
- [x] Project status derived vs stored? **Stored cache.**  
- [x] Shrink Target interface now? **No — document MVP uses Deploy; shrink when Scale/Logs APIs land.**  
- [x] Migration for snapshot shape? **No — JSON blob, code-only.**

---

## Approval

- [ ] Design reviewed and approved (required before implementation)
- [ ] DOMAIN.md / DESIGN.md updated on feature branch with this design
