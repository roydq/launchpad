---
name: launchpad-autonomous
description: >
  Launchpad Autonomous Development Mode (ADM). Long-running, low-input feature
  work: recommended path, spec+plan, subagents, docs sync, verification ladder,
  hard stops, optional persona dogfood. Use when user authorizes autonomous
  mode, ADM, feature program, "ship without design debates", multi-feature
  agent loop, or "/launchpad-autonomous".
---

# Launchpad Autonomous Development Mode

**Canonical protocol:** `docs/AUTONOMOUS-MODE.md` — read it at session start.

**Program files (required for selection + dogfood):**

| File | Use |
|------|-----|
| `docs/superpowers/program/QUEUE.md` | Pick next work; update status |
| `docs/superpowers/program/IDEAS.md` | Scout append-only; no silent builds |
| `docs/superpowers/program/PERSONA-SCRIPTS.md` | Persona user scripts |
| `docs/superpowers/program/feedback/` | Persona write-ups |

**Also read:** `docs/FEATURE-DEVELOPMENT.md`, `docs/DX-VISION.md` (Active / next), `docs/DOMAIN.md` when entities/APIs move.

## Authorization gate

Do **not** enter ADM unless the user clearly authorizes it (e.g. “ADM authorized…”, “autonomous mode”, “run the feature program”, `/launchpad-autonomous`).

If unauthorized, use `/launchpad-feature` (human design approval before code).

At start, confirm or apply budget defaults from `docs/AUTONOMOUS-MODE.md` (max PRs, no merge, scope).

## Orchestrator checklist

1. **Scope** — user-named feature or top `ready` row in `QUEUE.md` (cross-check DX-VISION); refuse `deferred`/`blocked` and silent deferred-boundary builds.
2. **Design** — spec + plan in `docs/superpowers/`; 2–3 approaches; pick recommended; status `Approved (autonomous)` only after self-review checklist in the protocol. Set QUEUE → `designing`.
3. **Branch** — never on `main`; prefer `.worktrees/feat-<name>`. QUEUE → `implementing`.
4. **Implement** — plan order; one Go layer per commit; tests with code; use subagents for multi-task plans (one implementer per branch).
5. **Review** — spec compliance then quality (or combined fast path for trivial tasks).
6. **Docs sync** — DOMAIN / OpenAPI / DX-VISION / QUEUE / plan checkboxes as required.
7. **Verify** — ladder L0–L4 in the protocol; always `mise exec --`.
8. **Persona** — when CLI/deploy UX in scope: `PERSONA-SCRIPTS.md` → `program/feedback/…`.
9. **Scout** — append `IDEAS.md`; promote to QUEUE only per protocol rules.
10. **Integrate** — push + `gh pr create` linking spec; QUEUE → `pr-open`; **do not merge** unless user said so.
11. **Stop** — budget done, no more authorized ready items, or hard stop → report decision needed; leave tree intact.

## Hard stops (do not override)

Deferred-boundary / DOMAIN fork · self-review fail · 3× same-task verify fail · unfixable P0 dogfood · destructive git/shared actions · budget exhausted · unexpected dirty WIP.

## Related skills

| Skill | Role |
|-------|------|
| `launchpad-feature` | Full interactive feature workflow |
| `launchpad-domain` | Entity/API/invariants |
| `launchpad-dev` | Build, test, smoke, API/worker |

## Invocation examples

```text
/launchpad-autonomous
ADM authorized: ship <feature>; 1 PR; no merge
Autonomous mode on the next DX-VISION item; stop on secrets ambiguity
```
