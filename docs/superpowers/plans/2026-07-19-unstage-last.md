# Unstage last mutation Implementation Plan

> **Status: Completed** — branch `feat/unstage-last`

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md`. Use `/launchpad-dev` for verification. Commit after each task with the message specified below.

**Goal:** Remove the most recently staged changeset change without full reset.

**Architecture:** Store delete-last by `created_at DESC, id DESC`; service returns deleted change; `DELETE …/changeset/changes/last`; CLI `unstage`.

**Tech Stack:** Go, chi, cobra

**Spec:** `docs/superpowers/specs/2026-07-19-unstage-last-design.md`

**Branch:** `feat/unstage-last`

---

## Task 1: Store + service

**Files:**
- Modify: `internal/store/changesets.go`
- Modify: `internal/store/store_test.go` (or focused test)
- Modify: `internal/service/changeset_service.go`
- Modify: `internal/service/changeset_service_test.go`

- [x] `DeleteLastChangesetChange(ctx, tx, changesetID)` returns deleted row or ErrNotFound
- [x] `UnstageLastChange(ctx, projectName)` service method
- [x] Tests
- [x] Verify: `mise exec -- go test ./internal/store/... ./internal/service/...`
- [x] Commit: `feat(store,service): unstage last changeset change`

## Task 2: API + client + OpenAPI

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `docs/openapi.yaml`
- Modify: `pkg/apiclient/client.go`

- [x] Route `DELETE /projects/{project}/changeset/changes/last`
- [x] Client method
- [x] OpenAPI
- [x] Verify: `mise exec -- go test ./internal/api/...` + `make openapi-check`
- [x] Commit: `feat(api): DELETE changeset/changes/last`

## Task 3: CLI + docs

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `docs/DOMAIN.md`, `docs/DX-VISION.md`, `docs/superpowers/program/QUEUE.md`

- [x] `launchpad unstage` command with friendly summary
- [x] Docs sync
- [x] Verify: `mise exec -- make test && make build && go vet ./...`
- [x] Commit: `feat(cli): launchpad unstage`
- [x] Commit: `docs: unstage last mutation`

---

## Final verification

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
mise exec -- make openapi-check
```

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status Completed
- [ ] Spec linked in PR
