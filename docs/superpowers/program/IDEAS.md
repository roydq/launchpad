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
| 2026-07-19 | adm-diff-env | Live layer env↔env (resolved live, not last deploy) | P3 | A | Distinct from shipped deploy archaeology |
| 2026-07-19 | adm-diff-env | Improve release↔release to full snapshot union (removes) | P3 | A | Pending-style BuildDiff misses keys only on from |
| 2026-07-19 | adm-unstage | `unstage --key FOO` / interactive pick | P3 | A | Last-only is enough for now |
| 2026-07-19 | adm-unstage | Status help line mention `unstage` next to reset | P3 | A | Polish |
| 2026-07-18 | adm-design | Program SESSION run logs under `program/feedback/` | P3 | process | Optional; create when first ADM feature run needs them |
| 2026-07-18 | secrets-design | `secret_ref` / external SM (Vault, AWS SM) as future value kind | P2 | D | After S1/S2; do not block typing+redaction |
| 2026-07-18 | secrets-design | Dual-key secret re-encrypt / rotation job | P3 | D | S2 notes `key_id` prefix; implement when needed |
| 2026-07-18 | secrets-design | Optional `--include-secrets` on env clone (break-glass) | P2 | D | Default clone must not copy secret material |
| 2026-07-18 | secrets-design | Forbid plain service override of shared secret without `--force` | P3 | A | Spec allows total service win; policy later |
| 2026-07-18 | secrets-design | QUEUE rows `secrets-s1` / `secrets-s2` after human accepts model | P1 | D | **Promoted** to QUEUE (ready) after model accept |
| 2026-07-18 | secrets-s1 | CLI `config get --typed` pretty printer for agents | P3 | A | After S1; default map + sentinel is enough for humans |
| 2026-07-18 | secrets-s1 | Reject staging sensitivity without value on demote | P3 | D | Explicit plain demote already requires a value via KEY=VAL |
| 2026-07-18 | secrets-s1 | Audit events for config set: key + sensitivity only | P2 | D | Not wired this PR; keys already not logging values |
| 2026-07-18 | secrets-s2 | Dual-key rotation job + `key_id` beyond `v1:` prefix | P3 | D | Spec notes dual-key re-encrypt; single key is enough for now |
| 2026-07-18 | secrets-s2 | Auto-reencrypt legacy plaintext secrets on list | P3 | D | Currently open-as-plaintext until next write seals |
| 2026-07-18 | secrets-s2 | Dev convenience: generate+persist key under `.launchpad/` | P3 | A | Spec optional; env-only for S2 |

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
