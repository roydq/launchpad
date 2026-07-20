# Plan: Recipes / `launchpad new` templates

> **Status:** Completed  
> **Branch:** `feat/recipes-templates` (from `adm/queue-2026-07-19`)  
> **Spec:** `docs/superpowers/specs/2026-07-19-recipes-templates-design.md`

## Goal

Ship `launchpad new` / `new list` with `hello-stub` and `web-stub` recipes using existing create + stage APIs.

## Tasks

### Task 1: Recipe registry + arg parse + tests

- [x] Add `internal/cli/recipes.go` with Recipe type, catalog, lookup, `ParseNewArgs`
- [x] Tests in `internal/cli/recipes_test.go`
- [x] Commit: `feat(cli): recipe catalog and new arg parsing`

### Task 2: `launchpad new` command + apply

- [x] Wire `new` / `new list` into `internal/cli/root.go` (or `new.go`)
- [x] Apply: CreateProject, write context (reuse use/project-local helpers), optional StageChanges for image + config
- [x] Commit: `feat(cli): launchpad new applies built-in recipes`

### Task 3: Docs + queue

- [x] README solo workflow one-liner; DX-VISION recipes status; QUEUE → pr-open
- [x] Commit: `docs: recipes / launchpad new`

### Final verification

- [x] `mise exec -- make test && make build && go vet ./...`
