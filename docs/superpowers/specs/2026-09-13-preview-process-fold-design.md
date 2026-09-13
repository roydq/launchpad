# Fold process mutations in pending preview

| Field | Value |
|-------|-------|
| **Status** | Draft |
| **Date** | 2026-09-13 |
| **Domain spec** | `docs/DOMAIN.md` — ChangesetChange types; GET `/preview` |
| **Scope** | Fold `process.set` / `process.unset` / `process.apply` in server-side preview so `GET /preview` and `launchpad diff` work after staging process definitions |

---

## Goal

Staging a process definition must not break pending preview. `FoldChanges` currently 400s on unknown types, so `GET /v1/projects/{project}/preview` and `launchpad diff` fail once a `process.set`, `process.unset`, or `process.apply` row is on the open changeset. Image/config/scale-only pending still works.

After this slice, preview and diff show **process definition** deltas versus last deploy (command, quantity, expose, health, target_extensions) — not quantity-only scale lines.

```bash
launchpad process set worker --command "run-worker"
launchpad diff
# HTTP 200; summary includes a Process section with + worker
# GET /v1/projects/{project}/preview → has_pending true, diff.process includes add worker
```

**Success criteria (DoD):**

1. `FoldChanges` accepts `process.set` / `process.unset` / `process.apply` and returns no error for valid payloads.
2. Invalid process payloads (missing name, empty/invalid Procfile JSON) still return `ErrBadRequest` (400).
3. Other unknown change types still 400.
4. `GET …/preview` (pending) after `process.set worker --command run-worker` returns 200, `has_pending=true`, and `diff.process` contains `op=add` (or `change` if worker already exists) with command `run-worker`.
5. `launchpad diff` prints a `## Process` section for that case (CLI uses `summary` from the API).
6. Preview shows command, quantity, expose, health, and `target_extensions` when those fields differ from the last succeeded deploy snapshot — not scale-only.
7. `process.unset` appears as `op=remove`; `process.apply` Procfile is a replace (names in the Procfile vs baseline: add/change; baseline names absent from the result: remove).
8. Scale-type quantity changes stay under `diff.scale` / `## Scale` (existing tests keep passing).
9. Unit tests cover fold, field-level process diffs, apply/unset, and PreviewPending with staged process.set.
10. OpenAPI `Preview` schema documents `pending.processes` and `diff.process`.
11. L0: `mise exec -- make test && make build && go vet ./...`
12. L1: `make e2e-stub` including a case that stages `process.set` then calls preview.
13. No new entities, tables, or routes.

---

## Approaches Considered

### A. Extend FoldChanges + EffectiveDiff with process definition fold/diff (recommended)

Teach `FoldChanges` the three process change types. Collect the same process ops `materializeChanges` already collects (last `process.apply`, all `process.set` in order, all `process.unset` in order). `BuildDiff` / `PreviewPending` apply those ops onto the baseline `process_snapshot` using the same field-merge defaults as `upsertProcessSet` / `applyProcessOps`, then emit `diff.process` plus a `## Process` summary. Keep `diff.scale` for `ChangeTypeScale` quantity-only lines.

**Pros:** Matches “what will change if you deploy now”; one API; CLI `diff` works via `summary`; no new routes; agents and MCP inherit the fix.  
**Cons:** Preview JSON grows; must document scale vs process quantity so clients do not double-count.

### B. Ignore process types in FoldChanges (no 400, no process diff)

Unknown-but-known process types become no-ops in fold. Preview returns 200 with only image/config/scale.

**Pros:** Tiny patch.  
**Cons:** Fails DoD — `launchpad diff` still hides process definition changes. “Diff before trust” stays broken for the runtime-depth CLI.

### C. CLI-only fold of process rows

CLI skips the preview API when process changes are present and prints a local summary.

**Pros:** Avoids API change.  
**Cons:** Agents, MCP `preview`, and `GET /preview` still 400; two dialects; rejected.

**Recommendation:** A. Reject B (incomplete DoD) and C (API remains broken).

---

## Scope

### In scope

- `FoldChanges` / `FoldedPending` / `BuildDiff` / `BuildSnapshotDiff` / `FormatDiffSummary` / `PreviewPending` / `foldedFromRelease` in `internal/service/preview.go`
- `pending.processes` and `diff.process` on the preview JSON
- OpenAPI `Preview` schema
- `pkg/apiclient` Preview structs (e2e + typed clients)
- Unit tests + e2e-stub case
- DOMAIN sentence that pending preview folds process definition changes
- DX-VISION / QUEUE status for this item

### Out of scope (this feature)

- Changing push/`applyProcessOps` order or “cannot remove the last process” (push still 400s; preview may still show the remove)
- New CLI verbs or a second diff formatter
- MCP tool schema changes (preview tool already returns the preview object)
- Live `GET /processes` vs snapshot drift display
- Dry-run deploy / target plan

