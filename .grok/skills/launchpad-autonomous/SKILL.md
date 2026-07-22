---
name: launchpad-autonomous
description: >
  Launchpad Autonomous Development Mode (ADM). Long-running, low-input feature
  work: named modes (single-feature / integration-stack / queue-drain),
  recommended path, spec+plan, worktree-isolated subagents, docs sync,
  verification ladder, hard stops, optional persona dogfood. Use when user
  authorizes autonomous mode, ADM, feature program, "ship without design debates",
  multi-feature agent loop, queue drain, or "/launchpad-autonomous".
---

# Launchpad Autonomous Development Mode

**Canonical protocol:** `docs/AUTONOMOUS-MODE.md` — read it at session start.

**Program files (required for selection + dogfood):**

| File | Use |
|------|-----|
| `docs/superpowers/program/QUEUE.md` | Pick next work; update status **on every merge** |
| `docs/superpowers/program/IDEAS.md` | Scout append-only; no silent builds |
| `docs/superpowers/program/PERSONA-SCRIPTS.md` | Persona user scripts |
| `docs/superpowers/program/feedback/` | Persona write-ups + optional `SESSION-*.md` |

**Helpers:** `scripts/adm-status` — ready rows, integration tip, dirty worktrees, open PRs.

**Also read:** `docs/FEATURE-DEVELOPMENT.md`, `docs/DX-VISION.md` (Active / next), `docs/DOMAIN.md` when entities/APIs move.

## Authorization gate

Do **not** enter ADM unless the user clearly authorizes it (e.g. “ADM authorized…”, “autonomous mode”, “run the feature program”, `/launchpad-autonomous`).

If unauthorized, use `/launchpad-feature` (human design approval before code).

At start, resolve a **named mode** and budget from `docs/AUTONOMOUS-MODE.md`:

| Mode | Default if user only says “ADM authorized” without detail |
|------|-----------------------------------------------------------|
| **single-feature** | Assume this: 1 PR, open only, no merge |
| **integration-stack** | User must name integration branch / “merge into adm” |
| **queue-drain** | User must say drain / remaining ready / until deferred only |

Restate mode + knobs in the first turn.

## Orchestrator checklist

1. **Mode + budget** — single-feature | integration-stack | queue-drain; merge policy; stop condition.
2. **Scope** — user-named feature or top `ready` row in `QUEUE.md` (cross-check DX-VISION); refuse `deferred`/`blocked` without override.
3. **DoD** — acceptance criteria on the row / plan / spec before code.
4. **Design** — spec + plan; recommended approach; self-review; QUEUE → `designing`.
5. **Branch / worktree** — never on `main`; **implementers in `.worktrees/feat-<name>`**; QUEUE Branch column = lease. QUEUE → `implementing`.
6. **Implement** — plan order; one Go layer per commit; tests with code; **push after each task**.
7. **Review** — two-stage (spec then quality) for service/api/jobs/cli/domain; combined only for trivial 1–2 file tasks.
8. **Docs sync** — DOMAIN / OpenAPI / DX-VISION / QUEUE / plan checkboxes.
9. **Verify** — ladder L0–L4; **L1 e2e-stub required** for service/jobs/target/deploy CLI; always `mise exec --`.
10. **Persona** — CLI/deploy UX: PERSONA-SCRIPTS → `feedback/`; stack/drain: S1 once before final PR to main (L1.5).
11. **Scout** — append IDEAS; promote only per protocol (same-PR fix / QUEUE / IDEAS only).
12. **Integrate** — push + `gh pr create`; QUEUE → `pr-open`. Stack/drain: merge into **integration only**; update QUEUE on that merge. Final PR integration → main; **no force-merge to main**.
13. **Stop** — mode stop (budget, drain complete = only deferred/blocked), or hard stop → report decision needed; leave trees intact.

## Isolation rules (non-negotiable)

- One implementer per branch + worktree.
- Orchestrator must **not** `reset --hard` / force-push an implementer’s leased branch.
- Do not thrash shared dirty checkouts across parallel agents.

## Hard stops (do not override)

Deferred-boundary / DOMAIN fork · self-review fail · 3× same-task verify fail · unfixable P0 dogfood · destructive git/shared actions · budget/mode stop · unexpected dirty WIP · branch ownership conflict.

## Related skills

| Skill | Role |
|-------|------|
| `launchpad-feature` | Full interactive feature workflow |
| `launchpad-domain` | Entity/API/invariants |
| `launchpad-dev` | Build, test, smoke, API/worker |

## Invocation examples

```text
/launchpad-autonomous
ADM authorized: single-feature — ship <feature>; 1 PR; no merge
ADM authorized: integration-stack — base adm/queue-2026-07-21; merge into adm; up to 3 PRs; final PR to main
ADM authorized: queue-drain — remaining ready items; merge into adm/…; stop when only deferred remain
```
