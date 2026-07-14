# Prod-readiness Implementation Plan

> **Status: In Progress** — branch `feat/prod-readiness-dogfood`

**Goal:** Dogfood-ready single-service path with e2e promote confidence and safer CLI.

**Spec:** `docs/superpowers/specs/2026-07-14-prod-readiness-design.md`

**Branch:** `feat/prod-readiness-dogfood` (PR to `main`)

---

## Task 1: Roadmap docs

- [ ] Update `docs/DX-VISION.md` with program tracks + prod-ready sequencing
- [ ] Commit: `docs: add prod-readiness program and four-track roadmap`

## Task 2: e2e multi-env promote

**Files:** `test/e2e/promote_test.go`, helpers if needed

- [ ] Test distinct staging/production config, promote, assert re-resolution
- [ ] Verify: `make e2e-stub`
- [ ] Commit: `test(e2e): multi-env promote with target config re-resolution`

## Task 3: CLI dogfood UX

**Files:** `internal/cli/root.go`, new helpers + tests

- [ ] Format and print APIError hints
- [ ] Sensitive env confirm (`production`) for deploy/promote/rollback
- [ ] Verify: `mise exec -- make test && make build`
- [ ] Commit: `feat(cli): print recovery hints and require --yes for production`

## Task 4: Finish

- [ ] README / DX-VISION active table
- [ ] Full verify + PR

---

## Follow-on (not this PR)

1. Server-side pending/diff preview  
2. OpenAPI + contract tests  
3. Failure-path e2e (409 deploy in progress)  
4. Postgres CI matrix  
5. Secrets design → env clone  
6. examples/ + docs site skeleton  
