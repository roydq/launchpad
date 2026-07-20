# Unstage last mutation

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
| **Date** | 2026-07-19 |
| **Domain spec** | `docs/DOMAIN.md` — implicit staging / changeset |
| **Scope** | Remove the most recently staged change row from the open changeset |

---

## Goal

Recover from a mistaken stage without discarding the entire pending batch.

```bash
launchpad config set FOO=1
launchpad config set BAR=2
launchpad unstage          # drops BAR stage only
launchpad diff             # still shows FOO
launchpad reset            # still discards everything
```

Success criteria:

- API removes the chronologically last change row (`created_at` desc, `id` tie-break)
- Empty open changeset after last row → still open or discarded is OK; prefer leave empty open (same as today after partial ops); no pin-only “dirty” for env switch (count remains 0)
- No open changeset / no changes → 404
- CLI `launchpad unstage` prints a one-line description of what was removed
- `reset` unchanged (full discard)

---

## Approaches Considered

### A. `DELETE …/changeset/changes/last` + `launchpad unstage` (recommended)

Server deletes one row by max `(created_at, id)`. Return the deleted change in JSON so CLI can summarize. No schema change.

**Pros:** Clear REST; agents can call it; keeps `reset` = full discard  
**Cons:** One row per call (multi-change stage in one POST unstages one row at a time — acceptable)

### B. `launchpad reset --last` only (CLI walks list and rewrites)

**Pros:** No API  
**Cons:** Cannot safely rewrite without delete API; forces GET+DELETE-all+re-stage race

### C. Undo by inverse mutation (stage unset of last config)

**Pros:** Append-only history  
**Cons:** Pollutes fold; wrong for scale/image; not true unstage

**Recommendation:** A

---

## Scope

### In scope

- Store: delete last change for open changeset
- Service + API + OpenAPI + apiclient
- CLI `unstage`
- Unit tests

### Out of scope

- Unstage by key / interactive pick
- Undo already-deployed releases
- Multi-row “logical stage” grouping (same timestamp batch)

### Deferred

- `unstage --all-matching KEY`

---

## Domain impact

| Entity | Change |
|--------|--------|
| ChangesetChange | Delete last row of open changeset |
| Changeset | Remains open if empty; `updated_at` bumped |

**Invariants:**

- Only `status=open` changesets may be unstaged
- Committed/discarded history is immutable
- Last = max `created_at`, then max `id` (UUID string order is fine as stable tie-break; prefer `ORDER BY created_at DESC, id DESC LIMIT 1`)

---

## API sketch

```
DELETE /v1/projects/{project}/changeset/changes/last
→ 200 {
  "id": "…",
  "type": "config",
  "service_name": "…",
  "payload": { … },
  "created_at": "…"
}
→ 404 if no open changeset or no changes
```

Scope: `project:write` (same as stage / discard).

CLI output example:

```
unstaged config FOO=1 (1 pending remaining)
```

or for empty after:

```
unstaged image app:v2 (staging empty)
```

---

## Schema sketch

None.

---

## Target / worker impact

None.

---

## Test strategy

- **Store/service:** stage 3 changes, unstage → 2 remain (last removed); empty → 404
- **API:** route + 404
- **CLI:** smoke via unit if feasible; manual not required

---

## Open questions

None.

---

## Spec self-review (ADM)

1–7 pass. **Status:** Approved (self-approve — ADM)

---

## Approval

- [x] Design reviewed and approved (ADM self-approve)
