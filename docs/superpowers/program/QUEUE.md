# Autonomous program queue

> **Policy:** Pick the top `ready` item that does not cross a deferred boundary without a design spike.
> **Source of truth for product intent:** `docs/DX-VISION.md` — keep this queue aligned when starting or shipping work.
> **Protocol:** `docs/AUTONOMOUS-MODE.md`

Statuses: `ready` → `designing` → `implementing` → `pr-open` → `shipped` | `blocked` | `deferred`

## Active queue

| Pri | ID | Item | Track | Status | Spec / notes | Branch / PR |
|-----|-----|------|-------|--------|--------------|-------------|
| 1 | env-use-project-local | Ensure env/project context updates project-local | A | shipped | Smoke fix on ADM branch | PR #38 → `adm/queue-2026-07-19` |
| 2 | process-commands | Process mutations + shell-form command + Procfile apply | A / runtime | ready | End state: runtime-target-depth **slice 2**; plan before code | Spec 2026-07-20 |
| 3 | deploy-health | Portable process health → readiness; deploy timeout in target_config | A / runtime | ready | **Slice 3**; ideal after process-commands | Spec 2026-07-20 |
| 4 | release-config-materialization | Immutable per-hash/release config Secrets pinned by workload | B / runtime | ready | **Slice 1**; independent of process UX | Spec 2026-07-20 |
| 5 | target-extensions | Snapshotted target_extensions + capabilities schema/API | A / runtime | ready | **Slice 4**; cleaner after process mutations | Spec 2026-07-20 |
| 6 | clone-secret-placeholder | Optional secret key placeholders on clone (`needs_value` rows) | D | ready | Report-only today; sticky sensitivity rows optional | From ADM smoke 2026-07-19 |
| 7 | e2e-env-clone | e2e-stub coverage for env clone + needs_value | B | ready | Regression for secrets-aware clone | ADM scout |
| 8 | e2e-recipes-new | e2e or example path for `launchpad new` | B | ready | Day-one recipe path | ADM scout |
| 9 | status-unstage-hint | Status/help mention `unstage` next to reset | A | ready | IDEAS P3 polish | — |
| 10 | audit-config-keys | Audit events for config set: key + sensitivity only | D | ready | IDEAS from secrets-s1 | — |
| 11 | completions-man | Shell completions / man pages | C | ready | Track C surface polish | — |
| 12 | oidc-design | OIDC (Azure AD / Google / generic) design | D | deferred | After principals phase 1 dogfood; design before code | — |
| 13 | mcp-server | Launchpad MCP server | A/C | deferred | After core DX loop solid | — |
| 14 | multi-service | Multi-service + ReleaseSet | domain-3 | deferred | Do not half-build; full spec required | — |
| 15 | bindings | Config bindings `${{ refs }}` | domain-4 | deferred | Do not half-build; full spec required | — |
| 16 | launchpad-yaml | `launchpad.yaml` project manifest | domain-6 | deferred | Domain roadmap phase 6; round-trip process/health/extensions | — |

**Runtime depth program:** one design, four implementable IDs — `docs/superpowers/specs/2026-07-20-runtime-target-depth-design.md`. Recommended ADM order: `process-commands` → `deploy-health` → `release-config-materialization` → `target-extensions` (materialization may run in parallel with 1–2).

## Recently shipped (reference)

Keep short; full history lives in DX-VISION and merged specs.

| ID | Item | Spec |
|----|------|------|
| env-clone | Environment clone | `docs/superpowers/specs/2026-07-19-env-clone-design.md` · PR #37 (ADM) |
| worker-stress | Worker lease/supersede stress | `docs/superpowers/specs/2026-07-19-worker-stress-design.md` · PR #36 |
| target-conformance | Target conformance suite (stub) | `docs/superpowers/specs/2026-07-19-target-conformance-design.md` · PR #35 |
| postgres-ci | Postgres CI + pgx driver fix | `docs/superpowers/specs/2026-07-19-postgres-ci-design.md` · PR #34 |
| shell-prompt | Shell prompt awareness | `docs/superpowers/specs/2026-07-19-shell-prompt-design.md` · PR #33 |
| recipes-templates | Recipes / `launchpad new` | `docs/superpowers/specs/2026-07-19-recipes-templates-design.md` · PR #32 |
| unstage-last | Unstage last mutation | `docs/superpowers/specs/2026-07-19-unstage-last-design.md` · PR #31 |
| diff-env-env | Diff env↔env | `docs/superpowers/specs/2026-07-19-diff-env-env-design.md` · PR #30 |
| secrets-s2 | Secrets S2 AES-GCM | `docs/superpowers/specs/2026-07-18-secrets-typed-config-design.md` · PR #29 |
| secrets-s1 | Secrets S1 typing + redaction | PR #28 |

## How ADM uses this file

1. On session start, read this queue + DX-VISION **Active / next**.
2. Select the highest-priority `ready` item within the user budget (or the user-named item).
3. Move status → `designing` / `implementing` / `pr-open` as you go; commit queue updates on the feature branch or a docs commit.
4. On ship (merge), set `shipped` and update DX-VISION.
5. Promote rows from `IDEAS.md` only when: human asks, pre-authorized class (e.g. “any P0 from persona”), or the idea unblocks current dogfood **and** fits MVP.

Do not implement `deferred` or `blocked` items without an explicit human override and a spec.
