# Autonomous program queue

> **Policy:** Pick the top `ready` item that does not cross a deferred boundary without a design spike.
> **Source of truth for product intent:** `docs/DX-VISION.md` — keep this queue aligned when starting or shipping work.
> **Protocol:** `docs/AUTONOMOUS-MODE.md`

Statuses: `ready` → `designing` → `implementing` → `pr-open` → `shipped` | `blocked` | `deferred`

## Active queue

| Pri | ID | Item | Track | Status | Spec / notes | Branch / PR |
|-----|-----|------|-------|--------|--------------|-------------|
| 1 | oidc-design | OIDC (Azure AD / Google / generic) design | D | deferred | After principals phase 1 dogfood; design before code — **human review required** | — |
| 2 | mcp-server | Launchpad MCP server | A/C | deferred | After core DX loop solid — **human review of product surface** | — |
| 3 | multi-service | Multi-service + ReleaseSet | domain-3 | deferred | Do not half-build; full spec required — **human design** | — |
| 4 | bindings | Config bindings `${{ refs }}` | domain-4 | deferred | Do not half-build; full spec required — **human design** | — |
| 5 | launchpad-yaml | `launchpad.yaml` project manifest | domain-6 | deferred | Domain roadmap phase 6 — **human design** | — |

**ADM stop condition (2026-07-20):** No `ready` / implementable items remain. Queue is only deferred items that require human product design review before code.

**Runtime depth program (shipped this ADM run):** process-commands → deploy-health → release-config-materialization → target-extensions. Design: `docs/superpowers/specs/2026-07-20-runtime-target-depth-design.md`.

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

1. On session start, read this queue + DX-VISION **Active / next**.
2. Select the highest-priority `ready` item within the user budget (or the user-named item).
3. Move status → `designing` / `implementing` / `pr-open` as you go; commit queue updates on the feature branch or a docs commit.
4. On ship (merge), set `shipped` and update DX-VISION.
5. Promote rows from `IDEAS.md` only when: human asks, pre-authorized class (e.g. “any P0 from persona”), or the idea unblocks current dogfood **and** fits MVP.

Do not implement `deferred` or `blocked` items without an explicit human override and a spec.
