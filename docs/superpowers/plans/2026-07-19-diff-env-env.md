# Diff env↔env Implementation Plan

> **Status: In Progress** — branch `feat/diff-env-env`

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md`. Use `/launchpad-domain` for entity changes, `/launchpad-dev` for verification. Commit after each task with the message specified below.

**Goal:** Compare last-deployed release snapshots between two environments via preview API and `launchpad diff --from-env/--to-env`.

**Architecture:** Extend server-side preview with mode `environments`. Full snapshot diff (union of config keys and processes) with existing secret redaction. CLI flags call the same API.

**Tech Stack:** Go, chi, cobra, SQLite/Postgres

**Spec:** `docs/superpowers/specs/2026-07-19-diff-env-env-design.md`

**Branch:** `feat/diff-env-env`

---

## Task 1: Service — snapshot diff + PreviewEnvironments

**Files:**
- Modify: `internal/service/preview.go`
- Modify: `internal/service/preview_test.go`

- [ ] Add `BuildSnapshotDiff(from, to *domain.Release) EffectiveDiff` (full key/process union; redaction)
- [ ] Add `FormatSnapshotDiffSummary(diff EffectiveDiff) string` (empty → `No differences\n`)
- [ ] Add `PreviewEnvironments(ctx, project, fromEnv, toEnv)` on `ChangesetService`
- [ ] Extend `PreviewResult` with `FromEnvironment` / `ToEnvironment` JSON fields
- [ ] Tests: different releases, secrets, same env error, empty env
- [ ] Verify: `mise exec -- go test ./internal/service/...`
- [ ] Commit: `feat(service): env↔env snapshot preview diff`

## Task 2: API + OpenAPI + apiclient

**Files:**
- Modify: `internal/api/handlers.go`
- Modify: `docs/openapi.yaml`
- Modify: `pkg/apiclient/client.go`

- [ ] Dispatch `from_env`/`to_env` in `preview` handler; mutual exclusion with release pair
- [ ] OpenAPI query params + Preview schema fields
- [ ] `Client.PreviewEnvironments(ctx, project, fromEnv, toEnv)`
- [ ] Verify: `mise exec -- go test ./internal/api/... ./pkg/apiclient/...` and `make openapi-check`
- [ ] Commit: `feat(api): preview from_env/to_env query params`

## Task 3: CLI + docs

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `docs/DOMAIN.md` (CLI table)
- Modify: `docs/DX-VISION.md`, `docs/superpowers/program/QUEUE.md`
- Modify: plan status

- [ ] Flags `--from-env` / `--to-env` on `diff`; mutual exclusion with release flags
- [ ] Print `# env A (vN) → B (vM)` header + summary
- [ ] Docs sync; mark queue implementing → pr-open on PR
- [ ] Verify: `mise exec -- make test && make build && go vet ./...`
- [ ] Commit: `feat(cli): diff --from-env/--to-env`
- [ ] Commit: `docs: env↔env diff shipped notes`

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
- [ ] Plan status updated to Completed
- [ ] Spec linked in PR description
- [ ] No `*.db`, `.env`, or `bin/` committed
