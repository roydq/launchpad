# Diff env↔env (release archaeology)

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Domain spec** | `docs/DOMAIN.md` — multi-env deployments, release archaeology |
| **Scope** | Compare last-deployed release snapshots between two environments |

---

## Goal

Trust multi-env by answering: **what differs between what is running (last deployed) in env A vs env B?**

```bash
# Compare last deploy in staging vs production
launchpad diff --from-env staging --to-env production

# Agents / CI
GET /v1/projects/{project}/preview?from_env=staging&to_env=production
```

Success criteria:

- API returns structured diff (image / config / scale) of latest release per env
- Secrets redacted (same rules as pending/release preview after S1)
- CLI prints human summary with env names + release versions
- Same env or missing query pair → 400
- Unknown env name → 404
- Env never deployed → treated as empty snapshot (not an error)

---

## Approaches Considered

### A. Extend `GET …/preview` with `from_env` / `to_env` (recommended)

Add query params alongside existing `from_release` / `to_release`. Mode `"environments"`. Load each env’s latest deployment release via existing `GetLatestReleaseForEnvironment`, then **full snapshot comparison** (union of config keys and processes — not pending-style “only staged keys”).

**Pros:** One preview surface for agents; reuses redaction + `EffectiveDiff`; no new entity  
**Cons:** Preview handler gains a third mode branch

### B. CLI-only: list releases with deployment annotations and client-side diff

**Pros:** No API change  
**Cons:** Agents reimplement fold; violates server-side preview principle from `2026-07-15-server-diff-preview`

### C. New path `GET …/diff/environments`

**Pros:** Clear resource  
**Cons:** Extra surface; preview already owns structured diff

**Recommendation:** A

---

## Scope

### In scope

- Server: `PreviewEnvironments` + full snapshot `BuildSnapshotDiff`
- API: `from_env` + `to_env` on existing preview route (mutually exclusive with release pair and with bare pending)
- CLI: `--from-env` / `--to-env` on `launchpad diff`
- OpenAPI + DOMAIN CLI table
- Unit tests (service + CLI flag mutual exclusion)

### Out of scope (this feature)

- Live layer config diff without deploy (compare *resolved live* vs *deployed*)
- Env clone, promote, or multi-service
- Diffing open pending across envs

### Deferred

- Side-by-side process logs / inspect merge

---

## Domain impact

| Entity | Change |
|--------|--------|
| Deployment / Release | Read-only: latest per service×env |
| Changeset | Unchanged |
| Preview API | New mode `environments` |

**Invariants to preserve:**

- Diff is archaeology of **deployed releases**, not live unstaged layers
- Secret values never returned in clear text in preview

**Invariants to add:**

- `from_env` and `to_env` must differ and both exist in the project
- Env pair and release pair are mutually exclusive

---

## API sketch

```
GET /v1/projects/{project}/preview?from_env=staging&to_env=production
Authorization: Bearer …
# X-Launchpad-Environment ignored for this mode (both envs explicit)
```

Response (extends existing `Preview`):

```json
{
  "mode": "environments",
  "from_environment": "staging",
  "to_environment": "production",
  "from_version": 2,
  "to_version": 5,
  "has_pending": false,
  "matches_baseline": false,
  "diff": {
    "image": { "from": "app:v1", "to": "app:v2" },
    "config": [
      { "op": "change", "key": "PORT", "from": "3000", "to": "8080", "sensitivity": "plain" },
      { "op": "add", "key": "NEW", "to": "y", "sensitivity": "plain" },
      { "op": "remove", "key": "OLD", "from": "x", "sensitivity": "plain" }
    ],
    "scale": [
      { "process": "web", "from": 1, "to": 3 }
    ]
  },
  "summary": "## Image\n  app:v1 → app:v2\n..."
}
```

Notes:

- `from_version` / `to_version` omitted when that env has never been deployed
- Empty both sides → empty `diff`, summary `No differences\n`
- Identical snapshots → `matches_baseline: true`, summary `No differences\n`
- Config values redacted to `[secret]` / sentinel when sensitivity is secret on either side

Errors:

| Case | Status |
|------|--------|
| Only one of from_env/to_env | 400 |
| from_env == to_env | 400 |
| Env pair + release pair both set | 400 |
| Unknown environment name | 404 |

---

## Schema sketch

None (no migrations).

---

## Target / worker impact

None.

---

## Test strategy

- **Unit (service):** two envs with different releases → image/config/scale ops; secret redaction; same env 400; never-deployed empty side
- **Unit (handler / OpenAPI):** query dispatch
- **CLI:** flag mutual exclusion with `--from-release`
- **Smoke:** not required (no deploy path change); optional e2e later

---

## Open questions

None — self-review passed.

---

## Spec self-review (ADM)

1. No placeholders — pass  
2. API / success criteria agree — pass  
3. Single vertical slice — pass  
4. No DOMAIN contradiction (docs CLI table update only) — pass  
5. MVP boundary OK — pass  
6. Recommended path recorded — pass  
7. Test strategy present — pass  

**Status:** Approved (self-approve — ADM)

---

## Approval

- [x] Design reviewed and approved (ADM self-approve)
