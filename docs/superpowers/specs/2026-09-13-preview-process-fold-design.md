# Fold process mutations in pending preview

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — ADM) |
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
2. Invalid process payloads still return `ErrBadRequest` (400): `process.set`/`unset` missing name; `process.apply` missing/empty `procfile` string, or Procfile **text** that fails `domain.ParseProcfile`.
3. Other unknown change types still 400.
4. `GET …/preview` (pending) after `process.set worker --command run-worker` returns 200, `has_pending=true`, and `diff.process` contains an entry with `op=add` (or `change` if worker already exists), `name=worker`, and `to.command=run-worker`. Unit and e2e assert this JSON — not merely that `summary` contains the word `worker`.
5. `launchpad diff` prints a `## Process` section for that case (CLI uses `summary` from the API).
6. Preview shows command, quantity, expose, health, and `target_extensions` when those fields differ from the last **deploy** snapshot (same baseline as today’s `PreviewPending`: latest deployment for the env, any status) — not scale-only.
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

- `FoldChanges` / `FoldedPending` / `BuildDiff` / `BuildSnapshotDiff` / `FormatDiffSummary` / `formatEffectiveDiff` / `PreviewPending` / `PreviewReleases` / `PreviewEnvironments` / `foldedFromRelease` in `internal/service/preview.go`
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
- Process definition preview applies ops in **push buckets** (not sequential last-write-wins): last `process.apply` (replace from Procfile), then every `process.set` in changeset order, then every `process.unset` in changeset order, then `scale` quantity overrides **only on names that still exist** in that topology. Those buckets match `materializeChanges` + `applyProcessOps` + `UpdateProcessQuantity`. Preview **diffs that result against the last-deploy `process_snapshot`** (empty if never deployed) — it is not a guarantee of bit-identical push materialization (push writes the live `processes` table, still 400s on last-process unset, and live-table drift is out of scope). `scale` never creates a process (push 404s via `UpdateProcessQuantity` if the name is missing). DOMAIN changeset LWW (“later changes to the same key override”) continues to apply to config/image/scale keys; process definition types are the documented exception (bucketed, same as push).
- Process diffs compare against the last **deploy** `process_snapshot` for the ambient environment (empty snapshot if never deployed) — same `GetLatestReleaseForEnvironment` as today’s pending preview.

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

### Apply onto baseline (pending preview only)

`PreviewPending` computes an **effective topology** with a pure function, e.g. `applyProcessOpsToSnapshot(baseline.ProcessSnapshot, folded)`. It starts from `baseline.ProcessSnapshot` (or empty map). It reads **unexported fold ops**, not `FoldedPending.Processes`.

1. **Apply (if present):** parse Procfile; result topology is **only** Procfile entries. Defaults match `ParseProcfile` + `applyProcessOps`: quantity 1 (release → 0), expose `http` for `web` else `none`, empty health/extensions. Baseline names not in the Procfile are gone (unless a later set re-adds them).
2. **Sets (all, in order):** merge onto current topology. New process defaults match `upsertProcessSet`: quantity 1; expose `http` if name is `web` else `none`. Pointer fields overlay. Health `type=none` or empty type clears health (`nil`). `target_extensions` replaced when the payload key is non-nil (including empty map).
3. **Unsets (all, in order):** delete name from topology. Preview does **not** enforce “cannot remove the last process.”
4. **Scale map:** for each `pending.Scales[name]`, if that name **already exists** in the topology after steps 1–3, set its quantity. If the name is missing, **do nothing to the topology** (do not invent a process with set-defaults). The scale row still appears in `pending.scales` / `diff.scale` as today. Push would 404 on `UpdateProcessQuantity` for that name.

`diff.process` for pending mode is `diffProcessSnapshots(baseline, effective, scaleNames)` — union of baseline names and this effective map. Untouched processes (e.g. `web` when only `worker` is set) are identical on both sides and omitted. `FoldedPending.Processes` is **not** the pending-side input to this union.

### `diff.process`

