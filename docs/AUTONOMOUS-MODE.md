# Autonomous Development Mode (ADM)

| Field | Value |
|-------|-------|
| **Status** | Experimental — process protocol (docs + skill + program files) |
| **Date** | 2026-07-18 |
| **Related** | `docs/FEATURE-DEVELOPMENT.md`, `docs/DX-VISION.md`, `.grok/skills/launchpad-autonomous/SKILL.md`, `docs/superpowers/program/` |

Long-running, low-input agent work: pick recommended paths, implement planned features, keep docs in sync, expand verification when risk warrants it, use subagents, and stop cleanly when human judgment is required.

This is **process**, not product code. Early-repo experiment — refine the protocol as runs teach us.

### Program files (Approach B)

| Path | Purpose |
|------|---------|
| [`docs/superpowers/program/QUEUE.md`](superpowers/program/QUEUE.md) | Ordered work items + status for ADM selection |
| [`docs/superpowers/program/IDEAS.md`](superpowers/program/IDEAS.md) | Edge cases / future ideas (not a build commitment) |
| [`docs/superpowers/program/PERSONA-SCRIPTS.md`](superpowers/program/PERSONA-SCRIPTS.md) | Synthetic user dogfood scripts |
| [`docs/superpowers/program/feedback/`](superpowers/program/feedback/) | Persona write-ups per run |

---

## When to use

| Situation | Mode |
|-----------|------|
| Human wants a multi-step feature with little back-and-forth | ADM (this doc) |
| One small fix or single-file change | Skip — normal branch + PR |
| Ambiguous product direction / deferred-boundary design | Spec with human gate (not self-approve) |
| Human wants design debate | Normal `/launchpad-feature` interactive flow |

**Authorization required.** The user must explicitly enable ADM for a session (or a named budget), e.g.:

> “ADM authorized: implement server-side X; up to 1 PR; no merge; stop on domain ambiguity.”

Without that phrase (or equivalent), do **not** self-approve designs or run multi-feature loops.

---

## Principles

1. **Files over chat** — resume from specs, plans, `QUEUE.md`, git log, and this protocol.
2. **Recommended path only** — write 2–3 approaches in the spec; pick the one that matches `DOMAIN.md` + MVP + DX-VISION sequencing; do not re-debate in chat.
3. **Spec + plan still required** for medium+ work — autonomy does not skip artifacts.
4. **PRs for humans** — open PRs for dogfood; **never force-merge** unless the user explicitly allows merge.
5. **Hard stops beat cleverness** — escalate rather than invent product policy.
6. **Docs are part of the feature** — DOMAIN, OpenAPI, DX-VISION, plan checkboxes, and queue status stay current in the same PR series.
7. **Subagents for focus** — fresh implementer context per task when the plan is multi-task; orchestrator coordinates only.
8. **Queue over vibes** — pick work from `QUEUE.md` (aligned with DX-VISION); park discoveries in `IDEAS.md` without silent scope expansion.

---

## Session budget (set at start)

At authorization, record (or assume defaults):

| Knob | Default if unspecified |
|------|------------------------|
| Max features / PRs | 1 |
| Max wall time | Session lifetime |
| Merge policy | Open PR only; no merge |
| Scope | Top `ready` row in `QUEUE.md`, DX-VISION “Active / next”, or user-named feature |
| Scout (edge cases / ideas) | Append to `IDEAS.md`; do not implement unless promoted to QUEUE |
| Persona user dogfood | Run when deploy/CLI UX changed (scripts in `PERSONA-SCRIPTS.md`); write `program/feedback/` |

---

## Roles

| Role | Who | Responsibility |
|------|-----|----------------|
| **Orchestrator** | Parent session | Budget, gates, dispatch, docs checklist, PR, stop/continue |
| **Designer** | Parent or plan subagent | Spec + plan; recommended approach; self-review checklist |
| **Implementer** | Subagent (worktree optional) | One plan task / layer; tests; commit message from plan |
| **Spec reviewer** | Read-only subagent | Code matches spec; no extras |
| **Quality reviewer** | Read-only subagent | Conventions, clarity, test quality |
| **Docs sync** | Parent or implementer | DOMAIN / OpenAPI / DX-VISION / README as needed |
| **Scout** | Explore / plan | Edge cases and future ideas → `IDEAS.md` only |
| **Persona user** | Execute-capable subagent | Scripts in `PERSONA-SCRIPTS.md`; feedback under `program/feedback/` |

### Parallelism

