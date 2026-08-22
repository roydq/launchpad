# launchpad.yaml v1 Implementation Plan

> **Status: Not Started** — branch `feat/launchpad-yaml`

> **For agentic workers:** Read `docs/FEATURE-DEVELOPMENT.md` and `docs/superpowers/specs/2026-08-22-launchpad-yaml-design.md`. Use `/launchpad-dev` for verification (`mise exec --`). Commit after each task with the message specified below. Worktree: `.worktrees/feat-launchpad-yaml`. Do not edit other agents' branches or `main`.

**Goal:** Ship `launchpad export` / `launchpad apply` and REST manifest export/apply for the shipped single-service model.

**Architecture:** `domain.Manifest` + validation; `ManifestService` export/apply using existing project/config/changeset/release services; JSON API; CLI YAML; no new tables; apply never deploys.

**Tech Stack:** Go, chi, cobra, gopkg.in/yaml.v3 (CLI), SQLite/Postgres (existing)

**Spec:** `docs/superpowers/specs/2026-08-22-launchpad-yaml-design.md`

**Branch:** `feat/launchpad-yaml`

**DoD:** spec success criteria 1–6; L0; L1 `make e2e-stub`; L2 `make openapi-check`; docs sync; QUEUE `pr-open`.

---

## Task 1: Domain types and validation

**Files:**
- Create: `internal/domain/manifest.go`, `internal/domain/manifest_test.go`

- [ ] Add `Manifest`, `ManifestProcess`, `ManifestEnvironment`, `ManifestConfig` types (json + yaml tags)
- [ ] `ValidateManifest(*Manifest) error` — version, project name, unknown top-level via explicit allowed-key helper on a generic map **or** struct-only plus `ValidateManifestMap` for CLI unknown-key check; reject `services`/`bindings`/`primary_service` mismatch; secret key also in value maps; DNS-label names; require `environments` non-empty for apply (export may still produce them)
- [ ] `ApplyProcessDefaults(name, proc) ManifestProcess` — quantity 1, expose http for `web` else none, empty command
- [ ] Tests: good fixture; version 2; unknown key; secret value; services key; empty project
- [ ] Verify: `mise exec -- go test ./internal/domain/...`
- [ ] Commit: `feat(domain): add launchpad.yaml v1 manifest types`

## Task 2: Manifest service (export + apply)

**Files:**
- Create: `internal/service/manifest_service.go`, `internal/service/manifest_service_test.go`
- Modify: (none unless a tiny helper is required on ProjectService — prefer using existing methods)

- [ ] `ManifestService` with store + Project/Config/Changeset/Release services
- [ ] `Export(ctx, project, envFilter) (*domain.Manifest, error)`
- [ ] `Apply(ctx, projectName, selectedEnv, doc) (*ApplyReport, error)` per spec (create project/env, warnings, stage diffs, no prune, no push)
- [ ] Tests with in-memory store + secrets box: export redacts secrets to `secret_keys`; apply new project; stages process/config/image diffs; no-op does not 400; extra process warning (no prune); secret value rejected; creates **selected** missing env only; create without `dev` rejected; pin conflict
- [ ] Verify: `mise exec -- go test ./internal/service/... ./internal/domain/...`
- [ ] Commit: `feat(service): export and apply launchpad.yaml manifests`

## Task 3: API + OpenAPI

**Files:**
- Modify: `internal/api/handlers.go`, `cmd/api/main.go`, `docs/openapi.yaml`
- Create: `internal/api/manifest_test.go` only if a focused httptest is cheap; otherwise rely on service + contract tests
- Modify: `internal/api/problem/hints.go` for new codes (`manifest_version`, `manifest_invalid`, `manifest_secret_value`, `manifest_deferred`, `manifest_env`, `manifest_project_mismatch`)

- [ ] Wire `ManifestService` into `Server`
- [ ] `GET /v1/projects/{project}/manifest`, `POST /v1/projects/{project}/manifest/apply`
- [ ] OpenAPI paths + schemas; `make openapi-check` green
- [ ] Verify: `mise exec -- go test ./internal/api/... ./internal/service/...` and `mise exec -- make openapi-check`
- [ ] Commit: `feat(api): project manifest export and apply endpoints`

## Task 4: CLI and apiclient

**Files:**
- Modify: `pkg/apiclient/client.go`, `internal/cli/root.go`
- Create: `internal/cli/manifest.go`, `internal/cli/manifest_test.go`

- [ ] Client `GetManifest`, `ApplyManifest`
- [ ] `launchpad export` / `launchpad apply` per spec (YAML via `gopkg.in/yaml.v3`; promote to a direct go.mod require)
- [ ] Tests: filename defaults, stdout, refuse overwrite without `--force`, YAML round-trip fixture, unknown-key rejection
- [ ] Verify: `mise exec -- go test ./internal/cli/... ./pkg/apiclient/...` and `mise exec -- make build`
- [ ] Commit: `feat(cli): launchpad export and apply for launchpad.yaml`

## Task 5: e2e-stub

**Files:**
- Create: `test/e2e/manifest_yaml_test.go`

- [ ] Follow spec e2e recipe: deploy source first so export includes `image:`; rewrite `project:` to dest; apply; assert no job from apply; dest `diff` non-empty; dest `deploy --wait` succeeds
- [ ] Separate: `secret_keys` → `needs_value`; yaml has no secret plaintext (no deploy in this case)
- [ ] Verify with full e2e at Task 6 (this task adds the test file; `mise exec -- go test -count=1 ./internal/... ./pkg/...` still green)
- [ ] Commit: `test(e2e): launchpad.yaml export and apply on stub`

## Task 6: Docs + queue

**Files:**
- Modify: `docs/DOMAIN.md`, `docs/DX-VISION.md`, `docs/DESIGN.md`, `README.md`, `docs/superpowers/program/QUEUE.md`, this plan (checkboxes + status)

- [ ] DOMAIN CLI table + phase 6 v1 + open question 5
- [ ] README solo workflow one-liner
- [ ] QUEUE → `pr-open` after PR exists (set `implementing` in this commit if PR not yet opened; orchestrator sets `pr-open` with link)
- [ ] Plan status **In Progress** / checkboxes
- [ ] Verify: `mise exec -- make test && make build && go vet ./...`
- [ ] Commit: `docs: launchpad.yaml v1 export/apply`

---

## Task 0 (orchestrator, before Task 1)

- [ ] Spec + this plan committed
- [ ] Commit: `docs: add launchpad.yaml v1 spec and implementation plan`

---

## Final verification

```bash
mise exec -- make test
mise exec -- make build
mise exec -- go vet ./...
mise exec -- make openapi-check
make e2e-stub
```

Persona: S1-equivalent yaml path (export/apply) after e2e; write `docs/superpowers/program/feedback/2026-08-22-launchpad-yaml.md`. Scout → `IDEAS.md` only.

## PR checklist

- [ ] All tasks checked off
- [ ] Plan status updated to Completed (ready for PR)
- [ ] Spec linked in PR description
- [ ] No `*.db`, `.env`, or `bin/` committed
- [ ] QUEUE Branch lease `feat/launchpad-yaml`; no merge to main
