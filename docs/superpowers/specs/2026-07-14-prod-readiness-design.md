# Prod-readiness program (dogfood track)

| Field | Value |
|-------|-------|
| **Status** | Approved (self-approve — program plan + first slices) |
| **Date** | 2026-07-14 |
| **Related** | `docs/DX-VISION.md`, `docs/DOMAIN.md` |
| **Scope** | Make single-service Launchpad trustworthy for daily dogfood; expand CI confidence; CLI safety/UX |

---

## Goal

Get Launchpad to a **prod-ready dogfood state** for a solo engineer / small team on **one primary service**, multi-env, stub or k8s:

- Trust promote + layered config under automated e2e
- CLI surfaces recovery and protects production by default
- Living roadmap so surfaces (docs site, TUI, dashboard, hosted) do not derail core

Success for this slice:

1. `make e2e-stub` includes multi-env promote with distinct config re-resolution  
2. CLI prints problem+json `hints` on API failure  
3. Promote/deploy into `production` requires `--yes` (or non-TTY fails without it)  
4. DX-VISION documents four parallel tracks (DX / confidence / surfaces / platform)

---

## Approaches

### A. Program doc + thin vertical slices (recommended)

Capture long-horizon tracks in DX-VISION; ship dogfood slices one PR at a time without multi-service/secrets.

### B. Big-bang “1.0” packaging

Helm + OpenAPI + dashboard + secrets first — **reject** for now; delays dogfood.

**Recommendation:** A.

---

## Scope

### In scope (this PR series / first PR)

- Living roadmap tracks in `docs/DX-VISION.md`
- e2e-stub: promote across envs + config assert
- CLI: multi-line hints from `*apiclient.APIError`
- CLI: `--yes` gate for sensitive target env (`production`) on deploy/promote/rollback

### Out of scope (documented tracks, later)

- Server-side pending/diff preview (next DX slice)
- OpenAPI golden, Postgres CI matrix
- Secrets-typed config, env clone
- Multi-service ReleaseSet
- TUI / web dashboard / docs site MVP
- Hosted multi-tenant control plane
- MCP server

---

## Domain impact

None for first slices. Sensitive-env confirmations are **CLI policy only** (API remains usable by agents with tokens).

---

## Test strategy

- `//go:build e2e` test: create project, staging+production, patch distinct config, deploy staging, promote, assert configs
- Unit: CLI confirm helper table tests (no TTY needed)
- `make e2e-stub`, `make test`, `make build`, `go vet`