- **One implementer per branch** (avoid edit conflicts).
- Reviewers after implementer finishes that task.
- Scout / persona only after green verification for the feature slice.
- Independent features in separate worktrees only if the user authorized multi-feature budget.

### Fast path (mechanical tasks)

For 1–2 file, fully specified plan tasks (e.g. pure store method + test), orchestrator may use **one combined review** (spec + quality) instead of two subagents. Prefer full two-stage review for service, API, worker, CLI, or domain changes.

---

## End-to-end loop

```text
User authorizes ADM + budget
  → Orchestrator reads DOMAIN, DX-VISION, FEATURE-DEVELOPMENT, QUEUE.md, this doc
  → Select work item (user-named or top ready QUEUE row / Active / next)
  → Update QUEUE status → designing
  → Designer: approaches → recommended → spec + plan
  → Spec self-review checklist → self-approve OR hard stop
  → Branch / worktree (never implement on main)
  → QUEUE → implementing
  → For each plan task:
        Implementer → L0 verify → review(s) → commit → check off plan
  → Docs sync if model/routes/CLI changed
  → Verification ladder (L0–L4 as triggered)
  → Persona dogfood when CLI/deploy UX in scope (PERSONA-SCRIPTS)
  → Scout pass → append IDEAS.md (no code from ideas alone)
  → Push + open PR (link spec; test plan); QUEUE → pr-open
  → Update DX-VISION status if applicable
  → Stop (budget) or next authorized ready item
```

Resume across sessions: `QUEUE.md` + plan checkboxes + `git log main..HEAD` + green `make test` (same as FEATURE-DEVELOPMENT).

---

## Spec self-review (self-approve gate)

Before implementation without human design approval, **all** must pass:

1. **No placeholders** — no TBD/TODO in requirements; no vague “handle errors appropriately.”
2. **Internal consistency** — API sketch, schema, and success criteria agree.
3. **Single plan scope** — one vertical slice; not multi-service + secrets + OIDC in one PR.
4. **No DOMAIN contradiction** — if mental model changes, DOMAIN update is in the plan.
5. **MVP boundary** — does not half-build AGENTS.md deferred items; if it would, stop or write a design-only spike for human review.
6. **Recommended path recorded** — rejected approaches listed briefly in the spec.
7. **Test strategy present** — unit and, if deploy-path, e2e-stub or smoke.

Mark the spec status, e.g. `Approved (autonomous)` or `Approved (self-approve — ADM)`.

If any check fails → **hard stop** and present the open question to the human.

---

## Recommended path selection

When choosing an approach (and when choosing *what* to build next):

1. Prefer an in-progress plan / `implementing` or `pr-open` fix over a new QUEUE row.
2. Else prefer top **`ready`** row in `QUEUE.md` that matches the user budget (design-only rows stay design-only).
3. Cross-check DX-VISION **Active / next** and Track A/B over C/D until dogfood is boring.
4. Prefer the option that preserves existing CLI verbs and ambient env context.
5. Prefer smaller shippable PR over completeness theater.
6. Never invent multi-service, bindings, secrets **implementation**, or OIDC as drive-by scope.
7. Never implement `deferred` / `blocked` QUEUE rows without human override + spec.

---

## Docs sync checklist

After implementation, before PR:

| Changed | Update |
|---------|--------|
| Entities, invariants, lifecycles | `docs/DOMAIN.md` |
| HTTP routes / schemas | `docs/openapi.yaml` + `make openapi-check` |
| Product sequencing / shipped table | `docs/DX-VISION.md` |
| Program work item | `docs/superpowers/program/QUEUE.md` status |
| User-facing run paths | `README.md` if the 60s path or env vars change |
| Plan progress | Checkboxes + status header in plan file |

Docs commits may be `docs:` on the feature branch; DOMAIN model changes belong in the same PR series as the code.

---

## Verification ladder

| Level | When | What |
|-------|------|------|
| **L0** | Every task / commit | `mise exec -- make test && make build && go vet ./...` |
| **L1** | `internal/service`, `jobs`, `target`, deploy CLI | `make e2e-stub` (or mid-task smoke per `/launchpad-dev`) |
| **L2** | New/changed HTTP routes | Update OpenAPI + `make openapi-check` |
| **L3** | Multi-env, promote, config resolution | Existing e2e coverage or add cases in the same PR |
| **L4** | Before PR | All triggered levels + plan “Final verification” + no debug leftovers |

