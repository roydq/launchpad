# Ideas and edge cases

> **Not a commitment to build.** Scout, persona user, and humans append here.
> **Promote to work:** move a row into `QUEUE.md` (or get human promotion). ADM must not silently implement ideas from this file.
> **Protocol:** `docs/AUTONOMOUS-MODE.md`

## Log

| Date | Source | Idea / edge case | Severity | Suggested track | Notes |
|------|--------|------------------|----------|-----------------|-------|
| 2026-07-18 | bootstrap | Seeded from DX-VISION “Later” / non-goals — see backlog rows below as themes, not tickets | — | — | Prefer QUEUE for ordered work |
| 2026-07-18 | dx-vision | Diff env↔env | P2 | A | Already on QUEUE as `diff-env-env` |
| 2026-07-18 | dx-vision | Unstage last mutation | P2 | A | QUEUE `unstage-last` |
| 2026-07-18 | dx-vision | `launchpad run` / env pull (local parity) | P3 | A | Local env injection without full deploy |
| 2026-07-18 | dx-vision | Ephemeral / PR environments | P3 | A/D | Needs identity + lifecycle; after secrets |
| 2026-07-18 | dx-vision | Completions / man pages | P3 | C | Surface polish |
| 2026-07-18 | dx-vision | TUI (inspect / deploy / releases) | P3 | C | Same apiclient as CLI |
| 2026-07-18 | dx-vision | Docs site (get-started + mental model) | P3 | C | After API stable |
| 2026-07-18 | dx-vision | Web dashboard MVP | P3 | C | OpenAPI + auth first |
| 2026-07-18 | dx-vision | Idempotency keys | P2 | D | Agent-friendly retries |
| 2026-07-18 | dx-vision | Deployment events / SSE | P2 | D | Live job progress beyond poll |
| 2026-07-18 | dx-vision | HA workers / packaging | P3 | D | Hosted readiness |
| 2026-07-18 | dx-vision | Richer RBAC / env policy | P2 | D | After roles + OIDC |
| 2026-07-18 | dx-vision | Hosted control plane (BYO data plane) | P3 | D | Future; same binary |
| 2026-07-18 | domain | Workspace config layer | P2 | domain | Deferred above service/shared |
| 2026-07-18 | agents | Scale API (target-side) | P3 | B | Deferred; target may already scale |
| 2026-07-18 | agents | Builds / image factory | P3 | A | Image-only releases for now |
| 2026-07-18 | non-goal | Continuous GitOps reconciliation | — | — | Explicit non-goal |
| 2026-07-18 | non-goal | Helm as primary UX | — | — | Explicit non-goal |
| 2026-07-18 | adm-design | Persona finds unclear recovery on pin mismatch | P2 | B | Validate on next persona run; add e2e if real |
| 2026-07-18 | adm-design | Program SESSION run logs under `program/feedback/` | P3 | process | Optional; create when first ADM feature run needs them |

## Severity guide

| Tag | Meaning |
|-----|---------|
| **P0** | Broken dogfood path or data risk — fix or hard-stop |
| **P1** | Core DX friction; should enter QUEUE soon |
| **P2** | Real improvement; schedule when capacity allows |
| **P3** | Nice-to-have / later surface |

## Append format

When scouting or running persona dogfood, append a row (do not rewrite history). Example:

```markdown
| 2026-07-20 | persona-user | `deploy --wait` silent on worker down | P1 | B | Repro: stop worker, deploy; want hint to start worker |
```

If severity is **P0** and the user pre-authorized “queue P0s from persona,” add a `ready` fix row to `QUEUE.md` and mention it in the PR.
