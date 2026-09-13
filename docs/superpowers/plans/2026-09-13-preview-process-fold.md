# Fold process mutations in pending preview — Implementation Plan

> **Status: In Progress** — branch `feat/preview-process-fold`, started 2026-09-13

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md`. Use `/launchpad-dev` for verification. Commit after each task with the message specified below. Verify from the trusted repo root: `mise exec -- go test -C .worktrees/feat-preview-process-fold <pkgs>`. Do not edit other agents’ branches. Push `feat/preview-process-fold` after each accepted task.

**Goal:** Fold `process.set` / `process.unset` / `process.apply` in server-side preview so `GET /preview` and `launchpad diff` show process definition deltas instead of 400.

**Architecture:** Extend `FoldChanges` to collect the same process ops `materializeChanges` already collects. Apply those ops onto the baseline `process_snapshot` (Procfile replace, then sets, then unsets, then scale quantity) using `upsertProcessSet` defaults. Emit additive `pending.processes` and `diff.process`; keep `diff.scale` for scale-type quantity lines. CLI uses `summary`.

**Tech Stack:** Go, chi, cobra, SQLite/Postgres (no schema change)

**Spec:** `docs/superpowers/specs/2026-09-13-preview-process-fold-design.md`

**Branch:** `feat/preview-process-fold`  
**Worktree:** `.worktrees/feat-preview-process-fold`  
**PR base:** `main`

---

## File map

| File | Role |
|------|------|
| `internal/service/preview.go` | Fold, apply ops, process diff, summary, empty checks, redact copy |
| `internal/service/preview_test.go` | Unit + PreviewPending process.set |
| `docs/openapi.yaml` | Preview schema |
| `pkg/apiclient/client.go` | Typed Preview pending/diff process fields |
| `test/e2e/failure_paths_test.go` or new `test/e2e/preview_process_test.go` | e2e-stub process.set → preview |
| `docs/DOMAIN.md` | One preview sentence |
| `docs/DX-VISION.md` | Active/next link |
| `docs/superpowers/program/QUEUE.md` | Status / lease |

Domain, store, target, worker: N/A.

---

## Task 1: Service — fold process types and process diffs

**Files:**
- Modify: `internal/service/preview.go`
- Modify: `internal/service/preview_test.go`

- [ ] Add types and fold collection per spec (`ProcessDiffOp`; `FoldedPending.Processes`; unexported apply/sets/unsets/replace; `IsEmpty` includes process ops; `EffectiveDiff.Process` in `IsEmpty`).
- [ ] `FoldChanges` handles `process.set` / `unset` / `apply` (validate name / Procfile via `domain.ParseProcfile`); unknown types still 400.
- [ ] Apply ops onto baseline with `upsertProcessSet` / Procfile defaults; scale quantity overlay.
- [ ] `BuildDiff` emits `diff.process` with quantity-suppression rule vs `diff.scale`.
- [ ] `BuildSnapshotDiff` and `foldedFromRelease` use the same process field compare (full snapshots; replace semantics for release `to`).
- [ ] `formatEffectiveDiff` appends `## Process` lines; `redactFoldedPending` copies `Processes`.
- [ ] `PreviewPending` populates `pending.processes` for affected names (null = unset) after applying ops to baseline.
- [ ] Tests in `preview_test.go` (fail first if following TDD):
  - `TestFoldChangesAcceptsProcessTypes` — set/unset/apply succeed; garbage type errors.
  - `TestBuildDiffProcessSetAddsWorker` — baseline web-only snapshot; process.set worker command; `op=add`.
  - `TestBuildDiffProcessFieldChanges` — command, expose, health, extensions on existing web.
  - `TestBuildDiffScaleQuantityOnlyNoProcessOp` — scale web=3 → scale row, no process op.
  - `TestBuildDiffProcessSetQuantityOnly` — process.set quantity without scale type → process `fields=["quantity"]`.
  - `TestBuildDiffProcessUnsetAndApply` — unset remove; apply Procfile drops names not in file.
  - `TestPreviewPendingProcessSet` — StageChanges process.set worker; PreviewPending has process add and `## Process` in summary.
- [ ] Verify: `mise exec -- go test -C .worktrees/feat-preview-process-fold ./internal/service/...`
- [ ] Commit: `feat(service): fold process mutations in pending preview`

### Apply-ops sketch (preview.go)

Reuse payload types from `internal/domain`. Do not call store. Pure functions:

```go
func applyProcessOpsToSnapshot(baseline map[string]domain.ProcessSnapshot, folded FoldedPending) map[string]domain.ProcessSnapshot
func mergeProcessSet(cur map[string]domain.ProcessSnapshot, set domain.ProcessSetPayload)
func processSnapshotsEqual(a, b domain.ProcessSnapshot) (fields []string)
```

Health/extensions equality as in the spec. Sort process op names and `fields`.

When `PreviewPending` builds the API `pending.processes` map: include names touched by process ops only (set names, unset names, apply result names, apply-removed baseline names). Scale-only names stay out of that map.

---

