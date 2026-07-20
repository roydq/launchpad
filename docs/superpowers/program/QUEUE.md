# Autonomous program queue

> **Policy:** Pick the top `ready` item that does not cross a deferred boundary without a design spike.
> **Source of truth for product intent:** `docs/DX-VISION.md` — keep this queue aligned when starting or shipping work.
> **Protocol:** `docs/AUTONOMOUS-MODE.md`

Statuses: `ready` → `designing` → `implementing` → `pr-open` → `shipped` | `blocked` | `deferred`

## Active queue

| Pri | ID | Item | Track | Status | Spec / notes | Branch / PR |
|-----|-----|------|-------|--------|--------------|-------------|
| 1 | env-clone | Environment clone | D | implementing | Spec: `docs/superpowers/specs/2026-07-19-env-clone-design.md` | `feat/env-clone` |
| 2 | worker-stress | Worker lease/supersede stress tests | B | shipped | Spec: `docs/superpowers/specs/2026-07-19-worker-stress-design.md` | PR #36 |
| 3 | target-conformance | Target conformance suite (stub/k8s/…) | B | shipped | Spec: `docs/superpowers/specs/2026-07-19-target-conformance-design.md` | PR #35 |
| 4 | postgres-ci | Postgres matrix in CI | B | shipped | Spec: `docs/superpowers/specs/2026-07-19-postgres-ci-design.md` | PR #34 · fixed pgx driver |
| 5 | shell-prompt | Shell prompt awareness (project@env) | A | shipped | Spec: `docs/superpowers/specs/2026-07-19-shell-prompt-design.md` | PR #33 |
| 6 | recipes-templates | Recipes / `launchpad new` templates | A | shipped | Spec: `docs/superpowers/specs/2026-07-19-recipes-templates-design.md` | PR #32 |
| 7 | oidc-design | OIDC (Azure AD / Google / generic) design | D | deferred | After principals phase 1 dogfood; design before code | — |
| 8 | mcp-server | Launchpad MCP server | A/C | deferred | After core DX loop solid | — |
| 9 | multi-service | Multi-service + ReleaseSet | domain-3 | deferred | Do not half-build; full spec required | — |
| 10 | bindings | Config bindings `${{ refs }}` | domain-4 | deferred | Do not half-build; full spec required | — |
| 11 | launchpad-yaml | `launchpad.yaml` project manifest | domain-6 | deferred | Domain roadmap phase 6 | — |

## Recently shipped (reference)

Keep short; full history lives in DX-VISION and merged specs.

| ID | Item | Spec |
|----|------|------|
| unstage-last | Unstage last mutation | `docs/superpowers/specs/2026-07-19-unstage-last-design.md` · PR #31 |
| diff-env-env | Diff env↔env (release archaeology) | `docs/superpowers/specs/2026-07-19-diff-env-env-design.md` · PR #30 |
| secrets-s2 | Secrets S2: AES-GCM at rest | `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md` · PR #29 |
| secrets-s1 | Secrets S1: typing + redaction + CLI `--secret` | `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md` · PR #28 |
| secrets-design | Secrets-typed config (**design only**) | `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md` · PR #26 · human-accepted model |
| server-diff-preview | Server-side pending/diff preview | `docs/superpowers/specs/2026-07-15-server-diff-preview-design.md` |
| examples-60s | examples/ + 60s path CI | PR #24 |
| openapi-contract | OpenAPI + CI drift check | PR #23 |
| failure-e2e | Failure-path e2e | PR #22 |
| prod-readiness | Prod-readiness dogfood | `docs/superpowers/specs/2026-07-14-prod-readiness-design.md` |
| identity-principals | Principals phase 1 | `docs/superpowers/specs/2026-07-14-identity-principals-design.md` |
| promote | Promote (Wave 3) | `docs/superpowers/specs/2026-07-13-promote-design.md` |

## How ADM uses this file

1. On session start, read this queue + DX-VISION **Active / next**.
2. Select the highest-priority `ready` item within the user budget (or the user-named item).
3. Move status → `designing` / `implementing` / `pr-open` as you go; commit queue updates on the feature branch or a docs commit.
4. On ship (merge), set `shipped` and update DX-VISION.
5. Promote rows from `IDEAS.md` only when: human asks, pre-authorized class (e.g. “any P0 from persona”), or the idea unblocks current dogfood **and** fits MVP.

Do not implement `deferred` or `blocked` items without an explicit human override and a spec.
