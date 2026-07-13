# Promote (Wave 3)

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve, spike program) |
| **Date** | 2026-07-13 |
| **Domain spec** | `docs/DOMAIN.md` — Promotion, invariant 13 |
| **Scope** | Cross-env promote: artifact + process topology; re-resolve config in target env |

---

## Goal

Promote a **succeeded** release that was applied in a source environment into a target environment by creating a **new** service release and enqueueing a deployment to the target. Config is **re-resolved** in the target environment — never copied from the source release.

```bash
# sticky prod context
launchpad env use production
launchpad promote --from staging --wait

# explicit (CI)
launchpad promote --from staging --to production --release 12 -m "ship" --wait
```

Success criteria:

- `POST /v1/projects/{project}/promote` returns `202` with new release, deployment (target env), job
- New release has source `artifact_ref` + `process_snapshot`
- New release `config_resolved` equals `ResolveConfig` for the **target** env (differs when layers differ)
- CLI `launchpad promote` wires flags and optional `--wait`

---

## Approaches Considered

### A. Mirror Rollback via `enqueueRelease` (recommended)

Service method `Promote` loads source release, validates, resolves target config, builds `releasePlan` with source artifact + process snapshot, calls existing `enqueueRelease` for target env.

**Pros:** Reuses TX/job/active-deploy path; worker unchanged; matches DOMAIN  
**Cons:** Slightly stricter source rules than current Rollback

### B. Copy full release including `config_resolved`

**Pros:** Bit-identical  
**Cons:** Violates invariant 13; prod gets staging config — **reject**

### C. New job type / reuse same release ID

**Pros:** None meaningful  
**Cons:** Worker complexity or dual configs on one immutable snapshot — **reject**

**Recommendation:** A

---

## Scope

### In scope

- `ReleaseService.Promote` + service tests (config re-resolution is the critical assertion)
- `POST /v1/projects/{project}/promote`
- apiclient + CLI `promote` with `--from`, `--to`, `--release`, `-m`, `--wait`
- DOMAIN / DX-VISION status updates when shipping

### Out of scope

- Multi-service / `--service` / ReleaseSet
- Bindings, workspace config layer, secrets
- Dry-run / preview / approval gates
- Blocking on dirty changesets
- Aligning Rollback’s succeeded-check (optional follow-up)
- New worker job types

### Deferred

- Multi-service coordinated promote
- Env clone / ephemeral promote pipelines

---

## Domain impact

| Entity | Change |
|--------|--------|
| Release | New row on promote (new monotonic version); unchanged source |
| Deployment | New deployment on **target** env |
| Job | Standard `deploy` job |
| Config / Process live tables | Unchanged |

**Invariants to preserve:**

- Releases immutable after create
- Deploy applies only release snapshot
- Invariant 13: promote preserves artifact + process topology; config fully re-resolved

**Selection rules (normative for this feature):**

1. Source release must exist and `status == succeeded`
2. Source must have a deployment in `from` with status `running` or `superseded`
3. `from != to`
4. Optional `version`; if omitted, use release of latest `running` deployment in `from`
5. Ambient `X-Launchpad-Environment` defaults **`to`** when body omits `to`

---

## API sketch

```
POST /v1/projects/{project}/promote
Scope: deploy
202 Accepted → same shape as rollback/push (release + deployment + job)

{
  "from": "staging",           // required
  "to": "production",          // optional; else header env / default dev
  "version": 12,               // optional; else running in from
  "description": "..."         // optional
}
```

Errors: `400` (same env, failed source, never deployed to from, no running when version omitted), `404` (project/env/release), `409` (active deploy on target).

---

## CLI

```
launchpad promote --from <env> [--to <env>] [--release N] [-m desc] [--wait] [--timeout]
```

- `--to` or ambient env → target
- `--from` required
- Reuse wait helpers from rollback

---

## Test strategy

1. **Service:** two envs, distinct config layers; promote; assert artifact/snapshot match source, config_resolved matches target ResolveConfig and differs from source
2. **Service:** process snapshot from source not live table after mutation
3. **Service:** rejects from==to, failed source, active deploy on to
4. **Service:** default version = running in from
5. Build/vet/full test suite

---

## Self-review checklist

- [x] No open TBDs on happy path
- [x] Scope is one plan / one PR
- [x] No contradiction with DOMAIN promote semantics
- [x] Deferred multi-service / bindings not half-built