### Deferred (future phase — do not half-build)

- Multi-service preview, bindings, OIDC
- Rewriting `materializeChanges` process bucketing to true sequential last-write-wins (pre-existing; park in IDEAS if scouted)

---

## Domain impact

No new entities. Changeset types `process.set` / `process.unset` / `process.apply` are already shipped. Preview is an existing read model over the open changeset vs last deploy.

| Entity | Change |
|--------|--------|
| Changeset / ChangesetChange | Unchanged |
| Process / ProcessSnapshot | Unchanged |
| Release | Unchanged |
| Preview DTO | Additive fields only |

**Invariants to preserve:**

- Releases remain immutable; preview does not write.
- Config/image/scale fold last-write-wins unchanged.
- Secret config redaction unchanged (process definitions have no secret values).
- Scale-type rows still populate `pending.scales` / `diff.scale`.

**Invariants to add:**

- Pending preview fold must accept every change type the changeset API can stage today (`config`, `shared_config`, `scale`, `image`, `process.set`, `process.unset`, `process.apply`).
- Process definition preview applies ops in **push order**, not sequential last-write-wins: last `process.apply` (replace from Procfile), then every `process.set` in changeset order, then every `process.unset` in changeset order, then `scale` quantity overrides. This matches `materializeChanges` + `applyProcessOps` so preview is “what push will materialize.”
- `BuildDiff` process ops compare against the last succeeded release `process_snapshot` for the ambient environment (empty snapshot if never deployed).

---

## Fold and diff rules

### Collect (FoldChanges)

On each change, in list order:

| Type | Behavior |
|------|----------|
| `config` / `shared_config` / `scale` / `image` | Unchanged |
| `process.set` | Require name; append `ProcessSetPayload` (partial pointers) |
| `process.unset` | Require name; append name to unsets |
| `process.apply` | Require `procfile`; `ParseProcfile` must succeed; **replace** stored apply payload (last apply wins) |
| anything else | `ErrBadRequest` unknown type |

`FoldedPending.IsEmpty()` is true only when image, config, scales, and process ops (apply/sets/unsets) are all empty.

### Apply onto baseline (PreviewPending / BuildDiff)

Start from `baseline.ProcessSnapshot` (or empty map).

1. **Apply (if present):** parse Procfile; result topology is **only** Procfile entries. Defaults match `ParseProcfile` + `applyProcessOps`: quantity 1 (release → 0), expose `http` for `web` else `none`, empty health/extensions. Baseline names not in the Procfile are gone (unless a later set re-adds them).
2. **Sets (all, in order):** merge onto current topology. New process defaults match `upsertProcessSet`: quantity 1; expose `http` if name is `web` else `none`. Pointer fields overlay. Health `type=none` or empty type clears health (`nil`). `target_extensions` replaced when the payload key is non-nil (including empty map).
3. **Unsets (all, in order):** delete name from topology. Preview does **not** enforce “cannot remove the last process.”
4. **Scale map:** for each `pending.Scales[name]`, set that process’s quantity (create the process with set-defaults if missing, quantity from scale).

### `diff.process`

Each process name in the union of baseline names and effective topology:

| Situation | `op` |
|-----------|------|
| not in baseline, in effective | `add` (`to` = effective snapshot) |
| in baseline, not in effective | `remove` (`from` = baseline snapshot) |
| both, any field differs | `change` (`from` / `to` full snapshots) |
| both, identical | omit |

`fields` on `change` is the sorted subset of `command`, `quantity`, `expose`, `health`, `target_extensions` that differ.

**Quantity vs scale (no double-count in summary/clients):**

- `diff.scale` continues to list `ChangeTypeScale` (and snapshot quantity-only compares) exactly as today.
- Omit a `change` process op when `fields == ["quantity"]` **and** that process already has a `diff.scale` row. Quantity-only `process.set` (not in `pending.scales`) **keeps** the process op.
- `add` / `remove` always emit process ops (quantity lives on the snapshot objects).

### Health and extensions equality

- Health: `nil` equals `nil`; `nil` equals `{type: none}` / empty type; otherwise field equality.
- Extensions: `nil` and empty map are equal; otherwise JSON-object deep equal (unmarshal to `map[string]any`).

### Release and env preview

Same `EffectiveDiff.process` schema.

- `foldedFromRelease`: copy full `to` process snapshots into `pending.processes` (non-null) and keep existing `scales` from quantities. Treat as complete topology (`process_replace` internal flag true) so names only on `from` become removes.
- `BuildSnapshotDiff` (env↔env): union of both snapshots’ process names; emit process ops for add/remove and non-suppressed field changes; keep existing scale quantity lines.

### Summary text

After `## Scale` (existing), when `diff.process` is non-empty:

```
## Process
  + worker command="run-worker" quantity=1 expose=none
  ~ web command: "" → "serve"
  ~ web health: (none) → http /healthz
  - worker
```