Each process name in the union of **from** snapshots and **to** snapshots (pending: baseline vs effective topology; releases: from vs to snapshots; env: both latest snapshots):

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
- `add` / `remove` always emit process ops (quantity lives on the snapshot objects). Keep **both** `## Scale` and `## Process` when an add/remove also has a scale row (env/release snapshot union). Do not drop the process add/remove to satisfy “no double-count.”
- Env snapshot diffs already encode from-only names as `diff.scale` `To:0` (quantity 0 still means “defined, not deployed” in DOMAIN). That line **stays**. A process `remove` for the same name is additive dual encoding — document in tests, do not suppress `To:0`.

### Health and extensions equality

- Health: `nil` equals `nil`; `nil` equals `{type: none}` / empty type; otherwise field equality.
- Extensions: `nil` and empty map are equal; otherwise JSON-object deep equal (unmarshal to `map[string]any`).

### Release and env preview

Same `EffectiveDiff.process` schema. **Do not** run `applyProcessOpsToSnapshot` here (there is no changeset).

- **Releases (`PreviewReleases`):** Build image/config/scale as today (`BuildDiff(foldedFromRelease(to), from)`). Then set `diff.process = diffProcessSnapshots(from.ProcessSnapshot, to.ProcessSnapshot, scaleNames)` where `scaleNames` is every process name in `foldedFromRelease` scales (all names on `to`). Quantity-only changes stay in `diff.scale` via the suppression rule. Command/expose/health/extensions and add/remove emit `diff.process`. Set `Summary` from `formatEffectiveDiff(diff)` on **that combined** `EffectiveDiff` — do **not** call `FormatDiffSummary` (it re-runs `BuildDiff` and would drop process-only command changes, so JSON and CLI `--from-release` would disagree). `foldedFromRelease` may copy snapshots into `pending.processes` as a **complete non-null `to` map** for the API; that map is display of `to` topology, not a sparse overlay, and must not be fed to the pending apply-ops path.
- **Environments (`PreviewEnvironments`):** `BuildSnapshotDiff` already unions config/scale; extend it to call the same `diffProcessSnapshots(from.ProcessSnapshot, to.ProcessSnapshot, scaleNames)` (scaleNames = names that already produce `diff.scale` quantity rows). Keep existing scale quantity lines.

Internal `processReplace` is **not** required if release/env always pass two complete snapshots into `diffProcessSnapshots`. Prefer that over overloading `FoldedPending.Processes` as BuildDiff input.

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

**Pending mode (`mode=pending`)** — `processes` is a **sparse overlay**, not the full topology and not BuildDiff’s input:

- Keys = names **affected by process ops only** (set names, unset names, apply result names, plus baseline names removed by apply).
- Values = snapshot from the effective topology after apply-ops; `null` means unset/removed.
- Scale-only names stay in `scales` only. A scale row for an unknown process does **not** add a `processes` key.

**Release mode** — `pending.processes` is the full non-null `to` process snapshot map (same completeness as today’s `pending.scales` copy of `to` quantities). Do not use `null` tombstones here; names only on `from` appear as `diff.process` `remove` via snapshot union.

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
    Processes         map[string]*domain.ProcessSnapshot `json:"processes,omitempty"` // API overlay only (see pending JSON rules)
    // unexported collection used during fold (pending path only):
    // processApply *domain.ProcessApplyPayload
    // processSets  []domain.ProcessSetPayload
    // processUnsets []string
}

