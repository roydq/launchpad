# Prod-readiness Implementation Plan

> **Status: Completed** — branch `feat/prod-readiness-dogfood`

**Goal:** Dogfood-ready single-service path with e2e promote confidence and safer CLI.

**Spec:** `docs/superpowers/specs/2026-07-14-prod-readiness-design.md`

**Branch:** `feat/prod-readiness-dogfood` (PR to `main`)

---

## Task 1: Roadmap docs

- [x] Update `docs/DX-VISION.md` with program tracks + prod-ready sequencing
- [x] Commit: `docs: add prod-readiness program and four-track roadmap`

## Task 2: e2e multi-env promote

**Files:** `test/e2e/promote_test.go`, helpers if needed

- [x] Test distinct staging/production config, promote, assert re-resolution
- [x] Verify: `make e2e-stub`
- [x] Commit: `test(e2e): multi-env promote with target config re-resolution`

## Task 3: CLI dogfood UX

**Files:** `internal/cli/root.go`, new helpers + tests

- [x] Format and print APIError hints
- [x] Sensitive env confirm (`production`) for deploy/promote/rollback
- [x] Verify: `mise exec -- make test && make build`
- [x] Commit: `feat(cli): print recovery hints and require --yes for production`

## Task 4: Finish

- [x] README / DX-VISION active table
- [x] Full verify + PR

---

## Follow-on (not this PR)

1. Server-side pending/diff preview  
2. OpenAPI + contract tests  
3. Failure-path e2e (409 deploy in progress)  
4. Postgres CI matrix  
5. Secrets design → env clone  
6. examples/ + docs site skeleton  