## Task 2: API contract + apiclient

**Files:**
- Modify: `docs/openapi.yaml` (`components.schemas.Preview`)
- Modify: `pkg/apiclient/client.go` (`Preview` struct)

- [ ] Document `pending` (`image`, `config`, `config_sensitivity`, `scales`, `processes` additionalProperties nullable process snapshot) and `diff` (`image`, `config`, `scale`, `process` with `op`/`name`/`from`/`to`/`fields`).
- [ ] Extend `apiclient.Preview` so e2e can assert `Diff.Process` and `Pending.Processes`. Keep existing fields. JSON names: `process` (array), `processes` (map).
- [ ] Verify: `mise exec -- make -C .worktrees/feat-preview-process-fold openapi-check` and `mise exec -- go test -C .worktrees/feat-preview-process-fold ./pkg/apiclient/... ./internal/api/...`
- [ ] Commit: `feat(api): document preview process diff contract`

If `make openapi-check` is not valid with `-C`, run from worktree after `mise trust` **or** from repo root:

```bash
mise exec -- go test -C .worktrees/feat-preview-process-fold ./internal/api/...
# openapi-check: (cd .worktrees/feat-preview-process-fold && mise exec -- make openapi-check)
# Prefer: mise exec -- make openapi-check  is Makefile at cwd; use
# make -C .worktrees/feat-preview-process-fold openapi-check
# with PATH from mise exec -- bash -lc 'make -C .worktrees/feat-preview-process-fold openapi-check'
```

Canonical: `mise exec -- bash -lc 'make -C .worktrees/feat-preview-process-fold openapi-check'`

---

## Task 3: e2e-stub

**Files:**
- Create: `test/e2e/preview_process_test.go` (`//go:build e2e`)

- [ ] `TestPreviewPendingProcessSet` using existing helpers (`requireE2E`, `newAuthedClient`, `CreateProject`, `StageChanges`, `PreviewPending`).
- [ ] Stage `{"type":"process.set","name":"worker","command":"run-worker"}`.
- [ ] Assert preview err is nil, `HasPending`, and either a process add named worker or `Summary` contains `worker` and `Process`.
- [ ] Verify with L1 (orchestrator): `mise exec -- make e2e-stub` from repo root **after** worktree changes are the ones under test. If Make always tests the main checkout, run e2e from the worktree directory (`cd .worktrees/feat-preview-process-fold` + `mise trust` once) so the feature binary is built.
- [ ] Commit: `test(e2e): preview process.set does not 400`

---

## Task 4: Docs + queue

**Files:**
- Modify: `docs/DOMAIN.md` (changeset workflow / preview note)
- Modify: `docs/DX-VISION.md` (Active/next → this spec)
- Modify: `docs/superpowers/program/QUEUE.md` (status implementing / later pr-open)
- Modify: this plan checkboxes + status

- [ ] DOMAIN: pending preview folds `process.set`/`unset`/`apply` and diffs definition fields vs last deploy.
- [ ] DX-VISION Active/next links this spec.
- [ ] QUEUE Branch = `feat/preview-process-fold`; status `implementing` until PR, then `pr-open` + PR link.
- [ ] Verify: docs-only; L0 still green.
- [ ] Commit: `docs: preview folds process definition changes`

---

## Task 5: Cleanup, persona, scout

- [ ] L0 from repo root: `mise exec -- bash -lc 'make -C .worktrees/feat-preview-process-fold test && make -C .worktrees/feat-preview-process-fold build && go vet -C .worktrees/feat-preview-process-fold ./...'`
- [ ] L1 e2e-stub as in Task 3
- [ ] Persona: S1-equivalent `process set` + `launchpad diff` if a stub API is up from e2e or local run; write `docs/superpowers/program/feedback/2026-09-13-preview-process-fold.md`. If dogfood cannot run, record `blocked` + reason — do not fake a pass.
- [ ] Scout: append one row to the **Open** table at the end of `docs/superpowers/program/IDEAS.md` (e.g. materialize process ops are bucketed not sequential — only if not already logged).
- [ ] Plan status → Completed (ready for PR) when tasks 1–4 are checked.
- [ ] Commit remaining docs/feedback if any: `docs: preview-process-fold closeout notes`

---

## Final verification

```bash
mise exec -- bash -lc 'make -C .worktrees/feat-preview-process-fold test && make -C .worktrees/feat-preview-process-fold build && go vet -C .worktrees/feat-preview-process-fold ./...'
mise exec -- bash -lc 'make -C .worktrees/feat-preview-process-fold openapi-check'
# L1 from the worktree so the feature binary is under test:
mise exec -- bash -lc 'cd .worktrees/feat-preview-process-fold && make e2e-stub'
```

If `18080` is busy, harness picks another port unless `LAUNCHPAD_E2E_API_ADDR` is set.

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status updated to Completed
- [ ] Spec linked in PR description
- [ ] DoD bullets from the spec in the PR test plan
- [ ] No `*.db`, `.env`, or `bin/` committed
- [ ] QUEUE `pr-open` with PR link
