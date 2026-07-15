# Identity Principals Implementation Plan

> **Status: In Progress** — branch `feat/identity-principals`

**Goal:** Phase 1 principals, token linkage, release attribution, audit events.

**Spec:** `docs/superpowers/specs/2026-07-14-identity-principals-design.md`

**Branch:** `feat/identity-principals` → PR to `main`

---

## Task 1: Spec + DOMAIN

- [ ] Spec, plan, DOMAIN identity section, DX-VISION Track D
- [ ] Commit: `docs: add identity principals phase 1 spec and domain notes`

## Task 2: Domain + migration

- [ ] `internal/domain` principal/audit types; Release created_by fields; APIToken.PrincipalID
- [ ] `004_identity_principals` sqlite + postgres; register in migrate.go
- [ ] Commit: `feat(domain,store): principals, membership, audit schema`

## Task 3: Store + auth

- [ ] Store principals, members, audit, token principal column
- [ ] Auth Authenticate + context token/principal; CreateToken mints SA
- [ ] Tests
- [ ] Commit: `feat(auth,store): link tokens to service account principals`

## Task 4: Service + API attribution

- [ ] enqueueRelease sets created_by + audit
- [ ] DTO + GET /v1/audit
- [ ] Tests
- [ ] Commit: `feat(service,api): attribute releases and record audit events`

## Task 5: Verify + PR

- [ ] make test / build / vet / e2e-stub
- [ ] PR to main