One line per op. `change` may use one line per changed field (`~ name field: from → to`) so command vs health stay readable. Empty command displays as `""`. Nil health displays as `(none)`. Extensions display compact JSON.

### `pending` JSON

```json
"pending": {
  "image": "app:v2",
  "config": { "PORT": "8080" },
  "scales": { "web": 3 },
  "processes": {
    "worker": { "command": "run-worker", "quantity": 1, "expose": "none" },
    "legacy": null
  }
}
```

`processes` keys are names **affected** by process ops (sets, unsets, apply result + apply removals). Values are the **effective** snapshot after fold; `null` means unset. Scale-only names stay in `scales`, not duplicated into `processes` unless a process op also touched that name.

`has_pending` is true when fold is non-empty, including process-only staging.

---

## API sketch

No new routes. `GET /v1/projects/{project}/preview` response grows additively.

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/v1/projects/{project}/preview` | Pending vs last deploy (unchanged query modes) |

Failure: invalid staged process payload → `400` `application/problem+json` (`ErrBadRequest`), same as today’s unknown-type 400.

CLI: `launchpad diff` already prints `prev.Summary`. No new flags. `launchpad status` may keep counting process rows as generic pending (optional; not required for DoD).

---

## Schema sketch

No SQL migrations.

Go (service package):

```go
type FoldedPending struct {
    Image             string             `json:"image,omitempty"`
    Config            map[string]*string `json:"config,omitempty"`
    ConfigSensitivity map[string]string  `json:"config_sensitivity,omitempty"`
    Scales            map[string]int     `json:"scales,omitempty"`
    Processes         map[string]*domain.ProcessSnapshot `json:"processes,omitempty"` // null value = unset; API
    // unexported collection used during fold:
    // processApply *domain.ProcessApplyPayload
    // processSets  []domain.ProcessSetPayload
    // processUnsets []string
    // processReplace bool // true after apply or foldedFromRelease
}

type ProcessDiffOp struct {
    Op     string                   `json:"op"` // add | change | remove
    Name   string                   `json:"name"`
    From   *domain.ProcessSnapshot  `json:"from,omitempty"`
    To     *domain.ProcessSnapshot  `json:"to,omitempty"`
    Fields []string                 `json:"fields,omitempty"`
}

type EffectiveDiff struct {
    Image   *ImageDiff      `json:"image,omitempty"`
    Config  []ConfigDiffOp  `json:"config,omitempty"`
    Scale   []ScaleDiffOp   `json:"scale,omitempty"`
    Process []ProcessDiffOp `json:"process,omitempty"`
}
```

`EffectiveDiff.IsEmpty()` includes `len(Process)==0`.

---

## Target / worker impact

None. Preview is control-plane read-only.

---

## Test strategy

- **Unit (`internal/service/preview_test.go`):**
  - `FoldChanges` on `process.set` / `unset` / `apply` does not error; last apply stored; unknown type still errors.
  - `BuildDiff` add worker from `process.set` vs default web-only baseline.
  - `BuildDiff` command/expose/health/extensions change on existing `web`.
  - Quantity-only `scale` → `diff.scale` only (no process op).
  - Quantity-only `process.set` → `diff.process` change with `fields=["quantity"]`.
  - `process.unset` → remove; `process.apply` Procfile replace removes names not in the file.
  - `PreviewPending` after `StageChanges` process.set + image (or process.set alone) returns 200-shaped result with process add and non-empty summary containing `## Process`.
  - Existing fold/diff/env tests still pass.
- **Integration:** none beyond existing in-memory store PreviewPending test.
- **e2e-stub:** create project, `StageChanges` `process.set` worker command, `PreviewPending` succeeds, `HasPending`, process add present (or summary contains worker). Does not require a successful deploy first (empty baseline → add).
- **OpenAPI:** `make openapi-check` after schema update.
- **Persona (CLI UX):** after L1, S1-style: `process set` then `launchpad diff` shows Process; write `docs/superpowers/program/feedback/2026-09-13-preview-process-fold.md` if dogfood runs. If e2e already covers the API, a short CLI invocation against the e2e API is enough.

---

## Docs

- `docs/DOMAIN.md` — Changeset workflow / preview: pending preview folds process.set/unset/apply and diffs definition fields vs last deploy.
- `docs/openapi.yaml` — `Preview` properties for `pending.processes` and `diff.process`.
- `docs/DX-VISION.md` — Active/next points at this spec while in PR; shipped row when merged.
- `docs/superpowers/program/QUEUE.md` — `implementing` / `pr-open` with branch lease.

---

## Open questions

None — fold order matches existing push materialize; quantity suppression rule is specified; last-process unset remains a push-time 400.

---

## Approval

- [ ] Design reviewed and approved (ADM spec self-review / self-approve)
