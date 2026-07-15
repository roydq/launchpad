# Identity Principals Implementation Plan

> **Status: Completed** — branch `feat/identity-principals`

**Goal:** Phase 1 principals, token linkage, release attribution, audit events.

**Spec:** `docs/superpowers/specs/2026-07-14-identity-principals-design.md`

**Branch:** `feat/identity-principals` → PR to `main`

---

## Task 1: Spec + DOMAIN

- [x] Spec, plan, DOMAIN identity section, DX-VISION Track D
- [x] Commit: `docs: add identity principals phase 1 spec and domain notes`

## Task 2: Domain + migration

- [x] `internal/domain` principal/audit types; Release created_by fields; APIToken.PrincipalID
- [x] `004_identity_principals` sqlite + postgres; register in migrate.go
- [x] Commit: `feat(domain,store): principals, membership, audit schema`

## Task 3: Store + auth

- [x] Store principals, members, audit, token principal column
- [x] Auth Authenticate + context token/principal; CreateToken mints SA
- [x] Tests
- [x] Commit: `feat(auth,store): link tokens to service account principals`

## Task 4: Service + API attribution

- [x] enqueueRelease sets created_by + audit
- [x] DTO + GET /v1/audit
- [x] Tests
- [x] Commit: `feat(service,api): attribute releases and record audit events`

## Task 5: Verify + PR

- [x] make test / build / vet / e2e-stub
- [x] PR to main
