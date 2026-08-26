# Ideas and edge cases

> **Not a commitment to build.** Scout, persona user, and humans add rows here.
> **Promote to work:** move a row into `QUEUE.md` (or get human promotion). ADM must not silently implement ideas from this file.
> **Protocol:** `docs/AUTONOMOUS-MODE.md`
>
> This is a **working set**, not a transcript. Shipped / promoted / wontfix / DX-VISION-duplicate rows are pruned into [Resolved](#resolved). New scout rows go at the **bottom of the Open table** (last section of this file) so end-of-file appends stay in the table.

Housekeeping: 2026-08-26 — recovered rows that had been pasted after the severity guide; dropped shipped, promoted, and DX-VISION/DOMAIN duplicates. Promoted preview FoldChanges 400 → QUEUE `preview-process-fold`.

## Severity guide

| Tag | Meaning |
|-----|---------|
| **P0** | Broken dogfood path or data risk — fix or hard-stop |
| **P1** | Core DX friction; should enter QUEUE soon |
| **P2** | Real improvement; schedule when capacity allows |
| **P3** | Nice-to-have / later surface |

## Append format

Add **one table row** to **Open** (end of this file). Do not paste rows under this heading. Do not re-log something already in Open, Resolved, QUEUE, or DX-VISION Later / deferred DOMAIN.

```markdown
| 2026-07-20 | persona-user | `deploy --wait` silent on worker down | P1 | B | Repro: stop worker, deploy; want hint to start worker |
```

If severity is **P0** and the user pre-authorized “queue P0s from persona,” add a `ready` fix row to `QUEUE.md` and mention it in the PR.

Scouts do not edit other rows. Housekeeping (human or explicit “trim IDEAS”) may prune into Resolved.

## Resolved

Do not re-open without new evidence. History lives in QUEUE, DX-VISION, and git.

**Fixed / promoted then shipped**

- Project-local `env use` after `launchpad new` — PR #38
- Postgres Open used driver name `postgres` instead of `pgx` — PR #34
- Secrets S1/S2; clone secret placeholders (#49); audit `config.set` key+sensitivity (#50)
- e2e coverage for `launchpad new` and env clone — PR #44
- Status help mentions `unstage` — PR #43
- Diff env↔env; unstage last mutation; env clone; recipes/`launchpad new`
- Runtime depth: process commands, deploy health, immutable config Secrets, target extensions
- Completions (man pages remain a DX-VISION Track C later item)
- `launchpad.yaml` v1; MCP stdio v1 ([PR #61](https://github.com/roydq/launchpad/pull/61))
- **Promoted** pending preview FoldChanges 400 on `process.set` → QUEUE `preview-process-fold`
- Worker concurrent lease + reclaim (ADM #36) covers the Postgres lease-stress scout note

**Wontfix / no action**

- Control-plane HTTP health poll after target ready — rejected as primary; keep target readiness
- Doctor + prompt/shell-init — local-only, still helpful
- Optional `program/feedback/SESSION-*.md` — already in the ADM protocol; create when a run needs them

**Themes that belong in DX-VISION / DOMAIN (not this log)**

`launchpad run` / env pull · ephemeral/PR envs · TUI · docs site · web dashboard · man pages · idempotency keys · deployment events/SSE · HA workers · richer RBAC · hosted control plane · workspace config layer · scale API · builds / image factory · GitOps reconciliation and Helm-as-UX (explicit non-goals)

## Open

| Date | Source | Idea / edge case | Severity | Suggested track | Notes |
|------|--------|------------------|----------|-----------------|-------|
| 2026-07-18 | adm-design | Persona finds unclear recovery on pin mismatch | P2 | B | Validate on next persona run; add e2e if real |
| 2026-07-18 | secrets | External `secret_ref` (Vault, AWS SM) | P2 | D | After S1/S2; do not block typing+redaction |
| 2026-07-18 | secrets | Dual-key rotation + auto-reencrypt legacy plaintext on list | P3 | D | Single `v1:` key is enough for now |
| 2026-07-18 | secrets | Clone `--include-secrets`; `--force` to override shared secret with plain | P2 | D/A | Default clone must not copy secrets; spec allows total service win |
| 2026-07-18 | secrets | `config get --typed`; persist generated key under `.launchpad/`; reject sensitivity-only demote | P3 | A | Map+sentinel is enough for humans; explicit `KEY=VAL` demote already required |
| 2026-07-19 | adm-diff-env | Live layer env↔env (resolved live, not last deploy) | P3 | A | Distinct from shipped deploy archaeology |
| 2026-07-19 | adm-diff-env | Release↔release full snapshot union (removes) | P3 | A | Pending-style BuildDiff misses keys only on from |
| 2026-07-19 | adm-unstage | `unstage --key FOO` / interactive pick | P3 | A | Last-only is enough for now |
| 2026-07-19 | adm-scout | Fish shell-init | P3 | C | bash/zsh only for now |
| 2026-07-19 | adm-scout | Recipe `web-k8s` with namespace defaults | P3 | A | After k8s dogfood |
| 2026-07-20 | runtime | Out of v1: `command_argv` exec form; liveness≠readiness; ConfigMap+Secret split | P3 | A | Shell form, readiness-only, single immutable Secret |
| 2026-07-20 | adm-runtime | Extension validation reject unknown keys at stage | P2 | A | Slice 4 shipped soft-apply; strict validate later |
| 2026-07-20 | adm-runtime | Config Secret GC of unreferenced hashes | P3 | B | Destroy cleans all; janitor optional |
| 2026-07-20 | adm-runtime | `process set --target-ext` CLI sugar | P3 | A | API map works via stage |
| 2026-07-20 | adm-runtime | Kind e2e for probes / resources / immutable secrets | P2 | B | Stub CI green; kind optional |
| 2026-08-22 | adm-yaml | `launchpad apply --project` retarget without editing yaml | P3 | A | e2e rewrites `project:`; no CLI flag in v1 |
| 2026-08-23 | adm-mcp | Streamable HTTP MCP in `cmd/api` | P3 | A/C | Spec v1 is stdio only; hosted agents would need this + OAuth |
| 2026-08-23 | adm-mcp | `process.apply` MCP tool with Procfile text | P3 | A | File path was out of scope; agents could pass contents |
| 2026-08-23 | adm-mcp | MCP resources for logs / manifest URIs | P3 | C | Tools-only v1; resources later |
