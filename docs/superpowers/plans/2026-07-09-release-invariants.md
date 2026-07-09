# Release Invariants Implementation Plan

> **Status: Implementation complete** — branch `feat/release-invariants`, ready for PR

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md` and the spec. Use `/launchpad-domain` for entity questions, `/launchpad-dev` for verification. Commit after each task with the message specified below. Do not implement until the design is approved if still Draft — docs tasks may land first.

**Goal:** Enforce release immutability at deploy time, atomic changeset push, stable snake_case API wire format, and align docs with reality.

**Architecture:** Expand process snapshot; single enqueue path shared by push and immediate release; worker derives target inputs only from the release; API response DTOs at the edge; docs updated first (or with domain commit).

**Tech Stack:** Go, chi, cobra, SQLite/Postgres

**Spec:** `docs/superpowers/specs/2026-07-09-release-invariants-design.md`

**Branch:** `feat/release-invariants`

---

## Task 0: Spec, plan, and domain/design docs

**Files:**
- Create: `docs/superpowers/specs/2026-07-09-release-invariants-design.md`
- Create: `docs/superpowers/plans/2026-07-09-release-invariants.md`
- Modify: `docs/DOMAIN.md`
- Modify: `docs/DESIGN.md`

- [x] Write design spec
- [x] Write this plan
- [x] Update DOMAIN.md (invariants, snapshot shape, push TX, MVP honesty, DeployRequest, Target surface, stale bits)
- [x] Update DESIGN.md (request flow, worker uses snapshot, TX boundaries)
- [x] Update AGENTS.md with phase 1b pointer
- [x] Verify: docs only — no Go changes yet
- [x] Commit: `docs: add release-invariants spec, plan, and domain alignment`

---

## Task 1: Domain types

**Files:**
- Modify: `internal/domain/process.go`
- Modify: `internal/domain/release.go` (comments only if needed)
- Create or modify: `internal/domain/process_test.go` (optional helpers)

- [x] Expand `ProcessSnapshot` with `Command string`, `Quantity int`, `Expose string` and json tags
- [x] Keep backward-compatible unmarshaling in mind (missing fields → zero values; worker/service treat empty command as image entrypoint)
- [x] Verify: `mise exec -- go test ./internal/domain/...`
- [x] Commit: `feat(domain): expand process snapshot for deployable desired state`

---

## Task 2: Store helpers (concurrency / supersede)

**Files:**
- Modify: `internal/store/releases.go` (or deployments section)
- Modify: `internal/store/store_test.go` as needed

- [x] Add `SupersedeRunningDeployments` that sets `running → superseded` for other rows
- [x] Map unique-index violations on deployment insert to `launchpad.ErrConflict`
- [x] `HasActiveDeploymentTx` for in-TX checks
- [x] Tests: two active deploys rejected; supersede updates previous running
- [x] Verify: `mise exec -- go test ./internal/store/...`
- [x] Commit: `feat(store): supersede running deployments and active-deploy conflict handling`

---

## Task 3: Services — snapshot build + atomic push

**Files:**
- Modify: `internal/service/release_service.go`
- Modify: `internal/service/changeset_service.go`
- Modify: `internal/service/changeset_service_test.go`

- [x] `buildProcessSnapshot` includes command, quantity, expose
- [x] `enqueueReleaseTx` shared path; active deploy check inside TX
- [x] `PushChangeset` one Transact for materialize + enqueue + commit
- [x] Remove pass-through project status wrapper
- [x] Use `errors.Is` for not-found checks
- [x] Tests: atomic push rollback; process snapshot fields
- [x] Verify: `mise exec -- go test ./internal/service/...`
- [x] Commit: `feat(service): atomic push and full process snapshots`

---

## Task 4: Worker — deploy from release only

**Files:**
- Modify: `internal/jobs/worker.go`
- Modify: `internal/jobs/worker_test.go`

- [x] Desired state from release snapshot only
- [x] Supersede previous running on `pending → deploying`
- [x] Fail-path uses `errors.Is`
- [x] Tests: snapshot isolation; supersede
- [x] Verify: `mise exec -- go test ./internal/jobs/...`
- [x] Commit: `feat(worker): deploy from release snapshot only`

---

## Task 5: Target (minimal)

**Files:**
- Modify: `internal/target/target.go`
- Modify: `internal/target/kubernetes/manifest.go`

- [x] Document snapshot-derived deploy inputs
- [x] K8s uses `process.Command` when set
- [x] Verify: `mise exec -- go test ./internal/target/...`
- [x] Commit: `feat(target): document snapshot-derived deploy inputs`

---

## Task 6: API DTOs and job service boundary

**Files:**
- Modify: `internal/api/handlers.go`
- Create: `internal/api/responses.go`

- [x] Snake_case response DTOs
- [x] Generic 500 detail (no raw internal leak)
- [x] Commit: `feat(api): snake_case response DTOs for MVP endpoints`

---

## Task 7: CLI and apiclient

**Files:**
- Modify: `pkg/apiclient/client.go`
- Modify: `internal/cli/root.go`

- [x] Typed structs for Changeset, Release, Process, Job, deploy result
- [x] Error body snippet on non-2xx
- [x] Remove dual `Changes` / `changes` key handling
- [x] Verify: `mise exec -- make build`
- [x] Commit: `feat(cli): typed API client responses`

---

## Task 8: Final verification and plan close-out

- [x] Full verification (`make test`, `make build`, `go vet`)
- [ ] Smoke: `scripts/smoke-stub.sh` with API + worker (optional local)
- [x] Plan status updated
- [x] Spec approval checkbox

---

## Final verification

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
```

Deploy flow changed — also run:

```bash
# terminals: run-api, run-worker
scripts/smoke-stub.sh
```

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status updated to Completed
- [ ] Spec linked in PR description
- [ ] No `*.db`, `.env`, or `bin/` committed
- [ ] DOMAIN.md / DESIGN.md match implementation