// Pending BuildDiff must not treat Processes as a complete topology.
// Pending process diffs: diffProcessSnapshots(baseline, applyProcessOpsToSnapshot(...), scaleNames).
// Release/env: diffProcessSnapshots(fromSnap, toSnap, scaleNames).

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
  - `TestFoldChangesAcceptsProcessTypes` — set/unset/apply succeed; unknown type errors.
  - `TestFoldChangesInvalidProcessPayloads` — set/unset missing name; apply empty procfile; apply text that fails `ParseProcfile` → `ErrBadRequest`.
  - `TestBuildDiffProcessSetAddsWorker` — baseline web-only; process.set worker command; `diff.process` `op=add` `name=worker` `to.command=run-worker`; **no** `remove` for `web`.
  - `TestBuildDiffProcessFieldChanges` — command, expose, health, extensions on existing `web`.
  - `TestBuildDiffScaleQuantityOnlyNoProcessOp` — scale web=3 → `diff.scale` only (no process op).
  - `TestBuildDiffScaleUnknownProcessNoProcessAdd` — scale `worker=3` with baseline web-only (no process.set) → `diff.scale` for worker, **no** `diff.process` add.
  - `TestBuildDiffProcessSetQuantityOnly` — process.set quantity without scale type → process `fields=["quantity"]`.
  - `TestBuildDiffProcessUnsetAndApply` — unset remove; apply Procfile replace removes names not in the file (`diff.process` `op=remove`).
  - `TestPreviewPendingProcessSet` — StageChanges process.set worker; assert JSON `diff.process` add + `to.command`; summary contains `## Process`; assert `pending.processes["worker"].command == run-worker` (sparse overlay).
  - `TestPreviewPendingProcessUnsetOverlay` — unset a baseline process → `pending.processes[name] == nil` and `diff.process` `op=remove`.
  - `TestPreviewReleasesProcessCommandChange` — two releases, command differs, quantity same → `diff.process` change `fields` includes `command`; **`Summary` contains `## Process`** (not “no effective delta”).
  - `TestBuildSnapshotDiffProcessCommand` — env-style snapshot diff emits process command change (not scale-only).
  - `TestProcessSnapshotEqualityAliases` — health `nil` vs `{type:none}` / empty type omit; extensions `nil` vs empty map omit.
  - Existing fold/diff/env tests still pass.
- **Integration:** none beyond existing in-memory store PreviewPending test.
- **e2e-stub:** create project, `StageChanges` `{"type":"process.set","name":"worker","command":"run-worker"}`, `PreviewPending` succeeds, `HasPending`, and **JSON** `diff.process` has `op=add`, `name=worker`, `to.command=run-worker`. Do not pass L1 on summary substring alone. Empty baseline (no prior deploy) is fine.
- **OpenAPI:** `make openapi-check` after schema update.
- **Persona (CLI UX):** after L1, S1-style: `process set` then `launchpad diff` shows Process; write `docs/superpowers/program/feedback/2026-09-13-preview-process-fold.md` if dogfood runs. If e2e already covers the API, a short CLI invocation against the e2e API is enough.

---

## Docs

- `docs/DOMAIN.md` — (1) pending preview folds process.set/unset/apply and diffs definition fields vs last deploy; (2) qualify ChangesetChange accumulation: config/image/scale are per-key last-write-wins; `process.set`/`unset`/`apply` materialize (and preview) in push buckets — last apply, then all sets in order, then all unsets in order, then scale quantity on **existing** process names only.
- `docs/openapi.yaml` — `Preview` properties for `pending.processes` and `diff.process`.
- `docs/DX-VISION.md` — Active/next points at this spec while in PR; shipped row when merged.
- `docs/superpowers/program/QUEUE.md` — `implementing` / `pr-open` with branch lease.

---

## Open questions

None — scale does not invent processes (matches push 404); pending `processes` overlay vs release full-snapshot maps are specified separately; process diffs for pending use apply-ops topology, not `FoldedPending.Processes` as a union input.

---

## Approval

- [x] Design reviewed and approved (self-approve — ADM)

First `adm-spec-review` (`pass=false`): blocker `scale-invents-process`. Fixed before re-review.

Second `adm-spec-review` (`pass=true`, 0 blockers, 7 warnings). Warnings pinned as implementer rules:

1. **Release summary** — `PreviewReleases.Summary` = `formatEffectiveDiff` on the combined diff (includes `diff.process`). Never `FormatDiffSummary` → `BuildDiff` for that path.
2. **Add/remove dual sections** — keep both `## Scale` and `## Process` when both rows exist; do not drop process add/remove.
3. **Overlay asserts** — unit-test `pending.processes` keys/nulls; `redactFoldedPending` copies `Processes`.
4. **DOMAIN wording** — same buckets, **diffed vs last-deploy snapshot**, not push-materialization equality.
5. **Env scale To:0** — keep existing `diff.scale` To:0 for from-only names; process `remove` is additive dual encoding.
6. **Overlay untested** — covered by pin 3 tests.
7. **Equality omit** — `TestProcessSnapshotEqualityAliases` for health/extensions aliases.