**Expand verification when:** persona or reviewer finds an uncovered failure mode; auth/config/deploy semantics change; a bug fix needs a regression test. Expansion lands in the same PR when practical; otherwise note a follow-up in the PR body (not silent drop).

Always use `mise exec --` for Go commands (see AGENTS.md).

---

## Hard stops (human required)

Stop ADM and report clearly:

1. **Deferred-boundary or DOMAIN fork** — secrets model, multi-service, OIDC shape, half-built deferred APIs.
2. **Spec self-review failure** — open TBD or two equally valid architectures.
3. **3× verification failure** on the same task after distinct fix attempts.
4. **P0 product break** from persona dogfood that one retry cannot fix.
5. **Destructive / shared actions** outside policy — force-push, `reset --hard` of published history, merge without permission, dropping data.
6. **Budget exhausted** — max PRs, time, or user-defined limit.
7. **Unexpected repo state** — unfamiliar dirty tree or branch that may be human WIP.

On hard stop: leave branch/worktree intact, summarize state, list the decision needed, do not thrash.

---

## Persona user (synthetic dogfood)

**Canonical scripts:** [`docs/superpowers/program/PERSONA-SCRIPTS.md`](superpowers/program/PERSONA-SCRIPTS.md)

When CLI/deploy UX is in scope (or user requests dogfood):

1. Run at least **S1** (day-one path) and **S4** (deliberate mistakes) after green verification.
2. Add S2/S3 when config or multi-env/promote is touched.
3. Write `docs/superpowers/program/feedback/YYYY-MM-DD-<feature>.md`.
4. Summarize findings in the PR body.

Promotion: same-PR fixes for regressions this feature introduced; otherwise `IDEAS.md` / QUEUE per persona script rules. Do not treat persona output as automatic multi-feature scope expansion.

If dogfood cannot run, record `blocked` + reason in feedback — do not fake a pass.

---

## Scout (edge cases / future ideas)

**Canonical log:** [`docs/superpowers/program/IDEAS.md`](superpowers/program/IDEAS.md)

After a green feature slice (or at session end):

1. Append rows for edge cases, doc gaps, and follow-ups (date, source, severity, track).
2. Mention notable P0/P1 items in the PR body.
3. Promote to `QUEUE.md` only if human asked, pre-authorized class (e.g. auto-queue P0s), or the idea unblocks current dogfood **and** fits MVP.

Do **not** silently implement scout ideas.

---

## Subagent guidance (orchestrator)

Prefer worktree isolation for non-trivial implementation (`.worktrees/feat-<name>`).

Per multi-task plan:

1. Extract full task text for the implementer (do not make them re-discover the plan alone).
2. Include: repo conventions from AGENTS.md, verify commands, exact commit message, layer boundaries.
3. On `BLOCKED` / `NEEDS_CONTEXT`: supply context or escalate; do not infinite-retry the same prompt.
4. Spec compliance review **before** code quality review when both run.
5. Mark plan checkboxes after each accepted task; commit plan progress periodically.

Reuse patterns from superpowers `subagent-driven-development` and bundled `execute-plan` personas when available; Launchpad layer-commit rules still win.

---

## Relationship to other docs

| Doc | Role |
|-----|------|
| `docs/FEATURE-DEVELOPMENT.md` | Canonical feature workflow; ADM is an **authorization mode** on top |
| `docs/DX-VISION.md` | What to build next; backlog truth |
| `docs/DOMAIN.md` | Product mental model — do not contradict |
| `AGENTS.md` | Toolchain, layout, MVP cut line |
| `.grok/skills/launchpad-feature` | Interactive / default feature skill |
| `.grok/skills/launchpad-autonomous` | Agent entrypoint for this protocol |
| `.grok/skills/launchpad-dev` | Build, test, smoke, local processes |
| `.grok/skills/launchpad-domain` | Entity / API / invariant questions |

---

## What ADM is not

- Not a license to skip tests, OpenAPI, or DOMAIN updates.
- Not overnight unattended CI (no durable agent host yet).
- Not auto-merge to main.
- Not multi-service or secrets implementation without an explicit design path.
- Not a substitute for human product taste on hard stops.

---

## Evolution

Current = protocol + skill + program files (queue, ideas, persona scripts). When runs are boringly useful, consider:

- Scheduler-backed continuation (session ticks, like `/pr-babysit`)
- Standalone `/launchpad-persona` skill
- Durable overnight agent host (Approach C)

Update this file when policy changes; keep experiments cheap while the product is early.
