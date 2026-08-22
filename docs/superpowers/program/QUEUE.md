# Autonomous program queue

> **Policy:** Pick the top `ready` item that does not cross a deferred boundary without a design spike.
> **Source of truth for product intent:** `docs/DX-VISION.md` — keep this queue aligned when starting or shipping work.
> **Protocol:** `docs/AUTONOMOUS-MODE.md`

Statuses: `ready` → `designing` → `implementing` → `pr-open` → `shipped` | `blocked` | `deferred`

**Disposition (2026-08-22):** Remaining July deferred rows were product-parked or promoted. See `docs/superpowers/program/feedback/2026-08-22-queue-disposition.md`. Do not start parked rows without a new override.

## Active queue

| Pri | ID | Item | Track | Status | Spec / notes | Branch / PR |
|-----|-----|------|-------|--------|--------------|-------------|
| 1 | launchpad-yaml | `launchpad.yaml` v1 (current model) | A / domain-6 | designing | Spec: `docs/superpowers/specs/2026-08-22-launchpad-yaml-design.md`. DoD: spec + apply/export + e2e-stub; secrets never in the file; not GitOps. | `feat/launchpad-yaml` |
| 2 | mcp-server | Launchpad MCP server | A/C | ready | **After yaml spec exists** (same integrations program). Thin tools over existing OpenAPI/CLI; token auth; no new domain entities. Prefer an `apply` manifest tool once yaml v1 is specified. DoD: spec with tool list + auth story; no DOMAIN fork. | — |

## Parked (not ADM-ready)

| Pri | ID | Item | Track | Status | Spec / notes | Branch / PR |
|-----|-----|------|-------|--------|--------------|-------------|
| — | oidc-design | OIDC login | D | deferred | **Parked.** Tokens + service-account principals already cover CLI/CI/agents. **Start when:** a second human user or a hosted control-plane spike needs interactive login. **IdP:** generic OIDC first (authorization code + CLI device flow); Azure AD / Google as issuer presets later — not a vendor-first design. | — |
| — | multi-service | Multi-service + ReleaseSet | domain-3 | deferred | **Parked.** Processes already cover web+worker on one image. **Start when:** a project needs a second independently versioned artifact (not just another process). Full spec required; do not half-build. Atomic rollback depth still an open DOMAIN question. | — |
| — | bindings | Config bindings `${{ refs }}` | domain-4 | deferred | **Parked — not independent.** Design as slice 2 of multi-service (composition via refs). Do not spec or ship bindings for a single primary service. | — |

**Runtime depth program (shipped 2026-07-20):** process-commands → deploy-health → release-config-materialization → target-extensions. Design: `docs/superpowers/specs/2026-07-20-runtime-target-depth-design.md`.

## Recently shipped (reference)

| ID | Item | Spec / PR |
|----|------|-----------|
| completions-man | Shell completion | PR #51 (ADM 2026-07-20) |
| audit-config-keys | Audit config.set key+sensitivity | PR #50 |
| clone-secret-placeholder | Secret placeholders on clone | PR #49 |
| target-extensions | Extensions + capabilities | PR #48 · runtime depth slice 4 |
| release-config-materialization | Immutable config Secrets | PR #47 · slice 1 |
| deploy-health | Portable health + probes | PR #46 · slice 3 |
| process-commands | Process set/unset/Procfile | PR #45 · slice 2 |
| e2e-env-clone / e2e-recipes-new | e2e coverage | PR #44 |
| status-unstage-hint | Status help unstage | PR #43 |
| env-clone | Environment clone | PR #37 |
| recipes-templates | `launchpad new` | PR #32 |

## How ADM uses this file

Canonical protocol: [`docs/AUTONOMOUS-MODE.md`](../../AUTONOMOUS-MODE.md) (named modes, DoD, worktrees, verification).

1. On session start, read this queue + DX-VISION **Active / next** (+ optional `scripts/adm-status`).
2. Select the highest-priority `ready` item within the **mode** budget (or the user-named item).
3. Before code: ensure **Definition of Done / acceptance** exists (spec success criteria, plan Final verification, or notes cell).
4. Move status → `designing` / `implementing` / `pr-open` as you go; set **Branch / PR** as the implementer **lease**.
5. **Update this file on every feature merge** into the integration branch (not only at session end).
6. On ship (merge to main or integrated + closed), set `shipped`, short Recently shipped row, and DX-VISION if product-facing.
7. Promote rows from `IDEAS.md` only when: human asks, pre-authorized class (e.g. “any P0 from persona”), or the idea unblocks current dogfood **and** fits MVP.

### Definition of Done (minimum for `ready` → `shipped`)

- Spec self-review or human-approved design linked  
- L0: `mise exec -- make test && make build && go vet ./...`  
- Triggered ladder levels (e2e-stub required for service/jobs/target/deploy CLI)  
- Docs sync for what changed  
- PR linked; no half-shipped surface  

### Deferred / blocked rows

Include a one-line **Decision needed:** in Spec / notes so humans can unstick without re-reading all of DOMAIN.

Do not implement `deferred` or `blocked` items without an explicit human override and a spec.

### Row template (new work)

```markdown
| N | my-feature | Short title | A | ready | DoD: <CLI/API/test bullets>. Spec: `docs/superpowers/specs/…` | — |
```
