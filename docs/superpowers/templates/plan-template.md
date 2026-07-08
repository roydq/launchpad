# <Feature Name> Implementation Plan

> **Status: Not Started** — branch `feat/<short-name>`

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md`. Use `/launchpad-domain` for entity changes, `/launchpad-dev` for verification. Commit after each task with the message specified below.

**Goal:** <one sentence>

**Architecture:** <2-3 sentences on approach>

**Tech Stack:** Go, chi, cobra, SQLite/Postgres

**Spec:** `docs/superpowers/specs/YYYY-MM-DD-<feature-name>-design.md`

**Branch:** `feat/<short-name>`

---

## Task 1: Domain types

**Files:**
- Create: `internal/domain/<file>.go`
- Modify: `internal/domain/<file>.go`
- Delete: (none)

- [ ] <specific step>
- [ ] Add/update tests in `internal/domain/<file>_test.go`
- [ ] Verify: `mise exec -- go test ./internal/domain/...`
- [ ] Commit: `feat(domain): <message>`

## Task 2: Schema and store

**Files:**
- Create: `internal/store/migrations/00N_<name>.up.sql`, `.down.sql`
- Modify: `internal/store/<repo>.go`

- [ ] Write migration
- [ ] Implement repository methods
- [ ] Update `internal/store/store_test.go`
- [ ] Verify: `mise exec -- go test ./internal/store/...`
- [ ] Commit: `feat(store): <message>`

## Task 3: Services

**Files:**
- Create: `internal/service/<name>_service.go`
- Modify: `internal/service/<name>_service.go`

- [ ] Implement business logic
- [ ] Add service tests
- [ ] Verify: `mise exec -- go test ./internal/service/...`
- [ ] Commit: `feat(service): <message>`

## Task 4: Target and worker

**Files:**
- Modify: `internal/target/...`, `internal/jobs/worker.go`

- [ ] <specific step — or "N/A, skip task" if no deploy changes>
- [ ] Verify: `mise exec -- go test ./internal/target/... ./internal/jobs/...`
- [ ] Commit: `feat(target,worker): <message>`

## Task 5: API

**Files:**
- Modify: `internal/api/handlers.go`, `cmd/api/main.go`

- [ ] Add routes and handlers
- [ ] Add handler tests
- [ ] Verify: `mise exec -- go test ./internal/api/...`
- [ ] Commit: `feat(api): <message>`

## Task 6: CLI and client

**Files:**
- Modify: `internal/cli/root.go`, `pkg/apiclient/client.go`

- [ ] Add commands and client methods
- [ ] Verify: `mise exec -- make build`
- [ ] Commit: `feat(cli): <message>`

## Task 7: Cleanup and docs

- [ ] Update `README.md` if user-facing workflow changed
- [ ] Remove dead code / temporary shims
- [ ] Full verification (see below)
- [ ] Commit: `chore: <message>`
- [ ] Update plan status to **Completed**

---

## Final verification

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
```

If deploy flow changed, also run `scripts/smoke-stub.sh` (API + worker must be running).

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status updated to Completed
- [ ] Spec linked in PR description
- [ ] No `*.db`, `.env`, or `bin/` committed