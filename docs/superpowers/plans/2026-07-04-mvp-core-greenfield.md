# MVP Core Greenfield Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild Launchpad on the Project/Environment/Service domain model from scratch, removing spike code.

**Architecture:** Layer-by-layer rewrite on `feat/mvp-core-greenfield` — domain types, single SQL migration, store repos, services, updated target interface, deploy worker, REST API, CLI. MVP hardcodes `dev` environment and single primary service.

**Tech Stack:** Go, chi, cobra, SQLite/Postgres (modernc.org/sqlite), client-go

**Spec:** `docs/superpowers/specs/2026-07-04-mvp-core-greenfield-design.md`

---

## Task 1: Domain types

**Files:**
- Replace: `internal/domain/*.go`

- [ ] Add `workspace.go`, `project.go`, `environment.go`, `service.go`, `process.go`
- [ ] Update `release.go`, `job.go`, `changeset.go`, `status.go`, `deployment_fsm.go`
- [ ] Delete `app.go`
- [ ] Commit: `feat(domain): add project/environment/service model types`

## Task 2: Schema and store

**Files:**
- Replace: `internal/store/migrations/001_initial.*.sql`
- Delete: `internal/store/migrations/002_*`, `migrations/`
- Replace: store repository files

- [ ] Write `001_initial` with workspaces, projects, environments, services, processes, config_vars, releases, deployments, changesets, jobs, tokens
- [ ] Implement repos: projects, environments, services, processes, config, releases, deployments, changesets, jobs, tokens
- [ ] Update `store_test.go`
- [ ] Commit: `feat(store): add schema and repositories for MVP model`

## Task 3: Services

**Files:**
- Replace: `internal/service/*.go`

- [ ] `project_service.go` — create/list/get with bootstrap
- [ ] `config_service.go` — service-scoped config in dev
- [ ] `release_service.go` — create release + deployment + job
- [ ] `changeset_service.go` — stage/push/discard
- [ ] Delete `app_service.go`, `scale_service.go`, `release_ops.go` (inline into release_service)
- [ ] Commit: `feat(service): add project, config, release, and changeset services`

## Task 4: Target and worker

**Files:**
- Update: `internal/target/target.go`, `internal/target/kubernetes/*`, `internal/target/stub/*`
- Update: `internal/jobs/worker.go`, `worker_test.go`

- [ ] Update `DeployRequest` shape
- [ ] Update K8s manifest naming to `{project}-{service}-{process}`
- [ ] Worker: deploy job only (remove scale/rollback handlers)
- [ ] Commit: `feat(target): update deploy interface for service+environment`

## Task 5: API and auth

**Files:**
- Replace: `internal/api/handlers.go`
- Update: `cmd/api/main.go`, `internal/auth/auth.go`

- [ ] Project/config/changeset/release/process/job/token routes
- [ ] Wire services in main
- [ ] Commit: `feat(api): REST handlers for MVP endpoints`

## Task 6: CLI and client

**Files:**
- Replace: `internal/cli/root.go`, `pkg/apiclient/client.go`
- Update: `cmd/launchpad/main.go`

- [ ] Commands: projects create, use (config file), config, changeset, deploy, ps, releases
- [ ] Commit: `feat(cli): client and commands for MVP workflow`

## Task 7: Cleanup

- [ ] Delete unused spike files and root `migrations/`
- [ ] Update `README.md`
- [ ] Run `go test ./...`
- [ ] Commit: `chore: remove spike app model and unused code`