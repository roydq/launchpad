# Environment clone

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Domain** | Environments + layered config; secrets clone policy (S3) |
| **Scope** | Clone env A → B with plain config copy; secret keys reported as needs_value (no secret material) |
| **Queue** | `env-clone` |
| **Depends** | Secrets S1+S2 shipped |

---

## Goal

```bash
launchpad env clone dev staging
# created environment staging from dev
# cloned plain: PORT, LOG_LEVEL
# needs_value (secrets): DATABASE_URL
# next: launchpad env use staging && launchpad config set --secret DATABASE_URL=... && launchpad deploy
```

---

## Approaches

### A. Server clone endpoint + policy (recommended)

`POST /v1/projects/{project}/environments/{from}/clone` creates B and copies layers in one TX.

### B. CLI-only loop of get/set

**Cons:** Leaks secrets if get returns them; slower; not atomic.

**Recommendation:** A.

---

## Clone policy (from secrets design)

| Source sensitivity | Behavior |
|--------------------|----------|
| `plain` | Copy value to same layer on B |
| `secret` | **Do not copy value.** Record key in `needs_value[]`. No secret row on B (operator must `config set --secret`). |

Target: copy source `target_type` + `target_config` unless request overrides.

Not cloned: releases, deployments, open changesets, process table (shared across envs), images.

---

## API

`POST /v1/projects/{project}/environments/{name}/clone`

Request:

```json
{
  "name": "staging",
  "target": { "type": "stub", "namespace": "default" },
  "ephemeral": false
}
```

`target` optional → copy from source.

Response `201`:

```json
{
  "environment": { "name": "staging", ... },
  "from": "dev",
  "cloned_plain": ["LOG_LEVEL", "PORT"],
  "needs_value": ["DATABASE_URL"],
  "shared_keys": 1,
  "service_keys": 2
}
```

Errors: 404 source, 409 target exists, 400 same name / invalid name.

---

## CLI

`launchpad env clone <from> <to> [--target] [--namespace] [--ephemeral]`

---

## Spec self-review

| Check | Result |
|-------|--------|
| Placeholders | Pass |
| Consistency | Pass |
| Single slice | Pass |
| DOMAIN | Update env clone note |
| MVP | Pass — not multi-service |
| Secrets policy | Matches design (no secret material) |

**Status:** Approved (self-approve — ADM)
