# Release Invariants Implementation Plan

> **Status: In Progress** — branch `feat/release-invariants`, started 2026-07-09

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

- [ ] Expand `ProcessSnapshot` with `Command string`, `Quantity int`, `Expose string` and json tags
- [ ] Keep backward-compatible unmarshaling in mind (missing fields → zero values; worker/service treat empty command as image entrypoint)
- [ ] Verify: `mise exec -- go test ./internal/domain/...`
- [ ] Commit: `feat(domain): expand process snapshot for deployable desired state`

---

## Task 2: Store helpers (concurrency / supersede)

**Files:**
- Modify: `internal/store/releases.go` (or deployments section)
- Modify: `internal/store/store_test.go` as needed

- [ ] Add `SupersedeRunningDeployment(ctx, tx, serviceID, environmentID, exceptDeploymentID)` (or equivalent) that sets `running → superseded` for other rows
- [ ] Optionally map unique-index violations on deployment insert to `launchpad.ErrConflict` if not already
- [ ] Keep `HasActiveDeployment` for pre-checks **or** rely on insert + unique index inside TX (prefer check inside same TX as create, or only unique index)
- [ ] Tests: two active deploys rejected; supersede updates previous running
- [ ] Verify: `mise exec -- go test ./internal/store/...`
- [ ] Commit: `feat(store): supersede running deployments and active-deploy conflict handling`

---

## Task 3: Services — snapshot build + atomic push

**Files:**
- Modify: `internal/service/release_service.go`
- Modify: `internal/service/changeset_service.go`
- Modify: `internal/service/changeset_service_test.go`
- Create: `internal/service/release_service_test.go` if needed

- [ ] `buildProcessSnapshot` includes command, quantity, expose from `ListProcesses`
- [ ] `enqueueRelease` remains the single path for create release + deployment + job + project status; call `HasActiveDeployment` **inside** the TX (or depend on unique index + conflict wrap)
- [ ] `PushChangeset`: **one** `Transact` that:
  1. Applies config/scale
  2. Resolves artifact (from changes or latest succeeded release)
  3. Builds process snapshot + config map for release
  4. Creates release, deployment, job
  5. Commits changeset  
  On any failure, no live mutations and changeset stays open
- [ ] Remove `updateProjectStatusTx` pass-through if still present — call store directly
- [ ] Use `errors.Is` for not-found checks
- [ ] Tests: atomic push rollback; process snapshot fields; concurrent active deploy → conflict
- [ ] Verify: `mise exec -- go test ./internal/service/...`
- [ ] Commit: `feat(service): atomic push and full process snapshots`

---

## Task 4: Worker — deploy from release only

**Files:**
- Modify: `internal/jobs/worker.go`
- Modify: `internal/jobs/worker_test.go`

- [ ] `loadDeployContext` / handleDeploy: set `Config` from `release.ConfigResolved`, `Processes` reconstructed from `release.ProcessSnapshot` (+ service ID / names as needed)
- [ ] Do **not** call `ListConfigVars` or `ListProcesses` for deploy desired state
- [ ] On `pending → deploying`, supersede previous running deployment for service×env
- [ ] Clean fail-path transitions with `errors.Is`; avoid discarded errors where possible
- [ ] Test: mutate live config after release create; deploy still uses snapshot
- [ ] Test: supersede previous running
- [ ] Verify: `mise exec -- go test ./internal/jobs/...`
- [ ] Commit: `feat(worker): deploy from release snapshot only`

---

## Task 5: Target (minimal)

**Files:**
- Modify: `internal/target/target.go` (doc comments on DeployRequest)
- Modify: `internal/target/kubernetes/manifest.go` / apply only if command from process is required and currently ignored
- Modify: tests if behavior changes

- [ ] Document that `Processes` and `Config` must be derived from the release by the caller
- [ ] Ensure k8s/stub use `process.Command` when building containers if not already
- [ ] Verify: `mise exec -- go test ./internal/target/...`
- [ ] Commit: `feat(target): document snapshot-derived deploy inputs`

---

## Task 6: API DTOs and job service boundary

**Files:**
- Modify: `internal/api/handlers.go`
- Create: `internal/api/responses.go` (or similar) for DTOs
- Optionally: thin job read via store remains acceptable if responses are DTO-shaped

- [ ] Add snake_case response types for Project (already map), Release, Process, Job, Changeset (+ changes)
- [ ] Stop encoding bare domain structs for GET list/get endpoints
- [ ] Keep `releaseJobResponse` consistent with DTOs
- [ ] Prefer `problem` package for auth errors if a small shared helper is easy; else leave auth as-is
- [ ] Avoid leaking raw internal errors on 500 where easy (log detail, generic client message) — optional nicety
- [ ] Verify: `mise exec -- go test ./internal/api/...` (add tests if package has none; otherwise exercise via service + manual JSON in client tests)
- [ ] Commit: `feat(api): snake_case response DTOs for MVP endpoints`

---

## Task 7: CLI and apiclient

**Files:**
- Modify: `pkg/apiclient/client.go`
- Modify: `internal/cli/root.go`

- [ ] Typed structs for Changeset, Release, Process, Job, deploy result
- [ ] Parse problem+json or at least return body snippet on non-2xx where cheap
- [ ] Remove dual `Changes` / `changes` key handling in CLI
- [ ] Verify: `mise exec -- make build` && `mise exec -- go test ./pkg/apiclient/... ./internal/cli/...` (if tests exist)
- [ ] Commit: `feat(cli): typed API client responses`

---

## Task 8: Final verification and plan close-out

- [ ] Full verification (below)
- [ ] Smoke: `scripts/smoke-stub.sh` with API + worker
- [ ] Update plan status to **Completed** (when PR ready)
- [ ] Spec approval checkbox if not already
- [ ] Commit: `chore: verify release-invariants and mark plan complete` (only if residual docs tweaks)

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
